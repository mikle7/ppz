package cli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pipescloud/ppz/internal/harness"
)

// HeartbeatIntervalSec is the canonical heartbeat cadence. The ticker
// runs every HeartbeatIntervalSec seconds, the payload's interval_sec
// field carries the same value so consumers (notably `ppz who`'s
// classifier) can scale online/stale/offline thresholds proportionally.
// Keep this as the single source of truth — three earlier copies of
// `60` (ticker, payload field, doc) drifted apart in review.
const HeartbeatIntervalSec = 60

// HeartbeatPayload is the wire shape published to <handle>.heartbeat.
// Every beat is fully self-describing — there is no "hello + delta"
// split — so consumers (notably `ppz who`) can read a single message
// and have everything needed to render an agent's identity + status.
type HeartbeatPayload struct {
	TS            string `json:"ts"`
	Seq           uint64 `json:"seq"`
	Harness       string `json:"harness"`
	HarnessSource string `json:"harness_source"` // "detected" | "env" | ""
	AgentState    string `json:"agent_state"`    // "" | "idle" | "working" | "blocked"
	ChildPID      int    `json:"child_pid"`      // foreground pid when a harness is detected
	Model         string `json:"model"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	PID           int    `json:"pid"`
	PPZVersion    string `json:"ppz_version"`
	StartedAt     string `json:"started_at"`
	IntervalSec   int    `json:"interval_sec"`
}

// heartbeatInputs is what the runtime collects per beat. Kept as an
// explicit struct (not function args) so the heartbeat loop and the
// pure builder agree on field shape without ordering hazards.
type heartbeatInputs struct {
	Now           time.Time
	Seq           uint64
	Harness       string
	HarnessSource string
	AgentState    string
	ChildPID      int
	Model         string
	Hostname      string
	OS            string
	Arch          string
	PID           int
	PPZVersion    string
	StartedAt     time.Time
	IntervalSec   int
}

// heartbeatDeps is the seam runHeartbeat reads everything it needs
// from. Real callers (the pty wrapper) pass concrete os/runtime/clock
// implementations; tests pass fakes so the loop can be exercised
// without standing up a daemon or waiting on a real ticker.
type heartbeatDeps struct {
	Now     func() time.Time
	Tick    <-chan time.Time
	Publish func(handle, channel, payload string) error
	GetEnv  func(string) string
	// Detect returns the live harness detection snapshot for the wrapped
	// PTY (zero Detection when nothing is identified). nil means the
	// caller has no detector wired (tests, future non-PTY callers).
	Detect func() harness.Detection
	// StateChanged wakes the loop to emit an immediate out-of-cycle beat
	// when the detected agent state transitions — 60s ticks are too
	// coarse for `ppz who`'s working/idle column. nil disables wakes.
	StateChanged <-chan struct{}
	Hostname     func() (string, error)
	OS           string
	Arch         string
	PID          int
	PPZVersion   string
	StartedAt    time.Time
	IntervalSec  int
}

// runHeartbeat publishes a heartbeat to <handle>.heartbeat once
// immediately (so consumers see the agent within milliseconds of boot,
// not after a full interval), then once per Tick. Exits on ctx cancel.
// Publish errors are swallowed: a missed beat is recoverable on the
// next tick, so we never want a transient daemon hiccup to take down
// the agent.
func runHeartbeat(ctx context.Context, handle string, deps heartbeatDeps) {
	var seq uint64
	emit := func() {
		seq++
		hostname, _ := deps.Hostname()
		var det harness.Detection
		if deps.Detect != nil {
			det = deps.Detect()
		}
		name, source := resolveHarness(det.Harness, deps.GetEnv("PPZ_AGENT_HARNESS"))
		raw, err := json.Marshal(buildHeartbeatPayload(heartbeatInputs{
			Now:           deps.Now(),
			Seq:           seq,
			Harness:       name,
			HarnessSource: source,
			AgentState:    string(det.State),
			ChildPID:      det.ChildPID,
			Model:         deps.GetEnv("PPZ_AGENT_MODEL"),
			Hostname:      hostname,
			OS:            deps.OS,
			Arch:          deps.Arch,
			PID:           deps.PID,
			PPZVersion:    deps.PPZVersion,
			StartedAt:     deps.StartedAt,
			IntervalSec:   deps.IntervalSec,
		}))
		if err != nil {
			return
		}
		_ = deps.Publish(handle, "heartbeat", string(raw))
	}

	emit()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deps.Tick:
			emit()
		case <-deps.StateChanged: // nil channel when no detector: blocks forever
			emit()
		}
	}
}

// resolveHarness picks the harness to stamp on a beat. Live detection
// wins over the launch-time env var: the env var is set once by `ppz
// agent` and survives the harness exiting back to a shell, while
// detection tracks the actual foreground. The returned source records
// which side won ("detected" / "env" / "").
func resolveHarness(detected, env string) (name, source string) {
	if detected != "" {
		return detected, "detected"
	}
	if env != "" {
		return env, "env"
	}
	return "", ""
}

// buildHeartbeatPayload is the pure transform from runtime inputs to
// wire shape. Times are rendered as RFC 3339 in UTC so the payload
// reads identically regardless of the agent's local zone.
func buildHeartbeatPayload(in heartbeatInputs) HeartbeatPayload {
	return HeartbeatPayload{
		TS:            in.Now.UTC().Format(time.RFC3339),
		Seq:           in.Seq,
		Harness:       in.Harness,
		HarnessSource: in.HarnessSource,
		AgentState:    in.AgentState,
		ChildPID:      in.ChildPID,
		Model:         in.Model,
		Hostname:      in.Hostname,
		OS:            in.OS,
		Arch:          in.Arch,
		PID:           in.PID,
		PPZVersion:    in.PPZVersion,
		StartedAt:     in.StartedAt.UTC().Format(time.RFC3339),
		IntervalSec:   in.IntervalSec,
	}
}
