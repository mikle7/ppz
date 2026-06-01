package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// TestCmdCommand_DefaultSequenceIsCR pins the default terminator at `\r`
// (a real keyboard Enter byte). Pre-v0.36 the default was `\n`, which
// every modern TUI input box (codex/copilot/agy/pi) treated as a literal
// newline inside the prompt rather than a submit — see the doc comment
// on cmdCommand.
func TestCmdCommand_DefaultSequenceIsCR(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "ls -la"}); err != nil {
		t.Fatalf("cmdCommand: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "ls -la")
	assertStdinRequest(t, (*reqs)[1], "myhost", "\r")
}

func TestCmdCommand_NoInstructionSendsOnlyCtrlSeq(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost"}); err != nil {
		t.Fatalf("cmdCommand no instruction: %v", err)
	}

	if len(*reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "\r")
}

func TestCmdCommand_ClaudeFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--claude", "myhost", "do something"}); err != nil {
		t.Fatalf("cmdCommand --claude: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "do something")
	assertStdinRequest(t, (*reqs)[1], "myhost", "\x1b[13u")
}

func TestCmdCommand_CRFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--cr", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --cr: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[1], "myhost", "\r")
}

func TestCmdCommand_CRLFFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--crlf", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --crlf: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[1], "myhost", "\r\n")
}

func TestCmdCommand_NewlineFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--newline", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --newline: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[1], "myhost", "\n")
}

func TestCmdCommand_NoneFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--none", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --none: %v", err)
	}

	if len(*reqs) != 1 {
		t.Fatalf("want 1 request (instruction only, no ctrl seq), got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "ping")
}

func TestCmdCommand_ClaudeFlagAfterPositionalArgs(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "do something", "--claude"}); err != nil {
		t.Fatalf("cmdCommand flag after positional args: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[1], "myhost", "\x1b[13u")
}

func TestCmdCommand_NoneFlagAfterPositionalArgs(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "ping", "--none"}); err != nil {
		t.Fatalf("cmdCommand --none after positional args: %v", err)
	}

	if len(*reqs) != 1 {
		t.Fatalf("want 1 request (instruction only), got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "ping")
}

func TestCmdCommand_FlagAfterPositionalArgsErrors(t *testing.T) {
	// Go's flag package stops at the first non-flag arg; flags that appear
	// after positional args would otherwise be silently ignored.
	err := cmdCommand([]string{"blue", "hello world", "--unknown"})
	if err == nil {
		t.Fatal("expected error for flag after positional args, got nil")
	}
}

func setupCommandDaemon(t *testing.T) *[]cliproto.SendRequest {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ppz-command-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	return serveSendAliasDaemon(t, sock)
}

func assertStdinRequest(t *testing.T, req cliproto.SendRequest, handle, payload string) {
	t.Helper()
	if req.Handle != handle || req.Channel != "stdin" || req.Payload != payload {
		t.Errorf("request: handle=%q channel=%q payload=%q, want handle=%q channel=stdin payload=%q",
			req.Handle, req.Channel, req.Payload, handle, payload)
	}
}
