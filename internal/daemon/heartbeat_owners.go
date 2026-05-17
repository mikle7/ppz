package daemon

import "github.com/pipescloud/ppz/internal/cliproto"

// enrichEntriesWithOwners joins the daemon's in-memory heartbeat cache
// with a server-side source listing (which carries the per-source
// CreatedBy / owner username) and produces the wire shape `ppz who`
// consumes. Pure function — no IPC, no network — so handleWho's
// orchestration and the join logic can evolve independently.
//
// Order from `cache` is preserved verbatim (Snapshot already sorts by
// handle). Cache rows whose handle isn't in `sources` (orphan: server
// deleted the source but a beat is still cached locally) get Owner =
// "" so the renderer can fall back to "-".
func enrichEntriesWithOwners(cache []HeartbeatEntry, sources []cliproto.Source) []cliproto.WhoEntry {
	owners := make(map[string]string, len(sources))
	for _, s := range sources {
		owners[s.Handle] = s.CreatedBy
	}
	out := make([]cliproto.WhoEntry, 0, len(cache))
	for _, e := range cache {
		out = append(out, cliproto.WhoEntry{
			Handle:    e.Handle,
			Owner:     owners[e.Handle],
			Payload:   e.Payload,
			ArrivedAt: e.ArrivedAt,
		})
	}
	return out
}
