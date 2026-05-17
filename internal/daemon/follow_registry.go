package daemon

import (
	"net"
	"sync"
)

// followRegistry tracks live IPC conns that are streaming JetStream
// events back to a CLI (the `Follow: true` mode of handleRead). When
// the daemon's NATS connection is replaced — logout (watcher), login
// re-run (handleLogin), or refresh-time rotation (ensureNATS) — the
// old consumers anchored to the old NC go silent: messages stop
// flowing to the CLI even though the IPC socket itself stays open,
// so the CLI's outer redial loop never fires.
//
// closeAll() — called from swapNC() — eagerly closes every
// registered conn so the CLI sees EOF, redials, and gets a fresh
// follow anchored to the new NC.
//
// Conns deregister themselves on normal exit (defer remove() in
// handleRead) so a closed conn doesn't leak a registry slot.
type followRegistry struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func newFollowRegistry() *followRegistry {
	return &followRegistry{conns: make(map[net.Conn]struct{})}
}

func (r *followRegistry) add(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[c] = struct{}{}
}

func (r *followRegistry) remove(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, c)
}

// closeAll closes every registered conn and clears the registry.
// Safe to call concurrently with add/remove; conns that race in
// after closeAll returns will see a fresh NC.
func (r *followRegistry) closeAll() {
	r.mu.Lock()
	toClose := make([]net.Conn, 0, len(r.conns))
	for c := range r.conns {
		toClose = append(toClose, c)
	}
	r.conns = make(map[net.Conn]struct{})
	r.mu.Unlock()
	for _, c := range toClose {
		_ = c.Close()
	}
}
