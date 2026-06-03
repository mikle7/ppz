package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Phase 1.5.1: bare LEAF in `ppz pipe create` / `ppz pipe destroy` no
// longer auto-collars under the current handle. The CLI forwards
// handle="" so the daemon takes the uncollared path at the session's
// current namespace. Renamed from BareNamePrefersEnvCurrentHandle —
// the pre-1.5.1 behaviour the original name pinned is intentionally
// gone (see PHASE-1.5-IMPLEMENTATION-PLAN.md in pipes-internal).

func TestCmdPipeCreate_BareNameSkipsCurrentHandle(t *testing.T) {
	t.Setenv("PPZ_SESSION", "pipe-current-test")
	t.Setenv("PPZ_CURRENT_HANDLE", "env-current")
	sock := pipeCurrentTestSocket(t)
	requests := servePipeCurrentDaemon(t, sock, "daemon-current")

	if err := cmdPipeCreate([]string{"alerts"}); err != nil {
		t.Fatalf("cmdPipeCreate: %v", err)
	}

	if requests.creates.count() != 1 {
		t.Fatalf("pipe create request count = %d, want 1", requests.creates.count())
	}
	got := requests.creates.at(0)
	if got.Handle != "" || got.Name != "alerts" {
		t.Fatalf("pipe create resolved to handle=%q name=%q, want empty handle + name=alerts (Phase 1.5.1 rule: bare LEAF goes uncollared regardless of current handle)", got.Handle, got.Name)
	}
}

func TestCmdPipeDestroy_BareNameSkipsCurrentHandle(t *testing.T) {
	t.Setenv("PPZ_SESSION", "pipe-current-test")
	t.Setenv("PPZ_CURRENT_HANDLE", "env-current")
	sock := pipeCurrentTestSocket(t)
	requests := servePipeCurrentDaemon(t, sock, "daemon-current")

	if err := cmdPipeDestroy([]string{"alerts"}); err != nil {
		t.Fatalf("cmdPipeDestroy: %v", err)
	}

	if requests.destroys.count() != 1 {
		t.Fatalf("pipe destroy request count = %d, want 1", requests.destroys.count())
	}
	got := requests.destroys.at(0)
	if got.Handle != "" || got.BareTarget != "alerts" {
		t.Fatalf("pipe destroy resolved to handle=%q bareTarget=%q, want empty handle + bareTarget=alerts (Phase 1.5.1 rule: bare LEAF destroys uncollared)", got.Handle, got.BareTarget)
	}
}

func pipeCurrentTestSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ppz-pipe-current-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	return sock
}

type pipeCurrentRequests struct {
	creates  recorder[cliproto.PipeCreateRequest]
	destroys recorder[cliproto.PipeDestroyRequest]
}

func servePipeCurrentDaemon(t *testing.T, sock, current string) *pipeCurrentRequests {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	requests := &pipeCurrentRequests{}
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
			var req struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = conn.Close()
				continue
			}
			switch req.Method {
			case cliproto.IPCStatus:
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.StatusReply{DaemonPID: 1234, LoggedIn: true, Current: current},
				})
			case cliproto.IPCPipeCreate:
				var pc cliproto.PipeCreateRequest
				_ = json.Unmarshal(req.Params, &pc)
				requests.creates.add(pc)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.PipeCreateReply{Handle: pc.Handle, Name: pc.Name},
				})
			case cliproto.IPCPipeDestroy:
				var pd cliproto.PipeDestroyRequest
				_ = json.Unmarshal(req.Params, &pd)
				requests.destroys.add(pd)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.PipeDestroyReply{Handle: pd.Handle, Name: pd.Name},
				})
			}
			_ = conn.Close()
		}
	}()

	return requests
}
