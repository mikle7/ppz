package daemon

import (
	"net"
	"testing"
	"time"
)

// TestReportNATSFailure_EvictsLiveFollows pins the fix for the
// "terminal-share relay forwards once, then silently stops" report
// (mikle7/ppz#1). reportNATSFailure closes the shared NATS conn directly
// when a JetStream op times out on it (the "zombie connection" case) —
// but unlike swapNCLocked, which always calls d.Follows.closeAll() before
// replacing/closing the NC, reportNATSFailure closed the NC and left any
// live Follow (e.g. `ppz terminal share`'s forwardStdin) registered.
//
// Consequence: the JetStream OrderedConsumer backing that Follow observes
// the NC close as ErrConnectionClosed, which nats.go's ordered consumer
// treats as terminal — it stops itself with no self-heal (unlike every
// other recovery trigger, which resets). Nothing tells the CLI-facing IPC
// socket to close, so the CLI's Follow-reader blocks forever with no EOF
// and no error: no redial, no crash, just silence after whatever was
// delivered before the close. A later send can still succeed (it opens
// its own fresh JetStream context against whatever NC is current by
// then), which is why the daemon-side send kept reporting success while
// the relay went quiet.
//
// RED (pre-fix): reportNATSFailure closes the NC but leaves the Follow
// conn registered and open — the CLI would never see EOF.
// GREEN (post-fix): reportNATSFailure evicts every registered Follow the
// same way swapNCLocked does, so the CLI observes EOF and redials.
func TestReportNATSFailure_EvictsLiveFollows(t *testing.T) {
	nc := startEmbeddedJS(t)

	d := New(t.TempDir(), "")
	d.Follows = newFollowRegistry()
	d.swapNC("test-init", nc)

	// Stand in for a live `Follow: true` IPC connection without needing
	// the full JetStream OrderedConsumer/stdin-relay machinery — handleRead
	// registers the raw net.Conn it was handed, so a bare net.Pipe half
	// exercises the exact same eviction path.
	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() { _ = clientSide.Close() })
	d.Follows.add(serverSide)

	d.reportNATSFailure()

	// net.Pipe is synchronous, so a Close() on serverSide unblocks a
	// pending Read on clientSide immediately if it happened; give the
	// (now synchronous) eviction a short window either way.
	_ = clientSide.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err := clientSide.Read(buf)

	if err == nil {
		t.Fatalf("expected the Follow conn to be closed by reportNATSFailure, got a live read")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("reportNATSFailure left a live Follow conn open (no EOF within 500ms) — " +
			"the CLI's redial loop would never fire, reproducing mikle7/ppz#1")
	}

	d.Follows.mu.Lock()
	n := len(d.Follows.conns)
	d.Follows.mu.Unlock()
	if n != 0 {
		t.Fatalf("Follows registry still has %d conn(s) after reportNATSFailure, want 0", n)
	}
}
