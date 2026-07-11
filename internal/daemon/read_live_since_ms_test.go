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

// TestHandleRead_Follow_FiltersOldMessagesEvenOnLivePath guards the second
// half of the stdin-replay fix: a message delivered through the LIVE follow
// consumer (not the initial historical drain) whose CreatedAt predates the
// caller's SinceMS cutoff must still be dropped. Without this, an
// OrderedConsumer that has to internally recover (NATS reconnect,
// consumer-deleted-due-to-inactivity, etc.) can redeliver old retained
// messages through the live path, which historically had no age filter at
// all — bypassing the historical-drain-time SinceMS filter entirely.
func TestHandleRead_Follow_FiltersOldMessagesEvenOnLivePath(t *testing.T) {
	nc := startEmbeddedJS(t)

	home, err := os.MkdirTemp("/tmp", "ppz-live-sincems-")
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

	// Open the Follow request BEFORE anything is published, with a small
	// SinceMS — mirrors forwardStdin anchoring its cutoff to its own start
	// time. Stream is empty, so nothing is drained historically; both
	// messages below arrive purely through the live path.
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

	// Publish an OLD message (CreatedAt well before the cutoff) directly to
	// the stream — simulates the live consumer redelivering something from
	// before this request's SinceMS window, e.g. after an internal recovery.
	oldMsg := envelope.New("someone", "", "OLD REPLAYED CONTENT", time.Now().Add(-time.Hour))
	oldBytes, err := oldMsg.Marshal()
	if err != nil {
		t.Fatalf("marshal old envelope: %v", err)
	}
	if _, err := js.Publish(ctx, subject, oldBytes); err != nil {
		t.Fatalf("publish old message: %v", err)
	}

	// Publish a genuinely NEW message right after — this one must come
	// through, proving the filter drops old messages specifically and
	// isn't just swallowing the live stream wholesale.
	newMsg := envelope.New("someone", "", "GENUINELY NEW", time.Now())
	newBytes, err := newMsg.Marshal()
	if err != nil {
		t.Fatalf("marshal new envelope: %v", err)
	}
	if _, err := js.Publish(ctx, subject, newBytes); err != nil {
		t.Fatalf("publish new message: %v", err)
	}
	_ = stream // referenced only for CreateStream's return value above

	// Read whatever the daemon sends back within the window.
	_ = ipcConn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	dec := json.NewDecoder(ipcConn)
	var payloads []string
	for {
		var evt cliproto.ReadEvent
		if err := dec.Decode(&evt); err != nil {
			break
		}
		if evt.Message != nil {
			payloads = append(payloads, evt.Message.Payload)
		}
	}

	for _, p := range payloads {
		if p == "OLD REPLAYED CONTENT" {
			t.Errorf("live-follow path delivered a message older than SinceMS cutoff: %q (payloads received: %v)", p, payloads)
		}
	}
	found := false
	for _, p := range payloads {
		if p == "GENUINELY NEW" {
			found = true
		}
	}
	if !found {
		t.Errorf("live-follow path did not deliver the genuinely new message; payloads received: %v", payloads)
	}
}
