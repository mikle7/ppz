package daemon

// Tests for the pattern detectors. Each test constructs a synthetic
// event slice that exercises one corner of one detector — either the
// happy-path "should fire" or a near-miss "should NOT fire". The
// detectors are pure, so no daemon / ring fixtures are required.

import (
	"strings"
	"testing"
	"time"
)

func TestDetectBurstSwapStorm_Fires(t *testing.T) {
	now := time.Now()
	events := []NATSEvent{
		{Type: "swap", At: now, Caller: "OnRefreshed-callback"},
		{Type: "swap", At: now.Add(100 * time.Millisecond), Caller: "ensureNATS"},
		{Type: "swap", At: now.Add(500 * time.Millisecond), Caller: "ensureNATS-refresh-due"},
	}
	hits := detectBurstSwapStorm(events)
	if len(hits) != 1 {
		t.Fatalf("detectBurstSwapStorm: got %d hits, want 1", len(hits))
	}
	if hits[0].Name != "burst-swap-storm" {
		t.Errorf("hit name = %q, want burst-swap-storm", hits[0].Name)
	}
	for _, c := range []string{"OnRefreshed-callback", "ensureNATS", "ensureNATS-refresh-due"} {
		if !strings.Contains(hits[0].Detail, c) {
			t.Errorf("detail missing caller %q: %s", c, hits[0].Detail)
		}
	}
}

func TestDetectBurstSwapStorm_DoesNotFireOnSpacedSwaps(t *testing.T) {
	// Three swaps but spaced over 10s — that's a normal rotation cadence,
	// not a storm.
	now := time.Now()
	events := []NATSEvent{
		{Type: "swap", At: now, Caller: "ensureNATS"},
		{Type: "swap", At: now.Add(5 * time.Second), Caller: "ensureNATS"},
		{Type: "swap", At: now.Add(10 * time.Second), Caller: "ensureNATS"},
	}
	if hits := detectBurstSwapStorm(events); len(hits) != 0 {
		t.Errorf("expected no hits for spaced swaps, got %d", len(hits))
	}
}

func TestDetectBurstSwapStorm_DoesNotFireOnTwoSwaps(t *testing.T) {
	// Threshold is 3 — two close swaps are not a storm.
	now := time.Now()
	events := []NATSEvent{
		{Type: "swap", At: now, Caller: "ensureNATS"},
		{Type: "swap", At: now.Add(50 * time.Millisecond), Caller: "ensureNATS"},
	}
	if hits := detectBurstSwapStorm(events); len(hits) != 0 {
		t.Errorf("expected no hits below threshold, got %d", len(hits))
	}
}

func TestDetectPostRotationAuthViolation_Fires(t *testing.T) {
	now := time.Now()
	exp := now.Unix() // disconnect at exactly jwt_exp → ±0
	events := []NATSEvent{
		{Type: "disconnect", At: now, Caller: "nats.go", JWTExp: exp, Reason: "EOF"},
		{Type: "closed", At: now.Add(5 * time.Second), Caller: "nats.go", Reason: "nats: Authorization Violation"},
	}
	hits := detectPostRotationAuthViolation(events)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].Name != "post-rotation-auth-violation" {
		t.Errorf("name = %q, want post-rotation-auth-violation", hits[0].Name)
	}
}

func TestDetectPostRotationAuthViolation_SkipsUnrelatedAuthViolation(t *testing.T) {
	// AuthViolation NOT near jwt_exp — operator revoked key mid-session.
	// Detector should not fire.
	now := time.Now()
	exp := now.Add(10 * time.Minute).Unix() // exp far in the future
	events := []NATSEvent{
		{Type: "disconnect", At: now, Caller: "nats.go", JWTExp: exp, Reason: ""},
		{Type: "closed", At: now.Add(time.Second), Caller: "nats.go", Reason: "nats: Authorization Violation"},
	}
	if hits := detectPostRotationAuthViolation(events); len(hits) != 0 {
		t.Errorf("expected no hits for non-rotation auth violation, got %d", len(hits))
	}
}

func TestDetectPostRotationAuthViolation_NoJWTExp(t *testing.T) {
	// Pre-Phase-0 events have JWTExp=0; detector should skip rather than
	// false-fire on the zero-value.
	now := time.Now()
	events := []NATSEvent{
		{Type: "disconnect", At: now, Caller: "nats.go", JWTExp: 0},
		{Type: "closed", At: now.Add(time.Second), Caller: "nats.go", Reason: "nats: Authorization Violation"},
	}
	if hits := detectPostRotationAuthViolation(events); len(hits) != 0 {
		t.Errorf("expected no hits when jwt_exp is unknown, got %d", len(hits))
	}
}

func TestDetectPatterns_OrdersChronologically(t *testing.T) {
	// Two detectors firing on the same event slice — output must be
	// sorted by At even when the detectors run in registration order.
	now := time.Now()
	exp := now.Add(2 * time.Minute).Unix()
	earlyAuth := now
	storm := now.Add(time.Minute)
	events := []NATSEvent{
		{Type: "disconnect", At: earlyAuth, JWTExp: earlyAuth.Unix(), Caller: "nats.go"},
		{Type: "closed", At: earlyAuth.Add(time.Second), Reason: "Authorization Violation"},
		{Type: "swap", At: storm, Caller: "ensureNATS"},
		{Type: "swap", At: storm.Add(100 * time.Millisecond), Caller: "OnRefreshed-callback"},
		{Type: "swap", At: storm.Add(200 * time.Millisecond), Caller: "ensureNATS-refresh-due"},
	}
	_ = exp
	hits := detectPatterns(events)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].At.After(hits[1].At) {
		t.Errorf("patterns not chronological: %s then %s", hits[0].At, hits[1].At)
	}
}
