package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// TestHandleRead_Follow_ClosedBySwapNC_DoesNotLeakGoroutine pins the
// goroutine-leak introduced by handleRead's follow mode: when swapNC calls
// Follows.closeAll() to evict a stale follow conn (e.g. on every JWT
// rotation), handleRead never exits — it blocks at <-ctx.Done() which only
// fires on daemon shutdown. One handleConn goroutine leaks per Follow per NC
// swap. Over 12 days with ~10-minute JWT rotation cadence and 2 Follow
// connections per agent, the daemon accumulates thousands of leaked goroutines.
//
// The correct behaviour: handleRead should return when the follow conn is
// closed (by the client OR by closeAll), not wait for daemon shutdown. The
// fix is to signal the outer goroutine via a done channel that the inner
// clientGone goroutine already closes when conn.Read returns.
//
// RED: with <-ctx.Done() as the sole exit, the handleConn goroutine stays
// alive after closeAll fires and the goroutine count never returns to
// baseline.
// GREEN: a "select { case <-done: case <-ctx.Done(): }" exits immediately
// when closeAll closes the conn.
func TestHandleRead_Follow_ClosedBySwapNC_DoesNotLeakGoroutine(t *testing.T) {
	// Start an embedded JetStream-capable NATS server.
	nc := startEmbeddedJS(t)

	// Build a daemon wired to that server. The socket lives under /tmp
	// (not t.TempDir) so the unix path stays under macOS's ~104-char limit.
	home, err := os.MkdirTemp("/tmp", "ppz-follow-leak-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	sockPath := filepath.Join(home, "daemon.sock")
	d := New(home, sockPath)

	// Credentials — needed by handleRead's d.State.Credentials() check.
	loginForWakeTests(t, d)

	// RefreshLoop — needed by bootstrapNATS (called from ensureNATS inside
	// handleRead). Set a long expiry so RefreshNowIfDue is a no-op and no
	// HTTP calls are made.
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

	// Wire the NC into the daemon. swapNC stamps ncExp = Refresh.JWTExp() so
	// rebuildNC's generation check coalesces and does NOT redial.
	d.NATSURL = nc.ConnectedUrl()
	d.swapNC("test-init", nc)

	// Create a JetStream stream for the pipe the Follow will subscribe to.
	// We use the uncollared (BareTarget) path so handleRead skips the HTTP
	// source-verification call.
	accountID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx := context.Background()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	// handleRead looks up the stream by natsubj.BuildStreamName — the Name
	// field must match exactly, not just the subject filter.
	manifold := d.State.CurrentNamespace("test-session") // "" for fresh state
	streamName := natsubj.BuildStreamName(accountID, manifold, "", "stdin")
	subject := natsubj.BuildSubject(accountID, manifold, "", "stdin")
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
	}); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	// Start the IPC server. Use a fresh context that we DON'T cancel early
	// (cancelling it is the one thing that would mask the bug).
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	t.Cleanup(daemonCancel)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen unix %s: %v", sockPath, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go d.serveIPC(daemonCtx, ln)

	// countHandleReadGoroutines returns how many live goroutines are inside
	// (or were spawned by) handleRead. We assert on THIS rather than
	// runtime.NumGoroutine() because the total process count is polluted by
	// nats-server internal goroutines — connecting nc2 for the swap spins up
	// server-side JSAPI/consumer goroutines that outlive our window and have
	// nothing to do with the leak under test. Counting handleRead frames
	// isolates the ppz goroutine that actually leaks: the handleConn goroutine
	// blocked at <-ctx.Done(), whose stack carries the handleRead frame.
	countHandleReadGoroutines := func() int {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		count := 0
		for _, g := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(g, "daemon.(*Daemon).handleRead") {
				count++
			}
		}
		return count
	}

	// Settle: let serveIPC's Accept goroutine, the embedded NATS server
	// goroutines, and the RefreshLoop goroutine reach steady state. No
	// handleRead is running yet, so the baseline is 0.
	runtime.Gosched()
	time.Sleep(20 * time.Millisecond)
	if base := countHandleReadGoroutines(); base != 0 {
		t.Fatalf("expected 0 handleRead goroutines before any Follow, got %d", base)
	}

	// Open a Follow IPC connection (mirroring what forwardStdin does in
	// ppz agent create / terminal share).
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
	})
	if err := json.NewEncoder(ipcConn).Encode(map[string]any{
		"method": cliproto.IPCRead,
		"params": json.RawMessage(body),
	}); err != nil {
		t.Fatalf("encode Follow request: %v", err)
	}

	// Wait until handleRead registers the conn with d.Follows, confirming
	// it has entered Follow mode. At that point the handleConn goroutine is
	// parked inside handleRead (at the <-ctx.Done / select), so
	// countHandleReadGoroutines() == 1.
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
	d.Follows.mu.Lock()
	registered := len(d.Follows.conns)
	d.Follows.mu.Unlock()
	if registered == 0 {
		t.Fatal("Follow conn was never registered in d.Follows — handleRead did not reach Follow mode")
	}
	// Let the consumer + clientGone goroutine finish spawning so handleRead
	// is fully parked at its exit-wait before the swap.
	time.Sleep(50 * time.Millisecond)
	if active := countHandleReadGoroutines(); active == 0 {
		t.Fatal("expected handleRead to be parked in Follow mode, but no handleRead goroutine is running")
	}

	// Trigger swapNC → closeAll() evicts the Follow conn, exactly as a
	// JWT rotation does in production.
	nc2, err := nats.Connect(nc.ConnectedUrl())
	if err != nil {
		t.Fatalf("connect nc2 for swap: %v", err)
	}
	t.Cleanup(func() { nc2.Close() })
	d.swapNC("test-rotation", nc2)

	// closeAll closed the follow conn, so conn.Read unblocks, the clientGone
	// goroutine calls cctx.Stop() and returns, and handleRead should exit.
	//   - CORRECT: handleRead's select <-done fires → handleConn exits → 0
	//   - BUGGY:   handleRead stays at <-ctx.Done() → handleConn leaks → 1
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if countHandleReadGoroutines() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	runtime.Gosched()

	if leaked := countHandleReadGoroutines(); leaked > 0 {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Logf("GOROUTINE DUMP:\n%s", buf[:n])
		t.Errorf(
			"handleRead leaked %d goroutine(s) after closeAll: the handleConn "+
				"goroutine blocks at <-ctx.Done() instead of exiting when the "+
				"follow conn is closed by swapNC; fix: replace <-ctx.Done() with "+
				"select{case <-done: case <-ctx.Done():} where done is closed by "+
				"the clientGone goroutine",
			leaked,
		)
	}
}
