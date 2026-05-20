package daemon

import (
	"errors"
	"sync"
	"testing"
)

// Tests PB-1, PB-2, PB-5, PB-6, PB-9, PB-10, PB-11 per
// docs/specs/session-binding.md (refined, ppid-based design).
//
// Dropped from the original draft: PB-3 (tty conflict), PB-4 (lookup by
// tty), PB-7/PB-8 (heartbeat) — features removed.

func newTestStateForBindings(t *testing.T) *State {
	t.Helper()
	dir := t.TempDir()
	return NewState(dir)
}

// PB-1: re-registering the same (pid, handle) tuple is idempotent.
func TestAgentBinding_RegisterIdempotent(t *testing.T) {
	s := newTestStateForBindings(t)

	first, err := s.RegisterAgentBinding("cindy", 41203)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first == nil || first.Handle != "cindy" || first.SharePID != 41203 {
		t.Fatalf("first register returned %#v", first)
	}
	if first.SessionKey != "agent:cindy" {
		t.Errorf("SessionKey = %q, want %q", first.SessionKey, "agent:cindy")
	}

	second, err := s.RegisterAgentBinding("cindy", 41203)
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if second == nil {
		t.Fatal("re-register returned nil")
	}

	if got := len(s.AgentBindingSnapshot()); got != 1 {
		t.Errorf("snapshot len = %d, want 1 (re-register must not add a second entry)", got)
	}
}

// PB-2: registering the same SharePID against a different handle is
// an error. Caller is expected to unregister first.
func TestAgentBinding_RegisterPIDConflictDifferentHandle(t *testing.T) {
	s := newTestStateForBindings(t)

	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := s.RegisterAgentBinding("bob", 41203)
	if !errors.Is(err, ErrBindingConflict) {
		t.Fatalf("re-register with different handle: err = %v, want ErrBindingConflict", err)
	}

	got := s.LookupAgentBindingByPID(41203)
	if got == nil || got.Handle != "cindy" {
		t.Errorf("after conflict, binding handle = %v, want cindy (original survives)", got)
	}
}

// PB-5: lookup by pid returns the registered binding; unknown pid returns nil.
func TestAgentBinding_LookupByPID(t *testing.T) {
	s := newTestStateForBindings(t)

	if got := s.LookupAgentBindingByPID(41203); got != nil {
		t.Fatalf("lookup empty state returned %#v, want nil", got)
	}

	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := s.LookupAgentBindingByPID(41203)
	if got == nil || got.Handle != "cindy" {
		t.Fatalf("lookup after register = %#v, want Handle=cindy", got)
	}
	if got := s.LookupAgentBindingByPID(99999); got != nil {
		t.Errorf("lookup unknown pid = %#v, want nil", got)
	}
}

// PB-6: unregister drops the binding. Unknown pid is a clean no-op.
func TestAgentBinding_Unregister(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	s.UnregisterAgentBinding(41203)
	if got := s.LookupAgentBindingByPID(41203); got != nil {
		t.Errorf("after unregister, lookup = %#v, want nil", got)
	}

	// Unregistering an unknown pid is a no-op.
	s.UnregisterAgentBinding(99999)
}

// PB-9: concurrent register/unregister/lookup must be race-free under
// `go test -race`.
func TestAgentBinding_ConcurrentSafe(t *testing.T) {
	s := newTestStateForBindings(t)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 3)

	for i := 0; i < n; i++ {
		pid := 10000 + i
		handle := "agent" + tspad(i)

		go func() {
			defer wg.Done()
			_, _ = s.RegisterAgentBinding(handle, pid)
		}()
		go func() {
			defer wg.Done()
			_ = s.LookupAgentBindingByPID(pid)
		}()
		go func() {
			defer wg.Done()
			s.UnregisterAgentBinding(pid)
		}()
	}
	wg.Wait()
	// No assertion on final state — interleaving is non-deterministic.
	// The point is to surface data races to `go test -race`.
}

// PB-10: destroying source `cindy` cascades to drop her binding.
func TestAgentBinding_SweepForDestroyedSource(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register cindy: %v", err)
	}
	if _, err := s.RegisterAgentBinding("bob", 41204); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	removed := s.SweepAgentBindingsForHandle("cindy")
	if removed != 1 {
		t.Errorf("sweep removed %d binding(s), want 1", removed)
	}
	if got := s.LookupAgentBindingByPID(41203); got != nil {
		t.Errorf("after sweep, cindy's binding survives: %#v", got)
	}
	if got := s.LookupAgentBindingByPID(41204); got == nil {
		t.Errorf("after sweep of cindy, bob's binding should remain, got nil")
	}

	if removed := s.SweepAgentBindingsForHandle("nobody"); removed != 0 {
		t.Errorf("sweep of unbound handle removed %d, want 0", removed)
	}
}

// PB-11: after sweep, lookup returns nil — resolver's ancestor-match
// precedence falls through rather than returning a binding for a
// deleted source.
func TestAgentBinding_LookupAfterSweepReturnsNil(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	s.SweepAgentBindingsForHandle("cindy")
	if got := s.LookupAgentBindingByPID(41203); got != nil {
		t.Errorf("lookup-by-pid after sweep = %#v, want nil", got)
	}
}

// tspad zero-pads i for stable handle names in concurrent tests.
func tspad(i int) string {
	s := ""
	for v := i; v > 0; v /= 10 {
		s = string(rune('0'+v%10)) + s
	}
	if s == "" {
		s = "0"
	}
	return s
}
