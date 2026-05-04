package cli

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestCmdTerminalShare_BareInvocationPrefersEnvCurrentHandle(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true command not available")
	}
	t.Setenv("PPZ_SESSION", "terminal-current-test")
	t.Setenv("PPZ_CURRENT_HANDLE", "env-current")

	dir, err := os.MkdirTemp("/tmp", "ppz-terminal-current-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveTerminalCurrentDaemon(t, sock, "daemon-current")

	if err := cmdTerminalShare([]string{"--", truePath}); err != nil {
		t.Fatalf("cmdTerminalShare bare: %v", err)
	}

	if len(requests.pipeCreates) != 3 {
		t.Fatalf("pipe create request count = %d, want 3", len(requests.pipeCreates))
	}
	for _, got := range requests.pipeCreates {
		if got.Handle != "env-current" {
			t.Fatalf("terminal share provisioned pipe %q on handle %q, want env-current", got.Name, got.Handle)
		}
	}
}

type terminalCurrentRequests struct {
	pipeCreates []cliproto.PipeCreateRequest
}

func serveTerminalCurrentDaemon(t *testing.T, sock, current string) *terminalCurrentRequests {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	requests := &terminalCurrentRequests{}
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleTerminalCurrentDaemonConn(conn, current, requests)
		}
	}()

	return requests
}

func handleTerminalCurrentDaemonConn(conn net.Conn, current string, requests *terminalCurrentRequests) {
	defer conn.Close()
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	enc := json.NewEncoder(conn)
	switch req.Method {
	case cliproto.IPCStatus:
		_ = enc.Encode(map[string]any{
			"result": cliproto.StatusReply{DaemonPID: 1234, LoggedIn: true, Current: current},
		})
	case cliproto.IPCPipeCreate:
		var pc cliproto.PipeCreateRequest
		_ = json.Unmarshal(req.Params, &pc)
		requests.pipeCreates = append(requests.pipeCreates, pc)
		_ = enc.Encode(map[string]any{
			"result": cliproto.PipeCreateReply{Handle: pc.Handle, Name: pc.Name},
		})
	case cliproto.IPCBroadcast:
		var br cliproto.BroadcastRequest
		_ = json.Unmarshal(req.Params, &br)
		_ = enc.Encode(map[string]any{
			"result": cliproto.BroadcastReply{
				ID:      "test-id",
				Subject: "test." + br.Handle + "." + br.Channel,
				Bytes:   len(br.Payload),
			},
		})
	case cliproto.IPCRead:
		_, _ = io.Copy(io.Discard, conn)
	}
}
