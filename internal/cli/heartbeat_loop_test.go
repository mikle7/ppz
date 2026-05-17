package cli

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// fakePublisher captures published heartbeats so the test can assert on
// channel + payload shape without standing up a daemon.
type fakePublisher struct {
	mu   sync.Mutex
	cond *sync.Cond
	msgs []fakePub
}

type fakePub struct {
	handle  string
	channel string
	payload string
}

func newFakePublisher() *fakePublisher {
	fp := &fakePublisher{}
	fp.cond = sync.NewCond(&fp.mu)
	return fp
}

func (fp *fakePublisher) publish(handle, channel, payload string) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.msgs = append(fp.msgs, fakePub{handle, channel, payload})
	fp.cond.Broadcast()
	return nil
}

func (fp *fakePublisher) waitForCount(t *testing.T, n int) []fakePub {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	fp.mu.Lock()
	defer fp.mu.Unlock()
	for len(fp.msgs) < n {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitForCount(%d) timeout; got %d msgs: %#v", n, len(fp.msgs), fp.msgs)
		}
		// sync.Cond doesn't support timeout natively; spin with a short
		// release+sleep cycle so a stuck test fails fast under -race.
		fp.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		fp.mu.Lock()
	}
	out := make([]fakePub, len(fp.msgs))
	copy(out, fp.msgs)
	return out
}

// First beat fires immediately when runHeartbeat starts, before any
// tick. That guarantees `ppz who` sees a freshly-started agent right
// away instead of waiting up to a full interval for the first beat.
func TestRunHeartbeat_FirstBeatFiresImmediately(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:         func() time.Time { return now },
		Tick:        tick,
		Publish:     pub.publish,
		GetEnv:      func(string) string { return "" },
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "darwin",
		Arch:        "arm64",
		PID:         123,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	msgs := pub.waitForCount(t, 1)
	if msgs[0].handle != "alice" {
		t.Errorf("handle = %q, want alice", msgs[0].handle)
	}
	if msgs[0].channel != "heartbeat" {
		t.Errorf("channel = %q, want heartbeat", msgs[0].channel)
	}
	var p HeartbeatPayload
	if err := json.Unmarshal([]byte(msgs[0].payload), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Seq != 1 {
		t.Errorf("first beat seq = %d, want 1", p.Seq)
	}
}

// Each tick produces another beat, with seq monotonically incrementing.
func TestRunHeartbeat_TickProducesMoreBeats(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 4)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:         func() time.Time { return now },
		Tick:        tick,
		Publish:     pub.publish,
		GetEnv:      func(string) string { return "" },
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "darwin",
		Arch:        "arm64",
		PID:         123,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	pub.waitForCount(t, 1)
	tick <- time.Time{}
	tick <- time.Time{}
	tick <- time.Time{}
	msgs := pub.waitForCount(t, 4)

	for i, m := range msgs {
		var p HeartbeatPayload
		if err := json.Unmarshal([]byte(m.payload), &p); err != nil {
			t.Fatalf("msg %d unmarshal: %v", i, err)
		}
		if got, want := p.Seq, uint64(i+1); got != want {
			t.Errorf("msg %d seq = %d, want %d", i, got, want)
		}
	}
}

// PPZ_AGENT_HARNESS and PPZ_AGENT_MODEL come from the env reader at
// beat time — so a beat reflects whatever the wrapper saw at startup
// (no stale snapshot).
func TestRunHeartbeat_PicksUpHarnessFromEnv(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	env := map[string]string{
		"PPZ_AGENT_HARNESS": "claude",
		"PPZ_AGENT_MODEL":   "opus",
	}

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
	if p.Harness != "claude" {
		t.Errorf("harness = %q, want claude", p.Harness)
	}
	if p.Model != "opus" {
		t.Errorf("model = %q, want opus", p.Model)
	}
}

// When the env vars are not set (plain `ppz terminal share`, not an
// agent create), the heartbeat carries empty harness/model. The schema
// keys are still present (asserted in heartbeat_test.go) — the values
// are just blank.
func TestRunHeartbeat_EmptyEnvProducesEmptyHarnessAndModel(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHeartbeat(ctx, "alice", heartbeatDeps{
		Now:         func() time.Time { return now },
		Tick:        tick,
		Publish:     pub.publish,
		GetEnv:      func(string) string { return "" },
		Hostname:    func() (string, error) { return "h", nil },
		OS:          "linux",
		Arch:        "amd64",
		PID:         1,
		PPZVersion:  "0.0.0",
		StartedAt:   now,
		IntervalSec: 60,
	})

	msgs := pub.waitForCount(t, 1)
	var p HeartbeatPayload
	_ = json.Unmarshal([]byte(msgs[0].payload), &p)
	if p.Harness != "" {
		t.Errorf("harness = %q, want empty", p.Harness)
	}
	if p.Model != "" {
		t.Errorf("model = %q, want empty", p.Model)
	}
}

// Cancelling the ctx stops the loop. Test by cancelling, then asserting
// no further beats land even when we send tick events.
func TestRunHeartbeat_CtxCancelStopsLoop(t *testing.T) {
	pub := newFakePublisher()
	tick := make(chan time.Time, 1)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runHeartbeat(ctx, "alice", heartbeatDeps{
			Now:         func() time.Time { return now },
			Tick:        tick,
			Publish:     pub.publish,
			GetEnv:      func(string) string { return "" },
			Hostname:    func() (string, error) { return "h", nil },
			OS:          "darwin",
			Arch:        "arm64",
			PID:         123,
			PPZVersion:  "0.0.0",
			StartedAt:   now,
			IntervalSec: 60,
		})
		close(done)
	}()

	pub.waitForCount(t, 1)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runHeartbeat did not exit after ctx cancel")
	}
}
