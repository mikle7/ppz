package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestCmdCommand_DefaultSequenceIsNewline(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost", "ls -la"}); err != nil {
		t.Fatalf("cmdCommand: %v", err)
	}

	if len(*reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "ls -la")
	assertStdinRequest(t, (*reqs)[1], "myhost", "\n")
}

func TestCmdCommand_NoInstructionSendsOnlyCtrlSeq(t *testing.T) {
	reqs := setupCommandDaemon(t)

	if err := cmdCommand([]string{"myhost"}); err != nil {
		t.Fatalf("cmdCommand no instruction: %v", err)
	}

	if len(*reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(*reqs))
	}
	assertStdinRequest(t, (*reqs)[0], "myhost", "\n")
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

func setupCommandDaemon(t *testing.T) *[]cliproto.BroadcastRequest {
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

func assertStdinRequest(t *testing.T, req cliproto.BroadcastRequest, handle, payload string) {
	t.Helper()
	if req.Handle != handle || req.Channel != "stdin" || req.Payload != payload {
		t.Errorf("request: handle=%q channel=%q payload=%q, want handle=%q channel=stdin payload=%q",
			req.Handle, req.Channel, req.Payload, handle, payload)
	}
}
