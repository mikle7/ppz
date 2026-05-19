package daemon

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/pipescloud/ppz/internal/envelope"
)

// subscribeOrgHeartbeats sets up a NATS subscription on the current
// connection that stamps the local heartbeat cache for every
// <handle>.heartbeat message published in the org — by this daemon or
// any other. This is what makes ppz who cross-daemon-aware: without it
// handleWho only shows agents whose heartbeats arrived via the local
// handleSend path.
//
// Subjects follow the shape <accountID>.<manifold...>.<handle>.heartbeat.
// We subscribe to <accountID>.> and filter client-side for the
// .heartbeat suffix; NATS does not support wildcards in the middle of a
// subject pattern.
//
// Each NATS message body is an envelope.Message; the raw heartbeat JSON
// is in the Payload field. We unmarshal the envelope and stamp the cache
// with the inner payload, matching the shape handleSend writes.
//
// Called once per NATS connect in ensureNATS after swapNC. The prior
// subscription is destroyed automatically when swapNC closes the old
// nats.Conn.
func (d *Daemon) subscribeOrgHeartbeats(accountID uuid.UUID) {
	prefix := accountID.String() + "."
	const suffix = ".heartbeat"
	_, _ = d.NC.Subscribe(prefix+">", func(msg *nats.Msg) {
		subj := msg.Subject
		if !strings.HasSuffix(subj, suffix) {
			return
		}
		// Handle is everything between the account prefix and .heartbeat,
		// e.g. "agent-b" for root-manifold or "ns.agent-b" for namespaced.
		handle := strings.TrimSuffix(strings.TrimPrefix(subj, prefix), suffix)
		if handle == "" {
			return
		}
		// NATS messages are envelope-wrapped; the heartbeat JSON is the
		// inner Payload field, matching what handleSend stamps directly.
		var env envelope.Message
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			return
		}
		d.Heartbeats.Stamp(handle, env.Payload, time.Now())
	})
}
