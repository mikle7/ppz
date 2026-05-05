package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// CLI dispatch tests for `ppz org list|switch|create|invite`. Each
// test stands up a fake daemon socket, runs the CLI, and asserts the
// IPC method + params the daemon received.

func TestOrg_ListDispatchesIPCOrgList(t *testing.T) {
	sock := tmpDaemonSock(t, "ppz-org-list-")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	requests := serveOrgFakeDaemon(t, sock, map[string]any{
		cliproto.IPCOrgList: cliproto.ListOrgsReply{Orgs: []cliproto.OrgInfo{
			{ID: "id-a", Name: "alpha", Role: "owner"},
			{ID: "id-b", Name: "beta", Role: "member"},
		}},
	})

	if err := cmdOrg([]string{"list"}); err != nil {
		t.Fatalf("cmdOrg list: %v", err)
	}

	if got := (*requests)[0].Method; got != cliproto.IPCOrgList {
		t.Errorf("method: got %q want %q", got, cliproto.IPCOrgList)
	}
}

func TestOrg_SwitchDispatchesIPCOrgSwitch(t *testing.T) {
	sock := tmpDaemonSock(t, "ppz-org-switch-")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	requests := serveOrgFakeDaemon(t, sock, map[string]any{
		cliproto.IPCOrgSwitch: cliproto.OrgSwitchReply{OrgID: "id-b", OrgName: "beta"},
	})

	if err := cmdOrg([]string{"switch", "beta"}); err != nil {
		t.Fatalf("cmdOrg switch: %v", err)
	}

	r := (*requests)[0]
	if r.Method != cliproto.IPCOrgSwitch {
		t.Errorf("method: got %q want %q", r.Method, cliproto.IPCOrgSwitch)
	}
	var sw cliproto.OrgSwitchRequest
	_ = json.Unmarshal(r.Params, &sw)
	if sw.Name != "beta" {
		t.Errorf("params.Name: got %q want %q", sw.Name, "beta")
	}
}

func TestOrg_CreateDispatchesIPCOrgCreate(t *testing.T) {
	sock := tmpDaemonSock(t, "ppz-org-create-")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	requests := serveOrgFakeDaemon(t, sock, map[string]any{
		cliproto.IPCOrgCreate: cliproto.CreateOrgReply{ID: "id-c", Name: "gamma"},
	})

	if err := cmdOrg([]string{"create", "gamma"}); err != nil {
		t.Fatalf("cmdOrg create: %v", err)
	}

	r := (*requests)[0]
	if r.Method != cliproto.IPCOrgCreate {
		t.Errorf("method: got %q want %q", r.Method, cliproto.IPCOrgCreate)
	}
	var c cliproto.OrgCreateRequest
	_ = json.Unmarshal(r.Params, &c)
	if c.Name != "gamma" {
		t.Errorf("params.Name: got %q want %q", c.Name, "gamma")
	}
}

func TestOrg_InviteDispatchesIPCOrgInvite(t *testing.T) {
	sock := tmpDaemonSock(t, "ppz-org-invite-")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	requests := serveOrgFakeDaemon(t, sock, map[string]any{
		cliproto.IPCOrgInvite: cliproto.CreateInviteReply{Invite: cliproto.Invite{
			ID:               "inv-1",
			InviteeUsername:  "alice",
			OrganisationName: "alpha",
		}},
	})

	if err := cmdOrg([]string{"invite", "alice"}); err != nil {
		t.Fatalf("cmdOrg invite: %v", err)
	}

	r := (*requests)[0]
	if r.Method != cliproto.IPCOrgInvite {
		t.Errorf("method: got %q want %q", r.Method, cliproto.IPCOrgInvite)
	}
	var inv cliproto.OrgInviteRequest
	_ = json.Unmarshal(r.Params, &inv)
	if inv.Username != "alice" {
		t.Errorf("params.Username: got %q want %q", inv.Username, "alice")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

type capturedIPCRequest struct {
	Method string
	Params json.RawMessage
}

func tmpDaemonSock(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", prefix)
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "daemon.sock")
}

// serveOrgFakeDaemon listens on sock and replies to one request per
// connection from the replyByMethod map. It captures every request
// for assertion in the parent test.
func serveOrgFakeDaemon(t *testing.T, sock string, replyByMethod map[string]any) *[]capturedIPCRequest {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	var requests []capturedIPCRequest
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
			requests = append(requests, capturedIPCRequest{Method: req.Method, Params: req.Params})
			result, ok := replyByMethod[req.Method]
			if !ok {
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"error": map[string]string{"code": "E_PROTOCOL", "message": "unhandled in test"},
				})
			} else {
				_ = json.NewEncoder(conn).Encode(map[string]any{"result": result})
			}
			_ = conn.Close()
		}
	}()

	return &requests
}
