package cli

import (
	"bytes"
	"os"
	"testing"
)

// TestDaemonGroupDispatchesRestart asserts that `ppz daemon restart`
// is wired through cmdDaemonGroup to cmdDaemonRestart. Source-level
// because the behavioural cycle (stop running daemon → fork a fresh
// one) requires a real binary on PATH; we cover the end-to-end in the
// docker-compose `tests/daemon/daemon-restart-cycles-pid` scenario.
func TestDaemonGroupDispatchesRestart(t *testing.T) {
	data, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("read daemon.go: %v", err)
	}
	if !bytes.Contains(data, []byte("cmdDaemonRestart")) {
		t.Errorf("daemon.go must dispatch the 'restart' subverb to cmdDaemonRestart")
	}
	if !bytes.Contains(data, []byte(`"restart"`)) {
		t.Errorf("daemon.go must recognise the 'restart' subverb in its switch")
	}
}
