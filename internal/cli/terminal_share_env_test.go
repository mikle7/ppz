package cli

import (
	"testing"
)

// TestTerminalShareEnv_ExportsPPZSession asserts that the child environment
// built for `ppz terminal share` includes PPZ_SESSION=<handle>. Without it,
// subprocesses spawned inside the pty (e.g. by an AI agent calling ppz read
// via a shell that uses setsid) get a fresh Unix session id and therefore a
// fresh cursor — so read calls never advance the cursor that subsequent ls
// or read calls observe.
func TestTerminalShareEnv_ExportsPPZSession(t *testing.T) {
	env := terminalShareEnv("my-agent")
	if !envContains(env, "PPZ_SESSION=my-agent") {
		t.Errorf("terminalShareEnv missing PPZ_SESSION=my-agent; got %v", env)
	}
}

// TestTerminalShareEnv_ExportsPPZCurrentHandle asserts the existing contract
// is preserved alongside the new PPZ_SESSION.
func TestTerminalShareEnv_ExportsPPZCurrentHandle(t *testing.T) {
	env := terminalShareEnv("my-agent")
	if !envContains(env, "PPZ_CURRENT_HANDLE=my-agent") {
		t.Errorf("terminalShareEnv missing PPZ_CURRENT_HANDLE=my-agent; got %v", env)
	}
}

func envContains(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}
