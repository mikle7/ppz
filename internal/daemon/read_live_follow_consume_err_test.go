package daemon

import (
	"context"
	"encoding/json"
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

// TestHandleRead_Follow_ConsumeErrIsRecordedAndSelfHealStillWorks pins the
// diagnostic-only instrumentation added for mikle7/ppz#1 ("forwards once,
// then silently stops" — see docs/investigations/
// relay-stops-forwarding-2026-07-11.md, Update 3). Before this, ANY error
// jetstream.OrderedConsumer hit while live-following was completely
// invisible: no log, no daemon-side trace. The fix here is deliberately
// log-only — it must NOT change delivery behavior, only make errors
// observable via the existing NATSEvent ring (`ppz diagnostics`).
//
// This forces a real ErrConsumerDeleted (one of the 5 sentinel errors
// nats.go's orderedConsumer self-heals from) by deleting the live
// consumer's underlying ephemeral JetStream consumer out from under it —
// the same technique the 2026-07-11 "Ruled out: generic consumer
// recycling" scratch test used. Asserts BOTH halves of the "log-only, no
// behavior change" contract in one test:
//  1. message #2 still arrives after the forced deletion (self-heal is
//     unaffected by the new handler), and
//  2. a "warn"/"handleRead-liveFollowConsume" NATSEvent was recorded with
//     the ErrConsumerDeleted-shaped reason.
func TestHandleRead_Follow_ConsumeErrIsRecordedAndSelfHealStillWorks(t *testing.T) {
	nc := startEmbeddedJS(t)

	home, err := os.MkdirTemp("/tmp", "ppz-consume-err-")
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

	d.NATSURL = nc.ConnectedUrl()
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
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
	})
	if err != nil {
		t.Fatalf("create stream: %v", err)
	}

	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	t.Cleanup(daemonCancel)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen unix %s: %v", sockPath, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go d.serveIPC(daemonCtx, ln)

	ipcConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial IPC: %v", err)
	}
	t.Cleanup(func() { _ = ipcConn.Close() })

	body, _ := json.Marshal(cliproto.ReadRequest{
		BareTarget: "stdin",
		Session:    "test-session",
		Follow:     true,
		NoAdvance:  true,
		SinceMS:    1,
	})
	if err := json.NewEncoder(ipcConn).Encode(map[string]any{
		"method": cliproto.IPCRead,
		"params": json.RawMessage(body),
	}); err != nil {
		t.Fatalf("encode Follow request: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.Follows.mu.Lock()
		n := len(d.Follows.conns)
		d.Follows.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let the live OrderedConsumer attach

	dec := json.NewDecoder(ipcConn)
	readOne := func(within time.Duration) (string, bool) {
		_ = ipcConn.SetReadDeadline(time.Now().Add(within))
		var evt cliproto.ReadEvent
		if err := dec.Decode(&evt); err != nil {
			return "", false
		}
		if evt.Message == nil {
			return "", false
		}
		return evt.Message.Payload, true
	}

	publish := func(payload string) {
		env := envelope.New("someone", "", payload, time.Now())
		b, err := env.Marshal()
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		if _, err := js.Publish(ctx, subject, b); err != nil {
			t.Fatalf("publish %q: %v", payload, err)
		}
	}

	publish("MESSAGE ONE")
	payload, ok := readOne(2 * time.Second)
	if !ok || payload != "MESSAGE ONE" {
		t.Fatalf("message #1 did not arrive as expected (payload=%q ok=%v)", payload, ok)
	}

	// Force ErrConsumerDeleted: delete every ephemeral consumer registered
	// on the stream out from under the live OrderedConsumer. It should
	// notice, fire our new ConsumeErrHandler, and self-heal.
	names := stream.ConsumerNames(ctx)
	var deleted int
	for name := range names.Name() {
		if derr := stream.DeleteConsumer(ctx, name); derr == nil {
			deleted++
		}
	}
	if err := names.Err(); err != nil {
		t.Fatalf("list consumers: %v", err)
	}
	if deleted == 0 {
		t.Fatalf("no consumers found to delete -- test setup didn't reach the live-follow OrderedConsumer")
	}

	publish("MESSAGE TWO")
	payload, ok = readOne(5 * time.Second)
	if !ok || payload != "MESSAGE TWO" {
		t.Fatalf("self-heal regressed: message #2 never arrived after forcing "+
			"ErrConsumerDeleted (payload=%q ok=%v) -- the new ConsumeErrHandler "+
			"must be strictly observational, not behavior-changing", payload, ok)
	}

	// The log-only half of the contract: a "warn" event naming this
	// specific source should now be in the ring.
	found := false
	for _, ev := range d.NATSEvents.Snapshot() {
		if ev.Type == "warn" && ev.Caller == "handleRead-liveFollowConsume" {
			found = true
			if ev.Reason == "" {
				t.Errorf("recorded warn event has an empty Reason -- should carry nats.go's error string")
			}
			t.Logf("recorded diagnostic event: reason=%q", ev.Reason)
		}
	}
	if !found {
		t.Fatalf("expected a warn/handleRead-liveFollowConsume NATSEvent after forcing " +
			"ErrConsumerDeleted, found none -- ConsumeErrHandler did not fire or did not record")
	}
}
