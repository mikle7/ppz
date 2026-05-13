package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// v0.25.0: --subject / --in-reply-to / --request-ack flags on `ppz send`,
// plus the success line moves from stdout to stderr, with an
// `ack=requested` token appended when --request-ack is set.

// --subject ack:foo is rejected by the CLI argument parser BEFORE the
// daemon call. Belt-and-suspenders rejection (handlers.go does it too at
// the IPC trust boundary).
func TestCmdSend_RejectsAckSubjectAtCLI(t *testing.T) {
	requests, _ := setupV25SendDaemon(t, "alpha")

	err := cmdSendForTest([]string{"foo", "hi", "--subject", "ack:read"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatalf("--subject ack:read should be rejected")
	}
	cerr := asCliErr(t, err)
	if cerr.Code != cliproto.EInvalidSubject {
		t.Fatalf("error code = %s, want E_INVALID_SUBJECT", cerr.Code)
	}
	if n := len(*requests); n > 0 {
		t.Fatalf("CLI should reject before any IPCSend call; got %d requests", n)
	}
}

// --request-ack with no current source: rejected at the CLI before
// SendRequest is sent. The CLI gets the current source from
// IPCStatus — when that returns "" the CLI exits with ENoCurrentSource.
func TestCmdSend_RequestAckRequiresCurrentSource(t *testing.T) {
	requests, _ := setupV25SendDaemon(t, "")

	err := cmdSendForTest([]string{"foo", "hi", "--request-ack"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatalf("--request-ack without current source should be rejected")
	}
	cerr := asCliErr(t, err)
	if cerr.Code != cliproto.ENoCurrentSource {
		t.Fatalf("error code = %s, want E_NO_CURRENT_SOURCE", cerr.Code)
	}
	for _, r := range *requests {
		if r.AckRequested {
			t.Fatalf("CLI should not have submitted a request with AckRequested=true")
		}
	}
}

// Non-flag normal send: SendRequest is unchanged, success line goes
// to STDERR (not stdout).
func TestCmdSend_SuccessLineWritesToStderr(t *testing.T) {
	_, _ = setupV25SendDaemon(t, "alpha")

	var stdout, stderr bytes.Buffer
	if err := cmdSendForTest([]string{"foo", "hi"}, &stdout, &stderr); err != nil {
		t.Fatalf("send: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty; got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "sent id=") {
		t.Fatalf("stderr should carry the success line; got %q", stderr.String())
	}
	// No --request-ack → no token.
	if strings.Contains(stderr.String(), "ack=requested") {
		t.Fatalf("plain send leaked ack=requested token: %q", stderr.String())
	}
}

// --request-ack adds the `ack=requested` token to the stderr success line
// AND propagates AckRequested through the SendRequest.
func TestCmdSend_RequestAckSetsTokenAndField(t *testing.T) {
	requests, _ := setupV25SendDaemon(t, "alpha")

	var stdout, stderr bytes.Buffer
	if err := cmdSendForTest([]string{"foo", "hi", "--request-ack"}, &stdout, &stderr); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(stderr.String(), "ack=requested") {
		t.Fatalf("--request-ack stderr missing ack=requested token: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should remain empty under v0.25.0 send: %q", stdout.String())
	}
	// IPCStatus is called for the preflight, so requests of method
	// IPCSend must be the second-to-last call. Walk and find it.
	found := false
	for _, r := range *requests {
		if r.AckRequested {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no SendRequest with AckRequested=true reached the daemon; got %+v", *requests)
	}
}

// --subject (legal) and --in-reply-to plumb through to SendRequest.
func TestCmdSend_SubjectAndInReplyTo(t *testing.T) {
	requests, _ := setupV25SendDaemon(t, "alpha")

	var stdout, stderr bytes.Buffer
	args := []string{"foo", "hi", "--subject", "status update", "--in-reply-to", "abc-123"}
	if err := cmdSendForTest(args, &stdout, &stderr); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(*requests) == 0 {
		t.Fatalf("no broadcast request reached the daemon")
	}
	var got *cliproto.SendRequest
	for i := range *requests {
		r := (*requests)[i]
		if r.MsgSubject != "" || r.InReplyTo != "" {
			got = &r
			break
		}
	}
	if got == nil {
		t.Fatalf("no SendRequest carried subject / in_reply_to: %+v", *requests)
	}
	if got.MsgSubject != "status update" {
		t.Fatalf("MsgSubject = %q, want status update", got.MsgSubject)
	}
	if got.InReplyTo != "abc-123" {
		t.Fatalf("InReplyTo = %q, want abc-123", got.InReplyTo)
	}
}

// cmdSendForTest is a tiny shim around cmdSend that funnels stdout /
// stderr to test-supplied writers via the package-level overrides.
func cmdSendForTest(args []string, stdout, stderr io.Writer) error {
	prevOut := sendOut
	prevErr := sendErr
	sendOut = stdout
	sendErr = stderr
	defer func() { sendOut = prevOut; sendErr = prevErr }()
	return cmdSend(args)
}

func asCliErr(t *testing.T, err error) *cliproto.Error {
	t.Helper()
	var c *cliproto.Error
	if !errors.As(err, &c) {
		t.Fatalf("expected *cliproto.Error, got %T: %v", err, err)
	}
	return c
}

// setupV25SendDaemon spins a fake daemon socket that responds to both
// IPCStatus (returns the configured `current` source) and IPCSend
// (records the request and replies with a deterministic ID). Returns a
// pointer to the recorded SendRequests so each test can assert on
// what the CLI sent.
func setupV25SendDaemon(t *testing.T, current string) (*[]cliproto.SendRequest, *[]string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ppz-send-v25-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}

	var (
		mu                sync.Mutex
		broadcastRequests []cliproto.SendRequest
		methodLog         []string
	)
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
			mu.Lock()
			methodLog = append(methodLog, req.Method)
			mu.Unlock()
			switch req.Method {
			case cliproto.IPCStatus:
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.StatusReply{DaemonPID: 1234, LoggedIn: true, Current: current},
				})
			case cliproto.IPCSend:
				var br cliproto.SendRequest
				_ = json.Unmarshal(req.Params, &br)
				mu.Lock()
				broadcastRequests = append(broadcastRequests, br)
				mu.Unlock()
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.SendReply{
						ID:      "deadbeefcafebabedeadbeefcafebabe",
						Subject: "org.foo.inbox",
						Bytes:   len(br.Payload),
					},
				})
			}
			_ = conn.Close()
		}
	}()

	return &broadcastRequests, &methodLog
}
