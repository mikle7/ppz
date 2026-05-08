package daemon

import (
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// buildBroadcastEnvelope is the pure envelope-assembly used inside
// handleBroadcast. Pulling it out means we can verify the v0.25.0 field
// plumbing without standing up NATS or the daemon's IPC plumbing.
func TestBuildBroadcastEnvelope_PlumbsAllFields(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	req := cliproto.BroadcastRequest{
		Handle:       "foo",
		Channel:      "inbox",
		Payload:      "hi",
		MsgSubject:   "status update",
		InReplyTo:    "11111111-2222-3333-4444-555566667777",
		AckRequested: true,
	}
	env := buildBroadcastEnvelope(req, "alpha", now)

	if env.Sender != "alpha" {
		t.Errorf("Sender = %q, want alpha", env.Sender)
	}
	if env.Subject != "status update" {
		t.Errorf("Subject = %q, want status update", env.Subject)
	}
	if env.Payload != "hi" {
		t.Errorf("Payload = %q", env.Payload)
	}
	if env.InReplyTo != "11111111-2222-3333-4444-555566667777" {
		t.Errorf("InReplyTo = %q, want from BroadcastRequest", env.InReplyTo)
	}
	if !env.AckRequested {
		t.Errorf("AckRequested lost; envelope did not pick it up from BroadcastRequest")
	}
	if !env.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", env.CreatedAt, now)
	}
}

// AckRequested defaults to false (zero value of the request) — confirm
// the helper doesn't synthesise it.
func TestBuildBroadcastEnvelope_NoAckByDefault(t *testing.T) {
	env := buildBroadcastEnvelope(cliproto.BroadcastRequest{Payload: "p"}, "alpha", time.Now())
	if env.AckRequested {
		t.Errorf("AckRequested should default to false")
	}
	if env.InReplyTo != "" {
		t.Errorf("InReplyTo should default to empty, got %q", env.InReplyTo)
	}
}
