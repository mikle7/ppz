package cli

import (
	"bytes"
	"os"
	"testing"
)

// TestStatusDoesNotCallMaybeNotifyUpdate asserts that cmdStatus no
// longer emits the standalone "update available: …" line on stderr.
// The new three-state daemon line in cliproto carries the same signal
// inline (amber state with "(update available, run 'ppz upgrade')"),
// so calling maybeNotifyUpdate() here would duplicate the notice and
// add stderr noise to `ppz status`.
//
// Source-level assertion because the function writes to os.Stderr at
// global scope — a runtime test would need the full manifest fetch
// plumbing here just to observe a no-op when the call is absent.
func TestStatusDoesNotCallMaybeNotifyUpdate(t *testing.T) {
	data, err := os.ReadFile("status.go")
	if err != nil {
		t.Fatalf("read status.go: %v", err)
	}
	if bytes.Contains(data, []byte("maybeNotifyUpdate()")) {
		t.Errorf("status.go must not call maybeNotifyUpdate() — the daemon line now carries update state; the standalone stderr notice belongs on ls/version/login")
	}
}
