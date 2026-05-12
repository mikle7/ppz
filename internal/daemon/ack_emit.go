package daemon

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/envelope"
)

// shouldEmitAck returns true when delivery of `m` should trigger a
// daemon-emitted `ack:read` back to `m.Sender.inbox`. Three guards:
//
//  1. AckRequested must be set on the original.
//  2. Sender must be non-empty (else there's no destination — see §4).
//  3. Subject must NOT start with `ack:` (loop guard — belt-and-
//     suspenders since acks have AckRequested=false by default).
func shouldEmitAck(m cliproto.ReadMessage) bool {
	if !m.AckRequested {
		return false
	}
	if m.Sender == "" {
		return false
	}
	if strings.HasPrefix(m.Subject, "ack:") {
		return false
	}
	return true
}

// buildAckEnvelope constructs the `ack:read` envelope a recipient
// daemon publishes back to the original sender's inbox after consuming
// `original`. `self` is the reading daemon's current source — empty
// string is permitted (renders as `-` in the tabular formatter).
func buildAckEnvelope(original cliproto.ReadMessage, self string, now time.Time) envelope.Message {
	ack := envelope.New(self, "ack:read", "", now)
	ack.InReplyTo = original.ID
	// Explicitly false. envelope.New leaves it false by default; restated
	// here so a future refactor that flips the default doesn't silently
	// turn ack-of-ack on.
	ack.AckRequested = false
	return ack
}

// emitAckFn is the publish abstraction emitAcks calls per message. The
// production wiring binds this to Daemon.publishEnvelope; tests inject
// a stub to verify call shape and to exercise the failure path
// (failures must not abort the loop — §4 "best-effort, fire-and-forget").
type emitAckFn func(accountID uuid.UUID, dest, pipe string, env envelope.Message) error

// emitAcks walks `retained` (the messages the read handler just
// delivered) and publishes one `ack:read` back to each qualifying
// original sender's inbox. Each publish failure is silently dropped —
// callers see no signal — because cursor advancement is decoupled from
// ack publishing on purpose: a NATS partition or transient failure must
// not wedge reading.
//
// Caller is responsible for advancing the cursor BEFORE invoking this
// function (per spec §4 advance-then-emit ordering).
func emitAcks(accountID uuid.UUID, self string, retained []cliproto.ReadMessage, now time.Time, publish emitAckFn) {
	for _, m := range retained {
		if !shouldEmitAck(m) {
			continue
		}
		ack := buildAckEnvelope(m, self, now)
		// Best-effort: ignore the error. Don't break, don't return —
		// each message is independent.
		_ = publish(accountID, m.Sender, "inbox", ack)
	}
}
