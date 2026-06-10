package cli

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/harness"
)

// Live detection beats the launch-time env var: the env var survives
// the harness exiting back to a shell, detection tracks the actual
// foreground. The source string records which side won so consumers
// can tell a `ppz agent` launch from an auto-detected manual one.
func TestResolveHarness_Precedence(t *testing.T) {
	cases := []struct {
		name       string
		detected   string
		env        string
		wantName   string
		wantSource string
	}{
		{"detected wins over env", "codex", "claude", "codex", "detected"},
		{"detected alone", "claude", "", "claude", "detected"},
		{"env fallback", "", "claude", "claude", "env"},
		{"neither", "", "", "", ""},
	}
	for _, c := range cases {
		gotName, gotSource := resolveHarness(c.detected, c.env)
		if gotName != c.wantName || gotSource != c.wantSource {
			t.Errorf("%s: resolveHarness(%q, %q) = (%q, %q), want (%q, %q)",
				c.name, c.detected, c.env, gotName, gotSource, c.wantName, c.wantSource)
		}
	}
}

// The pure builder maps the three new agent inputs straight through to
// the wire shape, same as every other field.
func TestBuildHeartbeatPayload_AgentFields(t *testing.T) {
	in := heartbeatInputs{
		Now:           time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		Seq:           1,
		Harness:       "claude",
		HarnessSource: "detected",
		AgentState:    "working",
		ChildPID:      4242,
		Model:         "",
		Hostname:      "h",
		OS:            "darwin",
		Arch:          "arm64",
		PID:           1,
		PPZVersion:    "0.0.0",
		StartedAt:     time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC),
		IntervalSec:   60,
	}
	got := buildHeartbeatPayload(in)

	if got.HarnessSource != "detected" {
		t.Errorf("harness_source = %q, want detected", got.HarnessSource)
	}
	if got.AgentState != "working" {
		t.Errorf("agent_state = %q, want working", got.AgentState)
	}
	if got.ChildPID != 4242 {
		t.Errorf("child_pid = %d, want 4242", got.ChildPID)
	}
}

// A wired detector stamps every beat with the live snapshot: harness
// from detection (source "detected"), the foreground pid, and the
// current state — even when the env var disagrees.
func TestRunHeartbeat_StampsDetectionSnapshot(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	env := map[string]string{"PPZ_AGENT_HARNESS": "claude", "PPZ_AGENT_MODEL": "opus"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:     func() time.Time { return now },
		Tick:    tick,
		Publish: pub.publish,
		GetEnv:  func(k string) string { return env[k] },
		Detect: func() harness.Detection {
			return harness.Detection{Harness: "codex", ChildPID: 777, State: harness.StateWorking}
		},
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "darwin",
		Arch:        "arm64",
		PID:         123,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	msgs := pub.waitForCount(t, 1)
	var p HeartbeatPayload
	if err := json.Unmarshal([]byte(msgs[0].payload), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Harness != "codex" {
		t.Errorf("harness = %q, want codex (detection wins over env)", p.Harness)
	}
	if p.HarnessSource != "detected" {
		t.Errorf("harness_source = %q, want detected", p.HarnessSource)
	}
	if p.ChildPID != 777 {
		t.Errorf("child_pid = %d, want 777", p.ChildPID)
	}
	if p.AgentState != "working" {
		t.Errorf("agent_state = %q, want working", p.AgentState)
	}
}

// When detection has nothing (plain shell in the foreground), the beat
// falls back to the env var with source "env" — `ppz agent` launches
// keep their harness column even before/without detection.
func TestRunHeartbeat_DetectionEmptyFallsBackToEnv(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	env := map[string]string{"PPZ_AGENT_HARNESS": "claude"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:         func() time.Time { return now },
		Tick:        tick,
		Publish:     pub.publish,
		GetEnv:      func(k string) string { return env[k] },
		Detect:      func() harness.Detection { return harness.Detection{} },
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "darwin",
		Arch:        "arm64",
		PID:         123,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	msgs := pub.waitForCount(t, 1)
	var p HeartbeatPayload
	_ = json.Unmarshal([]byte(msgs[0].payload), &p)
	if p.Harness != "claude" {
		t.Errorf("harness = %q, want claude (env fallback)", p.Harness)
	}
	if p.HarnessSource != "env" {
		t.Errorf("harness_source = %q, want env", p.HarnessSource)
	}
	if p.AgentState != "" {
		t.Errorf("agent_state = %q, want empty (nothing detected)", p.AgentState)
	}
}

// A nil Detect (no detector wired — non-PTY callers, old tests) must
// behave exactly like the pre-detection wrapper: env-stamped harness,
// no agent fields, no panic.
func TestRunHeartbeat_NilDetectFallsBackToEnv(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	env := map[string]string{"PPZ_AGENT_HARNESS": "claude"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:         func() time.Time { return now },
		Tick:        tick,
		Publish:     pub.publish,
		GetEnv:      func(k string) string { return env[k] },
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "darwin",
		Arch:        "arm64",
		PID:         123,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	msgs := pub.waitForCount(t, 1)
	var p HeartbeatPayload
	_ = json.Unmarshal([]byte(msgs[0].payload), &p)
	if p.Harness != "claude" || p.HarnessSource != "env" {
		t.Errorf("harness/source = %q/%q, want claude/env", p.Harness, p.HarnessSource)
	}
}

// A StateChanged wake emits an immediate out-of-cycle beat — 60s ticks
// are too coarse for the working/idle column in `ppz who`. Seq stays
// monotonic across wake- and tick-driven beats.
func TestRunHeartbeat_StateChangeWakeEmitsImmediateBeat(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	wake := make(chan struct{}, 1)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:          func() time.Time { return now },
		Tick:         tick,
		Publish:      pub.publish,
		GetEnv:       func(string) string { return "" },
		Detect:       func() harness.Detection { return harness.Detection{} },
		StateChanged: wake,
		Hostname:     func() (string, error) { return "h", nil },
		OS:           "darwin",
		Arch:         "arm64",
		PID:          123,
		PPZVersion:   "0.0.0",
		StartedAt:    now,
		IntervalSec:  60,
	})

	pub.waitForCount(t, 1) // startup beat
	wake <- struct{}{}
	msgs := pub.waitForCount(t, 2) // wake beat, no tick sent

	var p HeartbeatPayload
	_ = json.Unmarshal([]byte(msgs[1].payload), &p)
	if p.Seq != 2 {
		t.Errorf("wake beat seq = %d, want 2", p.Seq)
	}
}
