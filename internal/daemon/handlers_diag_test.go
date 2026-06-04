package daemon

import (
	"testing"
	"time"
)

// TestStateSinceFrom_ReturnsEntryEventType pins the helper's promoted
// signature: along with the transition timestamp it returns the event
// type that anchored it, so the CLI can distinguish a clean first-
// connect from a recent reconnect (see formatNATSLine's colour matrix).
//
// Scenarios:
//   - empty ring                 → (zero, "")
//   - state=connected, only `connect`   → (connect-at, "connect")
//   - state=connected, only `reconnect` → (reconnect-at, "reconnect")
//   - state=connected, both     → most recent of the two wins
//   - state=disconnected, mixed → most recent `disconnect`/`closed` wins
//   - state=connected, ring has only `disconnect` → (zero, "") — no match
//   - state="" (unobserved)     → (zero, "")
func TestStateSinceFrom_ReturnsEntryEventType(t *testing.T) {
	t0 := time.Date(2026, 5, 3, 6, 0, 0, 0, time.UTC)
	mk := func(typ string, offset time.Duration) NATSEvent {
		return NATSEvent{Type: typ, At: t0.Add(offset)}
	}

	cases := []struct {
		name      string
		state     string
		events    []NATSEvent
		wantAt    time.Time
		wantEntry string
	}{
		{
			name:      "empty ring returns zero",
			state:     "connected",
			events:    nil,
			wantAt:    time.Time{},
			wantEntry: "",
		},
		{
			name:      "connect-only marks first connect",
			state:     "connected",
			events:    []NATSEvent{mk("connect", 0)},
			wantAt:    t0,
			wantEntry: "connect",
		},
		{
			name:      "reconnect-only marks recovery",
			state:     "connected",
			events:    []NATSEvent{mk("disconnect", 1*time.Second), mk("reconnect", 5*time.Second)},
			wantAt:    t0.Add(5 * time.Second),
			wantEntry: "reconnect",
		},
		{
			name:      "most recent connect/reconnect wins",
			state:     "connected",
			events:    []NATSEvent{mk("connect", 0), mk("disconnect", 1*time.Second), mk("reconnect", 5*time.Second)},
			wantAt:    t0.Add(5 * time.Second),
			wantEntry: "reconnect",
		},
		{
			name:      "disconnected returns most recent disconnect",
			state:     "disconnected",
			events:    []NATSEvent{mk("connect", 0), mk("disconnect", 5*time.Second)},
			wantAt:    t0.Add(5 * time.Second),
			wantEntry: "disconnect",
		},
		{
			name:      "disconnected prefers closed over earlier disconnect",
			state:     "disconnected",
			events:    []NATSEvent{mk("disconnect", 1*time.Second), mk("closed", 2*time.Second)},
			wantAt:    t0.Add(2 * time.Second),
			wantEntry: "closed",
		},
		{
			name:      "connected with no matching entry returns zero",
			state:     "connected",
			events:    []NATSEvent{mk("disconnect", 1*time.Second), mk("swap", 2*time.Second)},
			wantAt:    time.Time{},
			wantEntry: "",
		},
		{
			name:      "unobserved state returns zero",
			state:     "",
			events:    []NATSEvent{mk("connect", 0)},
			wantAt:    time.Time{},
			wantEntry: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotAt, gotEntry := stateSinceFrom(tc.state, tc.events)
			if !gotAt.Equal(tc.wantAt) {
				t.Errorf("stateSinceFrom timestamp\nwant: %v\ngot:  %v", tc.wantAt, gotAt)
			}
			if gotEntry != tc.wantEntry {
				t.Errorf("stateSinceFrom entry type\nwant: %q\ngot:  %q", tc.wantEntry, gotEntry)
			}
		})
	}
}
