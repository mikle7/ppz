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

	if requests.count() != 1 {
		t.Fatalf("broadcast request count = %d, want 1", requests.count())
	}
	got := requests.at(0)
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
	if requests.count() != 1 {
		t.Fatalf("broadcast request count = %d, want 1", requests.count())
	}
	got := requests.at(0)
	if got.Session != "tty-send-session-test" {
		t.Fatalf("SendRequest.Session = %q, want %q (without it the daemon stamps sender=\"\")",
			got.Session, "tty-send-session-test")
	}
}

// TestCmdSend_StampsSenderFromEnvInsideSharedTerminal repros the
// shared-terminal "sender=empty" bug observed in the wild:
//
//	$ ppz terminal share jimmy                  # outer shell
//	jimmy@... %                                  # inner pty shell
//	jimmy@... % ppz send eric "hi"               # arrives at eric with sender=""
//	jimmy@... % ppz read eric.inbox --json
//	{"id":"...","sender":"","payload":"hi",...}
//
// What's wired today:
//   - `ppz terminal share H` exports PPZ_CURRENT_HANDLE=H + PPZ_SESSION=H
//     into the wrapped shell (terminalShareEnv in terminal.go).
//   - The daemon's IPCCreate deliberately skips SetCurrent for PTY-kind
//     sources, so State.Current("jimmy") is "" even though the env says
//     the current handle is jimmy. `ppz status` reads PPZ_CURRENT_HANDLE
//     direct from env and reports "current source: jimmy" — but
//     `ppz send` forwards only Session=jimmy, and the daemon stamps
//     envelope.sender = State.Current("jimmy") = "".
//
// Fix shape pinned by this test: cmdSend resolves the effective current
// handle (env override or daemon state) and forwards it as
// SendRequest.Sender so the daemon has an explicit hint when its own
// per-session state is empty. PPZ_CURRENT_HANDLE is the source of
// truth inside a wrapped pty; the CLI is the only place that env var
// can be observed.
func TestCmdSend_StampsSenderFromEnvInsideSharedTerminal(t *testing.T) {
	// Mirror the inner-shell env that `ppz terminal share jimmy` exports.
	t.Setenv("PPZ_SESSION", "jimmy")
	t.Setenv("PPZ_CURRENT_HANDLE", "jimmy")

	dir, err := os.MkdirTemp("/tmp", "ppz-send-shared-tty-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveSendAliasDaemon(t, sock)

	if err := cmdSend([]string{"eric", "hi"}); err != nil {
		t.Fatalf("cmdSend: %v", err)
	}
	if requests.count() != 1 {
		t.Fatalf("SendRequest count = %d, want 1", requests.count())
	}
	got := requests.at(0)
	if got.Sender != "jimmy" {
		t.Fatalf("SendRequest.Sender = %q, want %q — PPZ_CURRENT_HANDLE=jimmy must be forwarded as the sender hint so the daemon can stamp envelope.sender even when its per-session State.Current is empty (the shared-pty case: IPCCreate skips SetCurrent for PTY-kind sources)",
			got.Sender, "jimmy")
	}
}

// TestCmdSend_NoEnvCurrent_OmitsSenderHint regression-pins the no-env
// path: when PPZ_CURRENT_HANDLE is unset, cmdSend must NOT proactively
// query the daemon to populate Sender — the daemon's per-session
// State.Current already handles that fallback at stamp time, and
// adding an extra IPC roundtrip just to "be explicit" would slow
// every send for no behavioural gain (and would couple Sender
// forwarding to daemon reachability in non-shared sessions).
//
// GREEN today and post-fix; here to lock the precedence boundary so a
// future "always populate Sender" refactor doesn't sneak in.
func TestCmdSend_NoEnvCurrent_OmitsSenderHint(t *testing.T) {
	// Defensive: explicitly clear in case a parent test leaked it. The
	// scope of t.Setenv("", "") is a single value; an Unsetenv via
	// t.Setenv works through the testing.T cleanup machinery.
	t.Setenv("PPZ_CURRENT_HANDLE", "")
	_ = os.Unsetenv("PPZ_CURRENT_HANDLE")
	t.Setenv("PPZ_SESSION", "tty-no-env-test")

	dir, err := os.MkdirTemp("/tmp", "ppz-send-no-env-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveSendAliasDaemon(t, sock)

	if err := cmdSend([]string{"eric", "hi"}); err != nil {
		t.Fatalf("cmdSend: %v", err)
	}
	if requests.count() != 1 {
		t.Fatalf("SendRequest count = %d, want 1", requests.count())
	}
	got := requests.at(0)
	if got.Sender != "" {
		t.Fatalf("SendRequest.Sender = %q, want \"\" — no PPZ_CURRENT_HANDLE means no hint; daemon falls back to State.Current(session) on its own",
			got.Sender)
	}
}

func serveSendAliasDaemon(t *testing.T, sock string) *recorder[cliproto.SendRequest] {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	broadcastRequests := &recorder[cliproto.SendRequest]{}
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
			if req.Method == cliproto.IPCSend {
				var br cliproto.SendRequest
				_ = json.Unmarshal(req.Params, &br)
				broadcastRequests.add(br)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.SendReply{
						ID:      "test-id",
						Subject: "org.foo.inbox",
						Bytes:   len(br.Payload),
					},
				})
			}
			_ = conn.Close()
		}
	}()

	return broadcastRequests
}
