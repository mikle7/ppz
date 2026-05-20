package daemon

import (
	"testing"
	"time"
)

// Tests RS-1, RS-2, RS-A, RS-B, RS-C, RS-7, RS-8, RS-9, RS-11, RS-12,
// RS-13 per docs/specs/session-binding.md (refined, ppid-based).
//
// Dropped from the original draft: RS-3 (tty match — feature removed),
// RS-10 (orphan reparented — collapses into RS-7 fallback now that
// there's no separate ppid-walk daemon-side).

// RS-1: explicit declared session with no binding → uses declared,
// no bound handle. Precedence 1.
func TestResolveSession_ExplicitDeclaredWins(t *testing.T) {
	s := newTestStateForBindings(t)
	got := s.ResolveSession("script-session-XYZ", nil)
	if got.SessionKey != "script-session-XYZ" {
		t.Errorf("SessionKey = %q, want %q", got.SessionKey, "script-session-XYZ")
	}
	if got.BoundHandle != "" {
		t.Errorf("BoundHandle = %q, want empty", got.BoundHandle)
	}
}

// RS-2: ancestor match wins over declared session. Inverted from
// the original draft after PR #75 review — the CLI sends sessionID()
// (legacy sid-N) in every request for back-compat, so letting declared
// win would mean the resolver never engages. Putting the binding
// first means in-pty subprocesses always resolve to their agent
// regardless of what their inherited session id happens to be.
//
// Cost: explicit `PPZ_SESSION=foo ppz …` from inside an agent's pty
// is ignored. Documented in the spec.
func TestResolveSession_AncestorMatchWinsOverDeclared(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := s.ResolveSession("script-session-XYZ", []int{50000, 41203})
	if got.SessionKey != "agent:cindy" {
		t.Errorf("SessionKey = %q, want agent:cindy (binding wins over declared)", got.SessionKey)
	}
	if got.BoundHandle != "cindy" {
		t.Errorf("BoundHandle = %q, want cindy", got.BoundHandle)
	}
}

// RS-A: empty declared, ancestorPIDs[0] direct match (caller IS the
// share, e.g. `ppz terminal share` calling its own daemon).
func TestResolveSession_AncestorMatchDepth0(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := s.ResolveSession("", []int{41203})
	if got.SessionKey != "agent:cindy" {
		t.Errorf("SessionKey = %q, want agent:cindy", got.SessionKey)
	}
	if got.BoundHandle != "cindy" {
		t.Errorf("BoundHandle = %q, want cindy", got.BoundHandle)
	}
}

// RS-A.b: empty declared, ancestor at depth 1 (typical: caller bash →
// share). The most common case.
func TestResolveSession_AncestorMatchDepth1(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Caller chain: bash (50000) → ppz terminal share (41203)
	got := s.ResolveSession("", []int{50000, 41203})
	if got.SessionKey != "agent:cindy" || got.BoundHandle != "cindy" {
		t.Errorf("ResolveSession = %+v, want agent:cindy / cindy at depth 1", got)
	}
}

// RS-B: match at the maximum allowed depth.
func TestResolveSession_AncestorMatchAtMaxDepth(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 99); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Build a chain of exactly MaxPPIDWalkDepth entries, with binding
	// pid at the last position.
	chain := make([]int, 0, MaxPPIDWalkDepth)
	for i := 0; i < MaxPPIDWalkDepth-1; i++ {
		chain = append(chain, 50000+i)
	}
	chain = append(chain, 99) // last position = the binding's pid

	got := s.ResolveSession("", chain)
	if got.BoundHandle != "cindy" {
		t.Errorf("ancestor at depth %d should resolve, got %+v", MaxPPIDWalkDepth-1, got)
	}
}

