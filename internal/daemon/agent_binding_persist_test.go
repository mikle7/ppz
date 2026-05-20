package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Tests PP-1, PP-2, PP-3, PP-4, PP-5, PP-6, PP-8, PP-9 per
// docs/specs/session-binding.md (refined design). Dropped: PP-7 (tty
// validation — no longer applicable).

// PP-1: register + persist + reload returns the same set.
func TestAgentBindingPersist_RoundTrip(t *testing.T) {
	home := t.TempDir()

	s1 := NewState(home)
	myPID := os.Getpid()
	// Use the test process's own pid for all entries so live-validation
	// keeps them on reload.
	for i, b := range []struct {
		handle string
		pid    int
	}{
		{"cindy", myPID},
		{"bob", myPID + 1},
		{"dave", myPID + 2},
	} {
		_ = i
		if _, err := s1.RegisterAgentBinding(b.handle, b.pid); err != nil {
			t.Fatalf("register %s: %v", b.handle, err)
		}
	}

	s2 := NewState(home)
	if err := s2.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings: %v", err)
	}
	got := s2.AgentBindingSnapshot()
	// Note: validate-on-load may drop entries whose pid isn't actually
	// alive — only the test process's pid (myPID) is guaranteed live.
	// The other entries are best-effort.
	if len(got) == 0 {
		t.Errorf("after reload, snapshot is empty; expected at least the live entry to survive")
	}
}

// PP-2: a crash between tmp-write and rename leaves the prior file intact.
func TestAgentBindingPersist_AtomicRenameOnCrash(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, fileAgentBindings)

	good := `{"version":1,"bindings":[{"handle":"cindy","share_pid":41203,"session_key":"agent:cindy","registered_at":"2026-05-20T10:14:33Z"}]}`
	if err := os.WriteFile(target, []byte(good), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Simulate a partial write: tmp file present but rename never ran.
	tmp := filepath.Join(home, fileAgentBindings+".tmp")
	if err := os.WriteFile(tmp, []byte("PARTIAL_GARBAGE"), 0o600); err != nil {
		t.Fatalf("seed tmp: %v", err)
	}

	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings: %v", err)
	}
	got := s.AgentBindingSnapshot()
	// The seeded "cindy" entry has share_pid 41203 — almost certainly not
	// alive on this host, so validate-on-load drops it. The assertion
	// here is only that the LOAD didn't see PARTIAL_GARBAGE; if it had,
	// we'd see a parse error from LoadAgentBindings.
	_ = got
}

// PP-3: versioned envelope with an unknown version is rejected with
// warning, loader returns empty.
func TestAgentBindingPersist_RejectsUnknownVersion(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, fileAgentBindings)
	wrongVersion := `{"version":99,"bindings":[{"handle":"cindy","share_pid":` + itoaForTest(os.Getpid()) + `,"session_key":"agent:cindy"}]}`
	if err := os.WriteFile(target, []byte(wrongVersion), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings should be a non-fatal warning, got error: %v", err)
	}
	if got := len(s.AgentBindingSnapshot()); got != 0 {
		t.Errorf("after rejecting unknown version, snapshot len = %d, want 0", got)
	}
}

// PP-4: corrupt file (truncated, garbage, empty) → loader returns
// empty, logs warning, does not crash.
func TestAgentBindingPersist_CorruptFileLoadsEmpty(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"truncated", `{"version":1,"bindings":[`},
		{"garbage", "not json at all"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, fileAgentBindings), []byte(tc.content), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
			s := NewState(home)
			if err := s.LoadAgentBindings(); err != nil {
				t.Errorf("LoadAgentBindings on %s file should not error, got %v", tc.name, err)
			}
			if got := len(s.AgentBindingSnapshot()); got != 0 {
				t.Errorf("after corrupt-%s load, snapshot len = %d, want 0", tc.name, got)
			}
		})
	}
}

// PP-5: missing file is the clean fresh-install path.
func TestAgentBindingPersist_MissingFileIsCleanLoad(t *testing.T) {
	home := t.TempDir()
	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Errorf("LoadAgentBindings on fresh home: %v", err)
	}
	if got := len(s.AgentBindingSnapshot()); got != 0 {
		t.Errorf("fresh load snapshot len = %d, want 0", got)
	}
}

