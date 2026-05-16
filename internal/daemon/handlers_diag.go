package daemon

import (
	"context"
	"encoding/json"
	"net"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// handleDiag returns the daemon's introspection snapshot — Phase 0 of
// the agent hardening plan: NATS connection state + the recent ring
// of connection-state events.
//
// CRITICAL: this handler must succeed even when the daemon has no
// credentials and no NATS connection. The whole point of `ppz diagnostics`
// is being able to introspect a sick daemon — if the verb itself
// requires login or a live NATS connection, an operator can't use it
// in the failure modes it's designed to surface.
func (d *Daemon) handleDiag(ctx context.Context, conn net.Conn, params json.RawMessage) {
	_ = ctx
	var req cliproto.DiagRequest
	_ = json.Unmarshal(params, &req) // optional, currently empty

	state, drops, _ := d.natsStatusSnapshot()
	reply := cliproto.DiagReply{
		NATSState:         state,
		NATSDropsLastHour: drops,
		NATSEvents:        []cliproto.DiagEvent{},
	}
	if d.NATSEvents != nil {
		for _, ev := range d.NATSEvents.Snapshot() {
			reply.NATSEvents = append(reply.NATSEvents, cliproto.DiagEvent{
				Type:   ev.Type,
				At:     ev.At,
				Reason: ev.Reason,
			})
		}
	}
	writeIPC(conn, reply)
}
