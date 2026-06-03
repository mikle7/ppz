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

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(0), "myhost", "ls -la")
	assertStdinRequest(t, reqs.at(1), "myhost", "\r")
}

func TestCmdCommand_NoInstructionSendsOnlyCtrlSeq(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost"}); err != nil {
		t.Fatalf("cmdCommand no instruction: %v", err)
	}

	if reqs.count() != 1 {
		t.Fatalf("want 1 request, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(0), "myhost", "\r")
}

func TestCmdCommand_ClaudeFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--claude", "myhost", "do something"}); err != nil {
		t.Fatalf("cmdCommand --claude: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(0), "myhost", "do something")
	assertStdinRequest(t, reqs.at(1), "myhost", "\x1b[13u")
}

func TestCmdCommand_CRFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--cr", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --cr: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(1), "myhost", "\r")
}

func TestCmdCommand_CRLFFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--crlf", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --crlf: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(1), "myhost", "\r\n")
}

func TestCmdCommand_NewlineFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--newline", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --newline: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(1), "myhost", "\n")
}

func TestCmdCommand_NoneFlag(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"--none", "myhost", "ping"}); err != nil {
		t.Fatalf("cmdCommand --none: %v", err)
	}

	if reqs.count() != 1 {
		t.Fatalf("want 1 request (instruction only, no ctrl seq), got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(0), "myhost", "ping")
}

func TestCmdCommand_ClaudeFlagAfterPositionalArgs(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "do something", "--claude"}); err != nil {
		t.Fatalf("cmdCommand flag after positional args: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 requests, got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(1), "myhost", "\x1b[13u")
}

func TestCmdCommand_NoneFlagAfterPositionalArgs(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "ping", "--none"}); err != nil {
		t.Fatalf("cmdCommand --none after positional args: %v", err)
	}

	if reqs.count() != 1 {
		t.Fatalf("want 1 request (instruction only), got %d", reqs.count())
	}
	assertStdinRequest(t, reqs.at(0), "myhost", "ping")
}

func TestCmdCommand_FlagAfterPositionalArgsErrors(t *testing.T) {
	// Go's flag package stops at the first non-flag arg; flags that appear
	// after positional args would otherwise be silently ignored.
	err := cmdCommand([]string{"blue", "hello world", "--unknown"})
	if err == nil {
		t.Fatal("expected error for flag after positional args, got nil")
	}
}

// TestCmdCommand_StampsSenderFromEnvInsideSharedTerminal repros the
// same shared-pty sender-empty bug for `ppz command`, the sibling
// IPC-send verb. `ppz command H "instr"` issues TWO SendRequests
// (instruction, then terminator) via the same daemon.Call(IPCSend,
// …) shape `ppz send` uses, and both today omit any Sender hint —
// so an inner-shell `ppz command other-agent "do X"` lands on the
// receiver's stdin pipe with envelope.sender="" even though every
// other verb agrees you ARE the wrapped handle.
//
// Pinning here ensures the GREEN fix wires both sites symmetrically;
// fixing only `ppz send` would leave `ppz command` regressed.
//
// RED today.
func TestCmdCommand_StampsSenderFromEnvInsideSharedTerminal(t *testing.T) {
	t.Setenv("PPZ_SESSION", "jimmy")
	t.Setenv("PPZ_CURRENT_HANDLE", "jimmy")

	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"other-agent", "do X"}); err != nil {
		t.Fatalf("cmdCommand: %v", err)
	}

	if reqs.count() != 2 {
		t.Fatalf("want 2 SendRequests (instruction + terminator), got %d", reqs.count())
	}
	// Both the instruction send AND the trailing-control-sequence send
	// must carry the hint — `ppz command` does them as two distinct IPC
	// calls (see command.go:93 — the 100ms pause that makes copilot/
	// codex/agy treat the terminator as submit), and an asymmetric fix
	// would stamp sender on one and "" on the other.
	if reqs.at(0).Sender != "jimmy" {
		t.Fatalf("instruction SendRequest.Sender = %q, want %q — `ppz command` must forward PPZ_CURRENT_HANDLE for the same reasons `ppz send` does",
			reqs.at(0).Sender, "jimmy")
	}
	if reqs.at(1).Sender != "jimmy" {
		t.Fatalf("terminator SendRequest.Sender = %q, want %q — both IPC sends must carry the hint; receiver's stdin pipe sees two envelopes",
			reqs.at(1).Sender, "jimmy")
	}
}

func setupCommandDaemon(t *testing.T) *recorder[cliproto.SendRequest] {
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
