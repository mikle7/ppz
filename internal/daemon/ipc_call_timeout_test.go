package daemon

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// TestCall_DoesNotHangWhenDaemonAcceptsButNeverReplies is the unit-level
// reproduction of the production "ppz send hangs forever" report
// (copilot/alex session, 2026-05-26): right after a daemon restart, send
// hung for >2 minutes with no output, no error, and a still-zero exit,
// while `ppz read` kept working.
//
// The defect is that the CLI's daemon.Call sets NO read deadline on the
// IPC connection: net.Dial over the unix socket succeeds the moment the
// daemon *accepts*, then dec.Decode(&resp) blocks indefinitely if no
// reply ever comes. So the CLI has zero self-protection against a daemon
// that accepts a connection but is slow to (or never does) reply — e.g.
// a daemon still in its own restart/startup window before it serves IPC
// replies. (e2e fault-injection confirmed the server-down/-frozen paths
// themselves fast-fail with E_NATS_UNREACHABLE within ~5s, guarded by
// the daemon's 5s HTTP client and the pre-publish JetStream check; the
// unbounded wait lives in the IPC client, which this test pins.)
//
// The test stands up a fake daemon that accepts the connection then goes
// silent and asserts that Call RETURNS (with an error) rather than
// blocking — the contract "the IPC client must bound its own wait so a
// stuck daemon can never hang the CLI".
//
// RED: with no deadline in Call, the goroutine never sends on done and we
// fall through to the timeout branch and fail.
// GREEN: a read deadline (or context) in Call makes it return an error
// within the bound.
func TestCall_DoesNotHangWhenDaemonAcceptsButNeverReplies(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-ipc-hang-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// Accept the connection but never reply — model a daemon whose
	// handler is wedged inside NC.Flush() during a server outage. Hold
	// the conn open (don't close) so the client sees a live-but-silent
	// peer, not EOF.
	held := make(chan net.Conn, 1)
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		held <- conn // keep it alive; closing would let Decode return early
	}()
	t.Cleanup(func() {
		select {
		case c := <-held:
			_ = c.Close()
		default:
		}
	})

	// Pin a short client deadline so the test is fast and deterministic:
	// Call must bound its own wait (the fix) and return an error rather
	// than blocking. The ceiling is comfortably above the deadline so a
	// correctly-bounded Call lands inside it, while a never-returning
	// Call (no deadline — the bug) is caught instead of wedging the
	// whole test binary.
	prev := ipcCallTimeout
	ipcCallTimeout = 1 * time.Second
	t.Cleanup(func() { ipcCallTimeout = prev })
	const ceiling = 5 * time.Second

	done := make(chan error, 1)
	go func() {
		var reply cliproto.SendReply
		done <- Call(sock, cliproto.IPCSend,
			cliproto.SendRequest{Handle: "james", Channel: "inbox", Payload: "hi"},
			&reply)
	}()

	select {
	case err := <-done:
		// Returning is the whole point — but the contract is specifically
		// that the bounded wait surfaces as E_DAEMON_TIMEOUT (exit 26), so
		// pin the code. A regression that returned a different error
		// (e.g. E_DAEMON_NOT_RUNNING, or a wrapped net.Error) would
		// otherwise stay green here.
		if err == nil {
			t.Fatalf("Call returned nil error, but the fake daemon never sent a reply; expected E_DAEMON_TIMEOUT")
		}
		var ce *cliproto.Error
		if !errors.As(err, &ce) || ce.Code != cliproto.EDaemonTimeout {
			t.Fatalf("Call returned %v, want *cliproto.Error{Code: %s}", err, cliproto.EDaemonTimeout)
		}
	case <-time.After(ceiling):
		t.Fatalf("ppz send hung: daemon.Call did not return within %s when the daemon accepted but never replied (no read deadline on the IPC connection)", ceiling)
	}
}

// TestCallWait_BlocksPastTimeout pins the Call→CallWait wiring contract:
// the blocking-wait verbs (IPCListWatch, IPCSubsWait) must use CallWait,
// which sets NO read deadline, so a daemon that legitimately holds the
// connection open until a NATS event arrives (well past ipcCallTimeout)
// is waited on rather than aborted with E_DAEMON_TIMEOUT.
//
// The fake daemon accepts, then replies only after replyDelay — longer
// than the (tiny) ipcCallTimeout pinned here. Call() would time out;
// CallWait() must instead receive the reply. A regression that pointed
// ls --watch / subs wait back at Call (or that made CallWait honor the
// deadline) fails here.
func TestCallWait_BlocksPastTimeout(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-ipc-wait-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	const replyDelay = 200 * time.Millisecond
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		defer conn.Close()
		// Drain the request line, then stall past the deadline before
		// sending a valid empty reply.
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		time.Sleep(replyDelay)
		_, _ = conn.Write([]byte(`{"result":{}}` + "\n"))
	}()

	// Deadline far below the daemon's reply delay: a deadline-bound Call
	// would fire E_DAEMON_TIMEOUT well before the reply lands.
	prev := ipcCallTimeout
	ipcCallTimeout = 20 * time.Millisecond
	t.Cleanup(func() { ipcCallTimeout = prev })
	const ceiling = 5 * time.Second

	done := make(chan error, 1)
	go func() {
		var reply cliproto.ListReply
		done <- CallWait(sock, cliproto.IPCSubsWait,
			cliproto.SubsWaitRequest{Session: "sid-1"}, &reply)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CallWait returned %v; it must block past ipcCallTimeout (%s) and receive the delayed reply, not time out", err, ipcCallTimeout)
		}
	case <-time.After(ceiling):
		t.Fatalf("CallWait did not return within %s even though the daemon replied after %s", ceiling, replyDelay)
	}
}
