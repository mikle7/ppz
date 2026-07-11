package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// Regression for mikle7/ppz#1 duplicate variant. The daemon re-reads the full
// retained stdin backlog on every follow-redial (NoAdvance). The forwardStdin
// fix bounds SinceMS to a recent lookback so an OLD backlog is dropped on BOTH
// the historical-drain and live-follow age filters, on a redial, regardless of
// how long the wrapper has been running.
//
// This test drives the daemon with an OLD backlog and asserts:
//   - an UNBOUNDED SinceMS (the pre-fix long-lived-wrapper value) replays it;
//   - a BOUNDED SinceMS (the fix's capped value) does NOT.
func TestStdinRelay_BoundedLookback_NoReplayOfOldBacklog(t *testing.T) {
	// Mirror internal/cli.stdinRelayMaxLookback (can't import cli from daemon).
	const maxLookback = 60 * time.Second

	replayed := func(t *testing.T, sinceMS int64) int {
		nc := startEmbeddedJS(t)
		url := nc.ConnectedUrl()
		home, err := os.MkdirTemp("/tmp", "ppz-lookback-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(home) })
		sockPath := filepath.Join(home, "daemon.sock")
		d := New(home, sockPath)
		loginForWakeTests(t, d)
		d.Refresh = &RefreshLoop{
			AccountID: "00000000-0000-0000-0000-000000000001",
			Refresh: func(_ context.Context, _ string) (string, string, int64, error) {
				return "jwt", "seed", time.Now().Add(time.Hour).Unix(), nil
			},
		}
		if err := d.Refresh.Start(context.Background(), "jwt", "seed", time.Now().Add(time.Hour).Unix()); err != nil {
			t.Fatalf("RefreshLoop.Start: %v", err)
		}
		t.Cleanup(d.Refresh.Stop)
		d.NATSURL = url
		d.swapNC("test-init", nc)

		accountID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		ctx := context.Background()
		js, err := jetstream.New(nc)
		if err != nil {
			t.Fatalf("jetstream.New: %v", err)
		}
		manifold := d.State.CurrentNamespace("test-session")
		streamName := natsubj.BuildStreamName(accountID, manifold, "", "stdin")
		subject := natsubj.BuildSubject(accountID, manifold, "", "stdin")
		if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
			Name: streamName, Subjects: []string{subject},
		}); err != nil {
			t.Fatalf("create stream: %v", err)
		}

		// OLD backlog: created 90 min ago — older than the fix's lookback,
		// newer than a long-lived wrapper's process-start cutoff.
		const N = 300
		old := time.Now().Add(-90 * time.Minute)
		for i := 0; i < N; i++ {
			m := envelope.New("mstrctl", subject, fmt.Sprintf("old-%03d", i), old)
			b, _ := m.Marshal()
			if _, err := js.Publish(ctx, subject, b); err != nil {
				t.Fatalf("publish %d: %v", i, err)
			}
		}

		daemonCtx, daemonCancel := context.WithCancel(context.Background())
		t.Cleanup(daemonCancel)
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go d.serveIPC(daemonCtx, ln)

		got := make(chan string, 4096)
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		body, _ := json.Marshal(cliproto.ReadRequest{
			BareTarget: "stdin", Session: "test-session",
			Follow: true, NoAdvance: true, SinceMS: sinceMS,
		})
		_ = json.NewEncoder(conn).Encode(map[string]any{
			"method": cliproto.IPCRead, "params": json.RawMessage(body),
		})
		go func() {
			sc := bufio.NewScanner(conn)
			sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			for sc.Scan() {
				var evt cliproto.ReadEvent
				if json.Unmarshal(sc.Bytes(), &evt) == nil && evt.Message != nil {
					got <- evt.Message.Payload
				}
			}
		}()

		return drainLookback(got, N, 3*time.Second)
	}

	// Pre-fix long-lived wrapper: SinceMS = age of a 2h-old process -> cutoff is
	// 2h ago, well before the 90-min-old backlog -> it all gets re-delivered.
	unbounded := (2 * time.Hour).Milliseconds()
	if n := replayed(t, unbounded); n == 0 {
		t.Fatalf("expected the old backlog to replay with an unbounded SinceMS (pre-fix), got 0 — test not exercising the path")
	} else {
		t.Logf("unbounded SinceMS (pre-fix): %d old messages replayed (bug present without the cap)", n)
	}

	// Post-fix: SinceMS capped at the lookback -> cutoff is 60s ago -> the
	// 90-min-old backlog is filtered on both drain and live paths -> 0 replay.
	bounded := maxLookback.Milliseconds() + 1
	if n := replayed(t, bounded); n != 0 {
		t.Fatalf("REGRESSION: capped SinceMS still replayed %d old messages", n)
	}
	t.Logf("bounded SinceMS (fix): 0 old messages replayed")
}

// drainLookback counts deliveries arriving within timeout, up to max, stopping
// early once the stream goes quiet.
func drainLookback(ch <-chan string, max int, timeout time.Duration) int {
	n := 0
	deadline := time.After(timeout)
	quiet := time.NewTimer(1200 * time.Millisecond)
	defer quiet.Stop()
	for n < max {
		select {
		case <-ch:
			n++
			quiet.Reset(1200 * time.Millisecond)
		case <-quiet.C:
			return n
		case <-deadline:
			return n
		}
	}
	return n
}
