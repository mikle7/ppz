package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestCmdSend_BareHandleTargetsInbox(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-send-inbox-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveSendAliasDaemon(t, sock)

	if err := cmdSend([]string{"foo", "hello inbox"}); err != nil {
		t.Fatalf("cmdSend bare handle: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("broadcast request count = %d, want 1", len(*requests))
	}
	got := (*requests)[0]
	if got.Handle != "foo" || got.Channel != "inbox" || got.Payload != "hello inbox" {
		t.Fatalf("bare send resolved to handle=%q channel=%q payload=%q, want foo.inbox hello inbox",
			got.Handle, got.Channel, got.Payload)
	}
}

// `ppz send` MUST forward the calling shell's session id so the daemon's
// envelope.sender = d.State.Current(req.Session) resolves to the per-tty
// current source. Pre-fix, send.go omitted Session entirely; the daemon
// then read d.State.Current("") which normalises to the "default" session
// — almost always empty when the user actually has a current set on their
// real tty session — and stamped sender="" on every published envelope.
//
// Reproduction in the wild: `ppz source create zues; ppz send quux "hi"`
// from one terminal arrived on the receiver with sender="" instead of
// sender="zues".
func TestCmdSend_ForwardsSessionID(t *testing.T) {
	t.Setenv("PPZ_SESSION", "tty-send-session-test")
	dir, err := os.MkdirTemp("/tmp", "ppz-send-session-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveSendAliasDaemon(t, sock)

	if err := cmdSend([]string{"quux", "hi"}); err != nil {
		t.Fatalf("cmdSend: %v", err)
	}
	if len(*requests) != 1 {
		t.Fatalf("broadcast request count = %d, want 1", len(*requests))
	}
	got := (*requests)[0]
	if got.Session != "tty-send-session-test" {
		t.Fatalf("BroadcastRequest.Session = %q, want %q (without it the daemon stamps sender=\"\")",
			got.Session, "tty-send-session-test")
	}
}

func serveSendAliasDaemon(t *testing.T, sock string) *[]cliproto.BroadcastRequest {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	var broadcastRequests []cliproto.BroadcastRequest
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
			if req.Method == cliproto.IPCBroadcast {
				var br cliproto.BroadcastRequest
				_ = json.Unmarshal(req.Params, &br)
				broadcastRequests = append(broadcastRequests, br)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.BroadcastReply{
						ID:      "test-id",
						Subject: "org.foo.inbox",
						Bytes:   len(br.Payload),
					},
				})
			}
			_ = conn.Close()
		}
	}()

	return &broadcastRequests
}
