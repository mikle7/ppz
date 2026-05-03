package cli

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestRunRead_BareInboxResolvesToCurrentSourceInbox(t *testing.T) {
	t.Setenv("PPZ_SESSION", "read-inbox-test")
	dir, err := os.MkdirTemp("/tmp", "ppz-read-inbox-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveReadInboxAliasDaemon(t, sock, "foo")

	if err := runRead("inbox", true, false, false, false, false, 0, 0, 0); err != nil {
		t.Fatalf("runRead inbox: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("read request count = %d, want 1", len(*requests))
	}
	got := (*requests)[0]
	if got.Handle != "foo" || got.Channel != "inbox" {
		t.Fatalf("read inbox resolved to %q.%q, want foo.inbox", got.Handle, got.Channel)
	}
	if got.Session != "read-inbox-test" {
		t.Fatalf("read session = %q, want read-inbox-test", got.Session)
	}
}

func TestRunRead_BareInboxWithoutCurrentSourceErrors(t *testing.T) {
	t.Setenv("PPZ_SESSION", "read-inbox-test")
	dir, err := os.MkdirTemp("/tmp", "ppz-read-inbox-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	_ = serveReadInboxAliasDaemon(t, sock, "")

	readErr := runRead("inbox", true, false, false, false, false, 0, 0, 0)
	var pErr *cliproto.Error
	if !errors.As(readErr, &pErr) || pErr.Code != cliproto.ENoCurrentSource {
		t.Fatalf("runRead inbox without current error = %#v, want %s", readErr, cliproto.ENoCurrentSource)
	}
}

func serveReadInboxAliasDaemon(t *testing.T, sock, current string) *[]cliproto.ReadRequest {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	var readRequests []cliproto.ReadRequest
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
			case cliproto.IPCRead:
				var rr cliproto.ReadRequest
				_ = json.Unmarshal(req.Params, &rr)
				readRequests = append(readRequests, rr)
			}
			_ = conn.Close()
		}
	}()

	return &readRequests
}
