package daemon

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
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
		d.Heartbeats.Stamp(handle, string(msg.Data), time.Now())
	})
}
