package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests AC-1..AC-6 and BC-1..BC-3 from docs/specs/session-binding.md.
//
// AC tests exercise auto-write-current: when ResolveSession returns
// BoundHandle != "" and no current is set, daemon auto-populates
// current[SessionKey] = BoundHandle.
//
// BC tests guard wire backwards compat during rollout.

// --- AC-1..AC-6 ----------------------------------------------------------

// AC-1: resolver returns BoundHandle, State.Current unset → auto-write.
func TestAutoWriteCurrent_PopulatesOnFirstResolve(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := s.ResolveSessionWithAutoWrite("", []int{50000, 41203})
	if got.SessionKey != "agent:cindy" {
		t.Fatalf("SessionKey = %q, want agent:cindy", got.SessionKey)
	}
	if cur := s.Current("agent:cindy"); cur != "cindy" {
		t.Errorf("State.Current(agent:cindy) = %q, want cindy (auto-write should populate)", cur)
	}
}

// AC-2: auto-write persists to current.json.
func TestAutoWriteCurrent_PersistsToDisk(t *testing.T) {
	home := t.TempDir()
	s := NewState(home)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})

	s2 := NewState(home)
	if err := s2.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk: %v", err)
	}
	if cur := s2.Current("agent:cindy"); cur != "cindy" {
		path := filepath.Join(home, "current.json")
		body, _ := os.ReadFile(path)
		t.Errorf("after reload, current(agent:cindy) = %q, want cindy; current.json = %s", cur, string(body))
	}
}

// AC-3: repeated resolves don't churn the persistence file.
func TestAutoWriteCurrent_NoChurnOnRepeatedResolve(t *testing.T) {
	home := t.TempDir()
	s := NewState(home)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})

	currentPath := filepath.Join(home, "current.json")
	info1, err := os.Stat(currentPath)
	if err != nil {
		t.Skipf("current.json not present after auto-write: %v", err)
	}

	for i := 0; i < 5; i++ {
		_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})
	}

	info2, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat after repeats: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("repeated resolve rewrote current.json (mtime %v → %v); should be idempotent on disk", info1.ModTime(), info2.ModTime())
	}
}

// AC-4: existing current is preserved — auto-write doesn't overwrite.
func TestAutoWriteCurrent_DoesNotOverwriteExisting(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s.SetCurrent("agent:cindy", "bob"); err != nil {
		t.Fatalf("seed SetCurrent: %v", err)
	}

	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})

	if cur := s.Current("agent:cindy"); cur != "bob" {
		t.Errorf("after auto-write attempt, current = %q, want bob", cur)
	}
}

// AC-5: `ppz set handle bob` overrides cleanly. Binding unchanged.
func TestAutoWriteCurrent_SetHandleOverridesAfter(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})
	if cur := s.Current("agent:cindy"); cur != "cindy" {
		t.Fatalf("setup: current = %q, want cindy", cur)
	}

	if err := s.SetCurrent("agent:cindy", "bob"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}

	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})
	if cur := s.Current("agent:cindy"); cur != "bob" {
		t.Errorf("after set+resolve, current = %q, want bob", cur)
	}

	if b := s.LookupAgentBindingByPID(41203); b == nil || b.Handle != "cindy" {
		t.Errorf("binding mutated by set handle: %#v", b)
	}
}

// AC-6: `ppz unset handle` clears current; next resolve re-fires auto-write.
func TestAutoWriteCurrent_UnsetHandleAllowsReAutoWrite(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})

	if err := s.ClearCurrent("agent:cindy"); err != nil {
		t.Fatalf("ClearCurrent: %v", err)
	}
	if cur := s.Current("agent:cindy"); cur != "" {
		t.Fatalf("after clear, current = %q, want empty", cur)
	}

	_ = s.ResolveSessionWithAutoWrite("", []int{50000, 41203})
	if cur := s.Current("agent:cindy"); cur != "cindy" {
		t.Errorf("after clear+resolve, current = %q, want cindy (auto-write re-fires)", cur)
	}
}

// --- BC-1..BC-3 ----------------------------------------------------------

// BC-1: old CLI sends Session: "sid-12345" → precedence 1 legacy passthrough.
func TestBackwardsCompat_OldCLISessionFormat(t *testing.T) {
	s := newTestStateForBindings(t)
	if err := s.SetCurrent("sid-12345", "alice"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got := s.ResolveSession("sid-12345", nil)
	if got.SessionKey != "sid-12345" {
		t.Errorf("SessionKey = %q, want sid-12345 (legacy passthrough)", got.SessionKey)
	}
	if cur := s.Current(got.SessionKey); cur != "alice" {
		t.Errorf("legacy current lookup = %q, want alice", cur)
	}
}

// BC-2: new CLI sends empty Session + populated AncestorPIDs → engages resolver.
func TestBackwardsCompat_NewCLIEmptyEngagesResolver(t *testing.T) {
	s := newTestStateForBindings(t)
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := s.ResolveSession("", []int{50000, 41203})
	if got.SessionKey != "agent:cindy" {
		t.Errorf("empty declared + ancestors → resolver should yield agent:cindy; got %q", got.SessionKey)
	}
}

// BC-3: documentation guard — new CLI talking to old daemon is pinned
// by WP-2 (cliproto round-trip). Old daemon ignores AncestorPIDs and
// reads Session (which old CLI also populated). New CLI sends both.
func TestBackwardsCompat_NewCLIToOldDaemonContract(t *testing.T) {
	t.Log("Wire-compat: see WP-2 / WP-3 for the on-wire pin.")
}
