package daemon

import (
	"context"
	"encoding/json"
	"net"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// handleWho returns the daemon's in-memory heartbeat snapshot — the
// data backing `ppz who`. Like handleDiag, this verb must work even
// when the daemon has no NATS connection: an operator looking up "who
// was running before my daemon got sick" should always get an answer.
//
// All filter / colour / format work happens client-side in cmdWho; the
// daemon's only job is to dump the cache.
func (d *Daemon) handleWho(ctx context.Context, conn net.Conn, params json.RawMessage) {
	_ = ctx
	var req cliproto.WhoRequest
	_ = json.Unmarshal(params, &req) // currently empty; reserved for future scoping

	reply := cliproto.WhoReply{Entries: []cliproto.WhoEntry{}}
	for _, e := range d.Heartbeats.Snapshot() {
		reply.Entries = append(reply.Entries, cliproto.WhoEntry{
			Handle:    e.Handle,
			Payload:   e.Payload,
			ArrivedAt: e.ArrivedAt,
		})
	}
	writeIPC(conn, reply)
}
