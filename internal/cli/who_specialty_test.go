package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// specialtyEntries builds a roster with specialty + agent_state variety:
// muster stamps its role template into PPZ_AGENT_SPECIALTY, so a
// cross-machine dispatcher can ask "who is a free backend?".
func specialtyEntries(now time.Time) []cliproto.WhoEntry {
	beat := func(handle, specialty, state string, age time.Duration) cliproto.WhoEntry {
		payload := HeartbeatPayload{
			TS:          now.Add(-age).Format(time.RFC3339),
			Harness:     "claude",
			AgentState:  state,
			Specialty:   specialty,
			IntervalSec: 60,
		}
		raw, _ := json.Marshal(payload)
		return cliproto.WhoEntry{Handle: handle, Payload: string(raw), ArrivedAt: now.Add(-age)}
	}
	return []cliproto.WhoEntry{
		beat("backend-1", "backend", "working", 10*time.Second), // online, busy
		beat("backend-2", "backend", "idle", 5*time.Second),     // online, free ← the bench
		beat("backend-3", "backend", "idle", 10*time.Minute),    // offline — idle claim is stale, not dispatchable
		beat("front-1", "frontend", "idle", 5*time.Second),      // online, free, other specialty
		beat("plain", "", "idle", 5*time.Second),                // no specialty advertised
	}
}

func TestBuildHeartbeatPayload_Specialty(t *testing.T) {
	got := buildHeartbeatPayload(heartbeatInputs{Now: time.Now(), Specialty: "backend"})
	if got.Specialty != "backend" {
		t.Fatalf("specialty = %q, want backend", got.Specialty)
	}
	raw, _ := json.Marshal(got)
	var back map[string]any
	_ = json.Unmarshal(raw, &back)
	if back["specialty"] != "backend" {
		t.Fatalf("wire key missing: %s", raw)
	}
}

func TestFilterWhoEntries_BySpecialty(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	got := filterWhoEntries(specialtyEntries(now), now, whoFilter{Specialty: "backend"})
	if h := handlesOf(got); !equalStrSlice(h, []string{"backend-1", "backend-2", "backend-3"}) {
		t.Errorf("--specialty=backend: got %v", h)
	}
	if got := filterWhoEntries(specialtyEntries(now), now, whoFilter{Specialty: "nosuch"}); len(got) != 0 {
		t.Errorf("--specialty=nosuch: want empty, got %v", handlesOf(got))
	}
}

// --free is the dispatcher's bench query: provably reachable (online)
// AND provably not mid-task (agent_state idle). An offline agent whose
// last beat said idle must NOT count — that claim is stale.
func TestFilterWhoEntries_Free(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	got := filterWhoEntries(specialtyEntries(now), now, whoFilter{Free: true})
	if h := handlesOf(got); !equalStrSlice(h, []string{"backend-2", "front-1", "plain"}) {
		t.Errorf("--free: got %v", h)
	}
	got = filterWhoEntries(specialtyEntries(now), now, whoFilter{Free: true, Specialty: "backend"})
	if h := handlesOf(got); !equalStrSlice(h, []string{"backend-2"}) {
		t.Errorf("--free --specialty=backend: got %v", h)
	}
}
