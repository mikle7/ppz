package daemon

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// v0.25.0: handleBroadcast is the trust boundary for the `ack:` reserved-
// prefix invariant. CLI argument validation alone is not enough — any IPC
// client (custom scripts, third-party tools, harness adapters) can hit
// this handler. The check MUST live in the daemon, not just in the CLI
// flag parser. (See spec §3 "Daemon-side `ack:` prefix rejection".)
//
// Test rule: the handler returns E_INVALID_SUBJECT BEFORE doing any I/O.
// We rely on that to keep this test pure: no credentials, no NATS, no
// server.
func TestHandleBroadcast_RejectsAckPrefix(t *testing.T) {
	cases := []string{
		"ack:read",
		"ack:processed",
		"ack:",
		"ack:read foo",
	}
	for _, sub := range cases {
		t.Run(sub, func(t *testing.T) {
			err := callHandleBroadcast(t, cliproto.BroadcastRequest{
				Handle:     "foo",
				Channel:    "inbox",
				Payload:    "hi",
				MsgSubject: sub,
			})
			if err == nil {
				t.Fatalf("MsgSubject=%q expected an error, got success", sub)
			}
			if err.Code != cliproto.EInvalidSubject {
				t.Fatalf("MsgSubject=%q error code = %s, want E_INVALID_SUBJECT", sub, err.Code)
			}
		})
	}
}

// Subjects that don't start with `ack:` must NOT be rejected by the
// trust-boundary check. They may still fail downstream (auth, source
// resolution, etc.), but the failure MUST NOT be E_INVALID_SUBJECT.
func TestHandleBroadcast_AllowsNonAckSubjects(t *testing.T) {
	for _, sub := range []string{"", "status update", "[ack]", "ackreply", "user:ack:read"} {
		t.Run(sub, func(t *testing.T) {
			err := callHandleBroadcast(t, cliproto.BroadcastRequest{
				Handle:     "foo",
				Channel:    "inbox",
				Payload:    "hi",
				MsgSubject: sub,
			})
			// We don't care which specific error fires (likely ENotLoggedIn
			// in this minimal harness), only that it is NOT
			// E_INVALID_SUBJECT.
			if err != nil && err.Code == cliproto.EInvalidSubject {
				t.Fatalf("MsgSubject=%q wrongly rejected as E_INVALID_SUBJECT", sub)
			}
		})
	}
}

// callHandleBroadcast wires a Daemon to a net.Pipe()-backed connection,
// fires one BroadcastRequest at handleBroadcast, and returns the error
// (or nil on success).
func callHandleBroadcast(t *testing.T, req cliproto.BroadcastRequest) *cliproto.Error {
	t.Helper()
	d := &Daemon{State: NewState(t.TempDir())}
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	body, _ := json.Marshal(req)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverSide.Close()
		d.handleBroadcast(context.Background(), serverSide, body)
	}()

	dec := json.NewDecoder(clientSide)
	var resp struct {
		Result map[string]any  `json:"result,omitempty"`
		Error  *cliproto.Error `json:"error,omitempty"`
	}
	if err := dec.Decode(&resp); err != nil {
		// Server may have closed without writing anything if the path
		// reaches a write-and-publish that fails — surface as a synthetic
		// error so the test can assert on it.
		if strings.Contains(err.Error(), "EOF") {
			return &cliproto.Error{Code: "E_NO_RESPONSE", Message: err.Error()}
		}
		t.Fatalf("decode: %v", err)
	}
	<-done
	return resp.Error
}