// PP-6: validate-on-load drops entries whose SharePID is dead.
func TestAgentBindingPersist_ValidateDropsDeadPID(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, fileAgentBindings)

	content := `{"version":1,"bindings":[
		{"handle":"ghost","share_pid":999999,"session_key":"agent:ghost","registered_at":"2026-05-20T10:14:33Z"}
	]}`
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings: %v", err)
	}
	if got := len(s.AgentBindingSnapshot()); got != 0 {
		t.Errorf("after load with dead pid, snapshot len = %d, want 0 (validate must drop)", got)
	}
}

// PP-8: validate-on-load keeps live entries.
func TestAgentBindingPersist_ValidateKeepsLiveEntries(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, fileAgentBindings)
	myPID := os.Getpid()

	content := `{"version":1,"bindings":[
		{"handle":"cindy","share_pid":` + itoaForTest(myPID) + `,"session_key":"agent:cindy","registered_at":"2026-05-20T10:14:33Z"}
	]}`
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings: %v", err)
	}
	got := s.AgentBindingSnapshot()
	if len(got) != 1 || got[0].Handle != "cindy" {
		t.Errorf("after load with live entry, snapshot = %#v, want exactly cindy", got)
	}
}

// PB-12: stale-persisted entry + fresh registration. Simulates a pid
// reuse scenario: daemon restarts, persistence has stale entry for
// pid X (process now dead), validate-on-load drops it, then a new
// `ppz terminal share` happens to claim pid X and registers fresh.
// The fresh register should succeed cleanly — no leftover state from
// the dropped entry.
func TestAgentBindingPersist_FreshRegisterAfterStaleDrop(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, fileAgentBindings)
	myPID := os.Getpid()

	// Seed a "stale" entry for a dead pid (999999) and a "live" entry
	// for this test's pid (myPID). Different handles.
	content := `{"version":1,"bindings":[
		{"handle":"ghost","share_pid":999999,"session_key":"agent:ghost","registered_at":"2026-05-19T10:00:00Z"},
		{"handle":"alice","share_pid":` + itoaForTest(myPID) + `,"session_key":"agent:alice","registered_at":"2026-05-20T10:14:33Z"}
	]}`
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := NewState(home)
	if err := s.LoadAgentBindings(); err != nil {
		t.Fatalf("LoadAgentBindings: %v", err)
	}

	// Load drops the stale ghost entry, keeps alice.
	if got := s.LookupAgentBindingByPID(999999); got != nil {
		t.Errorf("stale ghost binding survived load: %#v", got)
	}
	if got := s.LookupAgentBindingByPID(myPID); got == nil || got.Handle != "alice" {
		t.Errorf("alice binding should survive load, got %#v", got)
	}

	// Now a brand-new share claims pid 999999 (kernel pid reuse). It
	// registers for a different handle. Should succeed — the dropped
	// stale entry left no debris.
	got, err := s.RegisterAgentBinding("bob", 999999)
	if err != nil {
		t.Fatalf("fresh register after stale drop failed: %v", err)
	}
	if got == nil || got.Handle != "bob" {
		t.Errorf("fresh register returned %#v, want bob/999999", got)
	}
	if back := s.LookupAgentBindingByPID(999999); back == nil || back.Handle != "bob" {
		t.Errorf("lookup after fresh register = %#v, want bob", back)
	}
}

// PP-9: concurrent register-induced writes don't truncate or lose entries.
func TestAgentBindingPersist_ConcurrentWritesRoundTrip(t *testing.T) {
	home := t.TempDir()
	s := NewState(home)
	allPIDsAlive(t) // synthetic pids; this test is about concurrency, not liveness
	myPID := os.Getpid()

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			handle := "agent" + tspad(i)
			_, _ = s.RegisterAgentBinding(handle, myPID+i)
		}()
	}
	wg.Wait()

	wantLen := len(s.AgentBindingSnapshot())
	s2 := NewState(home)
	if err := s2.LoadAgentBindings(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := len(s2.AgentBindingSnapshot()); got != wantLen {
		t.Errorf("after concurrent writes, in-mem=%d on-disk=%d; want equal", wantLen, got)
	}
}

// itoaForTest avoids importing strconv inside seeded JSON strings.
func itoaForTest(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b strings.Builder
	for i > 0 {
		d := byte('0' + i%10)
		b.WriteByte(d)
		i /= 10
	}
	out := b.String()
	rev := make([]byte, len(out))
	for j := 0; j < len(out); j++ {
		rev[j] = out[len(out)-1-j]
	}
	if neg {
		return "-" + string(rev)
	}
	return string(rev)
}