// RS-C: chain truncated client-side at MaxPPIDWalkDepth — bindings
// past the cap are invisible. Falls through to fallback.
func TestResolveSession_BindingPastMaxDepthFallsThrough(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 99); err != nil {
		t.Fatalf("register: %v", err)
	}

	// The CLI walks up to MaxPPIDWalkDepth. If the share is deeper, the
	// chain passed to the resolver simply doesn't include it. Resolver
	// has nothing to match on.
	chain := []int{50000, 50001, 50002, 50003}
	got := s.ResolveSession("", chain)
	if got.BoundHandle == "cindy" {
		t.Errorf("binding not in chain → must not resolve to cindy, got %+v", got)
	}
}

// RS-7: empty declared, no ancestor match → fallback session key
// (non-empty), empty bound handle. Covers bare shells with no agent.
func TestResolveSession_FallbackWhenNoMatch(t *testing.T) {
	s := newTestStateForBindings(t)

	got := s.ResolveSession("", []int{50000, 50001})
	if got.SessionKey == "" {
		t.Error("fallback SessionKey is empty; resolver must always return a usable key")
	}
	if got.BoundHandle != "" {
		t.Errorf("fallback BoundHandle = %q, want empty", got.BoundHandle)
	}
}

// RS-8: caller with empty chain → still gets a session key (the
// daemon's "default" fallback).
func TestResolveSession_EmptyChainStillResolves(t *testing.T) {
	s := newTestStateForBindings(t)
	got := s.ResolveSession("", nil)
	if got.SessionKey == "" {
		t.Error("empty chain caller must still get a session key")
	}
}

// RS-9: resolver terminates cleanly on a chain that reaches PID 1
// (init) without matching — no infinite loops, no panics.
func TestResolveSession_ChainTerminatesCleanly(t *testing.T) {
	s := newTestStateForBindings(t)
	// Bound runtime ceiling to surface regressions as test timeouts.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.ResolveSession("", []int{50000, 41203, 1})
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ResolveSession did not terminate within 500ms")
	}
}

// RS-11: State.Current override beats binding's BoundHandle. If the
// agent in cindy's pty has run `ppz set handle bob`, the resolver
// still reports BoundHandle="cindy" (the binding's owner) but the
// caller (e.g. send handler) is expected to prefer State.Current.
func TestResolveSession_StateCurrentOverridesBoundHandle(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s.SetCurrent("agent:cindy", "bob"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}

	resolved := s.ResolveSession("", []int{50000, 41203})
	if resolved.SessionKey != "agent:cindy" {
		t.Errorf("SessionKey = %q, want agent:cindy", resolved.SessionKey)
	}
	if resolved.BoundHandle != "cindy" {
		t.Errorf("BoundHandle = %q, want cindy (resolver still reports the binding's owner)", resolved.BoundHandle)
	}

	if got := s.Current("agent:cindy"); got != "bob" {
		t.Errorf("State.Current(agent:cindy) = %q, want bob", got)
	}
}

// RS-12: binding swept (source destroyed) → resolver falls through
// to the fallback path.
func TestResolveSession_DestroyedSourceFallsThrough(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.SweepAgentBindingsForHandle("cindy")

	got := s.ResolveSession("", []int{50000, 41203})
	if got.BoundHandle == "cindy" {
		t.Errorf("after sweep, resolver returned %+v; should fall through", got)
	}
}

// RS-13: two agents' bindings don't cross-contaminate.
func TestResolveSession_TwoAgentsNoCrossContamination(t *testing.T) {
	s := newTestStateForBindings(t)
	allPIDsAlive(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register cindy: %v", err)
	}
	if _, err := s.RegisterAgentBinding("bob", 41204); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	gotA := s.ResolveSession("", []int{50000, 41203})
	gotB := s.ResolveSession("", []int{50001, 41204})

	if gotA.BoundHandle != "cindy" {
		t.Errorf("caller A BoundHandle = %q, want cindy", gotA.BoundHandle)
	}
	if gotB.BoundHandle != "bob" {
		t.Errorf("caller B BoundHandle = %q, want bob", gotB.BoundHandle)
	}
}
