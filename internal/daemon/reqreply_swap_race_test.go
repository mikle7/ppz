package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// TestBuildFilteredList_SurvivesSwapWindow is the regression test for the
// mikle7/ppz#1 transient variant: `ppz subs read` no-shows a message during
// the routine ~270s JWT-refresh swap and works on retry.
//
// Root cause (confirmed): buildFilteredList's JetStream request-reply calls
// (streamInfoByName/ListStreams) run on the shared NC; a swapNC closing that
// NC mid-request makes the request fail, surfaced as ENATSUnreachable. Unlike
// the subscription/arrival path (rearmAll dual-subscribes + flushes before
// closing old — proven swap-safe by TestPublishDuringSwapWindowStillWakesWatch),
// the request-reply path had no resilience.
//
// Two fixes verified here, both against real production code racing a real
// swapNC loop:
//
//	Part A — buildFilteredList reads d.NC via currentNC() (under ncMu), so the
//	         reader/swap access is race-free. This test run under -race is the
//	         proof: it must stay clean where a bare d.NC read would flag.
//	Part B — buildFilteredListRetrying retries once on ENATSUnreachable, so a
//	         transient swap-window failure is ridden out instead of surfaced.
//	         Asserted: zero surfaced errors across the whole swap storm.
//
// The bare-buildFilteredList phase is informational (logged, not asserted) to
// avoid timing flake — it shows the window is genuinely exercised (single
// attempts DO fail), which is what Part B then absorbs.
func TestBuildFilteredList_SurvivesSwapWindow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sources", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cliproto.ListSourcesReply{
			Sources: []cliproto.Source{{Handle: "alice"}},
		})
	})
	mux.HandleFunc("GET /api/v1/pipes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cliproto.ListUncollaredPipesReply{})
	})
	d := newDaemonWithFakeServer(t, mux)

	nc0 := startEmbeddedJS(t)
	url := nc0.ConnectedUrl()
	d.swapNC("test-init", nc0)

	acct := uuid.New()
	ctx := context.Background()

	// A real stream+message so ListStreams/GetMsg do genuine request-reply
	// work (widening the in-flight window a swap can land in).
	js0, err := jetstream.New(nc0)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	if _, err := js0.CreateStream(ctx, jetstream.StreamConfig{
		Name:     natsubj.BuildStreamName(acct, "", "alice", "inbox"),
		Subjects: []string{natsubj.Subject(acct, "alice", "inbox")},
	}); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if _, err := js0.Publish(ctx, natsubj.Subject(acct, "alice", "inbox"), []byte("hi")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// swapDriver fires `swaps` back-to-back JWT-refresh-style swaps (fresh
	// conn each), spaced enough that a two-attempt retry fits between two
	// swaps, then signals done. Returns the conns to close on cleanup.
	swapDriver := func(swaps int, spacing time.Duration) chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			var conns []*nats.Conn
			t.Cleanup(func() {
				for _, c := range conns {
					c.Close()
				}
			})
			for i := 0; i < swaps; i++ {
				nc, err := nats.Connect(url)
				if err != nil {
					return
				}
				conns = append(conns, nc)
				d.swapNC("test-jwt-refresh", nc)
				time.Sleep(spacing)
			}
		}()
		return done
	}

	// Phase 1 (informational): bare buildFilteredList — single attempt. Shows
	// the swap window is real (single attempts fail transiently). Race-free
	// because buildFilteredList reads d.NC via currentNC() now.
	bareErrs, bareRuns := 0, 0
	done := swapDriver(80, 3*time.Millisecond)
	for {
		select {
		case <-done:
			goto phase2
		default:
		}
		bareRuns++
		if _, e := d.buildFilteredList(ctx, acct, "s", []string{"alice"}); e != nil && e.Code == cliproto.ENATSUnreachable {
			bareErrs++
		}
	}
phase2:
	t.Logf("phase1 bare buildFilteredList: %d/%d attempts hit ENATSUnreachable during swaps (window exercised)", bareErrs, bareRuns)

	// Phase 2 (asserted): buildFilteredListRetrying — must never surface an
	// error across the whole swap storm.
	fixErrs, fixRuns := 0, 0
	done = swapDriver(80, 3*time.Millisecond)
	for {
		select {
		case <-done:
			goto check
		default:
		}
		fixRuns++
		if _, e := d.buildFilteredListRetrying(ctx, acct, "s", []string{"alice"}); e != nil {
			fixErrs++
			t.Errorf("buildFilteredListRetrying surfaced %s during a swap — retry did not ride out the window", e.Code)
		}
	}
check:
	t.Logf("phase2 buildFilteredListRetrying: %d/%d runs errored (want 0)", fixErrs, fixRuns)
	if fixErrs != 0 {
		t.Fatalf("REGRESSION: retry did not absorb the swap window (%d/%d errored)", fixErrs, fixRuns)
	}
}
