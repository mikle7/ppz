package daemon

import "testing"

// allPIDsAlive overrides the package-level pidAlive var so tests that
// use synthetic share pids (not real processes) get past the lazy-
// validation drop in the resolver. Auto-restores via t.Cleanup.
func allPIDsAlive(t *testing.T) {
	t.Helper()
	orig := pidAlive
	pidAlive = func(int) bool { return true }
	t.Cleanup(func() { pidAlive = orig })
}
