package daemon

import (
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// natsStateString collapses the rich nats.Conn.Status() enum into the
// three-value vocabulary surfaced by `ppz status`'s `nats:` line.
// CONNECTING / RECONNECTING both render as "connecting" — they're the
// same thing from an operator's perspective ("client is trying").
// CLOSED collapses to "disconnected" — the connection is gone, with
// or without intent to reconnect.
//
// Returns "" when nc is nil (daemon has never attempted to connect,
// e.g. fresh process pre-login).
func natsStateString(nc *nats.Conn) string {
	if nc == nil {
		return ""
	}
	switch nc.Status() {
	case nats.CONNECTED:
		return "connected"
	case nats.CONNECTING, nats.RECONNECTING:
		return "connecting"
	case nats.DISCONNECTED, nats.CLOSED:
		return "disconnected"
	default:
		return "disconnected"
	}
}

// natsEventRingCap is the maximum number of NATS connection-state events
// retained in memory. Phase 0 of the agent-hardening plan calls for a
// short tail — enough to catch events that happened "a few minutes ago"
// when an operator runs `ppz diag`, not a full history.
const natsEventRingCap = 32

// NATSEvent is one entry in the daemon's NATS connection-state log.
// Type is one of "connect", "disconnect", "reconnect", "closed" —
// matching the nats.go handler set we register. At is the moment the
// handler fired. Reason captures the error string for disconnect /
// closed events (empty for the others). Phase 0 is observe-only —
// these events drive `ppz status` and `ppz diag` output but do not
// influence reconnect behaviour.
type NATSEvent struct {
	Type   string    `json:"type"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason,omitempty"`
}

// NATSEventRing is a fixed-capacity, append-only, drop-oldest ring of
// NATSEvent records. Used by the daemon to surface a recent tail of
// connection-state transitions through `ppz diag`.
//
// Thread-safe: every accessor takes mu. The ring lives on Daemon and
// is initialised in New() — before any nats.Connect call — so the
// handlers we register can append without nil-checking.
type NATSEventRing struct {
	mu     sync.Mutex
	events []NATSEvent
	cap    int
}

func newNATSEventRing(cap int) *NATSEventRing {
	if cap <= 0 {
		cap = natsEventRingCap
	}
	return &NATSEventRing{cap: cap, events: make([]NATSEvent, 0, cap)}
}

// Append records one event. When the ring is full the oldest entry is
// dropped — diag's value is precisely catching transient events that
// happened "a few minutes ago", so we keep the tail and lose the head.
func (r *NATSEventRing) Append(typ, reason string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev := NATSEvent{Type: typ, At: at, Reason: reason}
	if len(r.events) < r.cap {
		r.events = append(r.events, ev)
		return
	}
	copy(r.events, r.events[1:])
	r.events[len(r.events)-1] = ev
}

// Snapshot returns a copy of the current events in chronological order
// (oldest first). Safe to retain — the returned slice is independent of
// the ring's internal storage.
func (r *NATSEventRing) Snapshot() []NATSEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]NATSEvent, len(r.events))
	copy(out, r.events)
	return out
}

// natsStatusSnapshot derives the fields the `ppz status` "nats:" line
// needs from the daemon's current NATS connection + ring buffer. Lives
// here next to the ring so the dependencies stay co-located.
//
// Returns ("", 0, nil) when the daemon has never attempted a NATS
// connection (NC nil AND no events recorded). The CLI renders that as
// "unknown" — preferable to lying with "disconnected" before any
// connect has been tried.
func (d *Daemon) natsStatusSnapshot() (state string, dropsLastHour int, lastEventAt *time.Time) {
	state = natsStateString(d.NC)
	if d.NATSEvents == nil {
		return state, 0, nil
	}
	events := d.NATSEvents.Snapshot()
	if len(events) > 0 {
		t := events[len(events)-1].At
		lastEventAt = &t
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, ev := range events {
		if ev.Type == "disconnect" && !ev.At.Before(cutoff) {
			dropsLastHour++
		}
	}
	return state, dropsLastHour, lastEventAt
}
