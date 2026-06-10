package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Rows covering the liveness × agent-state matrix for the renderer:
// fresh beats with each state, a stale beat with a state, and an
// offline beat whose stamped state must be suppressed.
func sampleWhoStateEntries(now time.Time) []cliproto.WhoEntry {
	beat := func(handle, harness, agentState string, age time.Duration) cliproto.WhoEntry {
		payload := HeartbeatPayload{
			TS:            now.Add(-age).Format(time.RFC3339),
			Seq:           1,
			Harness:       harness,
			HarnessSource: "detected",
			AgentState:    agentState,
			ChildPID:      4242,
			Hostname:      "jimmy-mbp",
			OS:            "darwin",
			Arch:          "arm64",
			PID:           12345,
			PPZVersion:    "0.46.0",
			StartedAt:     now.Add(-1 * time.Hour).Format(time.RFC3339),
			IntervalSec:   60,
		}
		raw, _ := json.Marshal(payload)
		return cliproto.WhoEntry{Handle: handle, Payload: string(raw), ArrivedAt: now.Add(-age)}
	}
	return []cliproto.WhoEntry{
		beat("alice", "claude", "working", 10*time.Second), // online|working
		beat("bob", "codex", "idle", 10*time.Second),       // online|idle
		beat("carol", "claude", "blocked", 10*time.Second), // online|blocked
		beat("dave", "claude", "", 10*time.Second),         // online (no harness state)
		beat("erin", "claude", "working", 120*time.Second), // stale|working
		beat("frank", "claude", "working", 10*time.Minute), // offline — state suppressed
	}
}

// The STATUS column combines liveness with the beat's agent state:
// "online|working", "online|blocked", … Plain shells keep the bare
// liveness word, and offline rows never show a state — it would be
// minutes-old noise presented as live.
func TestRenderWho_StatusCombinesLivenessAndAgentState(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	out := renderWho(sampleWhoStateEntries(now), now, whoRenderOpts{Format: "table", UseColor: false})

	for _, want := range []string{"online|working", "online|idle", "online|blocked", "stale|working"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered table missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "offline|") {
		t.Errorf("offline row leaked an agent-state suffix; got:\n%s", out)
	}

	// dave (no agent state) keeps the bare liveness word in his row.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "dave") && strings.Contains(line, "online|") {
			t.Errorf("plain-shell row shows a state suffix: %q", line)
		}
	}
}

// Colour mode wraps the whole combined cell in the liveness colour —
// green "online|working", amber "stale|working", red "offline". The
// state suffix doesn't change the colour key (a distinct colour for
// blocked is a phase-3 candidate, not pinned here).
func TestRenderWho_StatusColorWrapsCombinedCell(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	out := renderWho(sampleWhoStateEntries(now), now, whoRenderOpts{Format: "table", UseColor: true})

	if !strings.Contains(out, "\x1b[32monline|working\x1b[0m") {
		t.Errorf("expected green-wrapped online|working, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[33mstale|working\x1b[0m") {
		t.Errorf("expected amber-wrapped stale|working, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[31moffline\x1b[0m") {
		t.Errorf("expected red-wrapped bare offline, got:\n%s", out)
	}
}

// --json stays machine-readable: top-level status keeps the bare
// liveness word (no "|" composites), and the agent state rides in the
// heartbeat payload for consumers to compose themselves.
func TestRenderWho_JSONStatusStaysLivenessOnly(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	out := renderWho(sampleWhoStateEntries(now), now, whoRenderOpts{Format: "json", UseColor: false})

	var rows []whoJSONRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, r := range rows {
		if strings.Contains(r.Status, "|") {
			t.Errorf("row %q: json status = %q, want bare liveness", r.Handle, r.Status)
		}
		if r.Handle == "alice" && r.Heartbeat.AgentState != "working" {
			t.Errorf("alice heartbeat.agent_state = %q, want working", r.Heartbeat.AgentState)
		}
	}
}
