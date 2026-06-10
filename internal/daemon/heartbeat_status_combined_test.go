package daemon

import "testing"

// CombineHeartbeatStatus merges the liveness tri-state with the agent
// state from the beat: state suffixes attach while the beat is fresh
// enough to mean something (online/stale), and drop entirely for
// offline rows where they'd just be minutes-old noise.
func TestCombineHeartbeatStatus(t *testing.T) {
	cases := []struct {
		liveness   string
		agentState string
		want       string
	}{
		{"online", "working", "online|working"},
		{"online", "idle", "online|idle"},
		{"online", "blocked", "online|blocked"},
		{"online", "", "online"},
		{"stale", "working", "stale|working"},
		{"stale", "", "stale"},
		{"offline", "working", "offline"},
		{"offline", "", "offline"},
	}
	for _, c := range cases {
		if got := CombineHeartbeatStatus(c.liveness, c.agentState); got != c.want {
			t.Errorf("CombineHeartbeatStatus(%q, %q) = %q, want %q",
				c.liveness, c.agentState, got, c.want)
		}
	}
}
