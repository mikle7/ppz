package daemon

import (
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

// buildBroadcastEnvelope is the pure envelope-assembly step inside
// handleSend. Kept separate so unit tests can verify field plumbing
// (sender, subject, in_reply_to, ack_requested) without standing up NATS
// or the daemon's IPC plumbing.
//
// `sender` is the broadcaster's *own* current source — distinct from the
// request's Handle, which is the destination. Empty string is permitted
// (headless publish).
func buildBroadcastEnvelope(req cliproto.SendRequest, sender string, now time.Time) envelope.Message {
	env := envelope.New(sender, req.MsgSubject, req.Payload, now)
	env.InReplyTo = req.InReplyTo
	env.AckRequested = req.AckRequested
	return env
}

// publishEnvelope is the in-process helper for daemon-internal envelope
// emission. Two callers:
//
//   - handleSend (after assembling the envelope from a CLI request)
//   - the read-path ack auto-emitter (which builds an `ack:read` envelope
//     directly and bypasses handleSend on purpose, since the IPC-
//     boundary `ack:` rejection rule has no exception path).
//
// No validation here — the rule is "validate at the IPC trust boundary
// (handleSend), trust the daemon-internal callers". Returns nil on
// successful publish + flush.
func (d *Daemon) publishEnvelope(accountID uuid.UUID, dest, pipe string, env envelope.Message) error {
	data, err := env.Marshal()
	if err != nil {
		return err
	}
	// Phase 1.5.1: look up the destination handle's manifold so the
	// subject lands at the right path. Empty for handles cached at
	// root (the common case).
	subject := natsubj.BuildSubject(accountID, d.State.HandleManifold(dest), dest, pipe)
	if err := d.NC.Publish(subject, data); err != nil {
		return err
	}
	return d.NC.Flush()
}
