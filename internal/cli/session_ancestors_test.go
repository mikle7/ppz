package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// Tests AW-1..AW-4 from docs/specs/session-binding.md.

// AW-1: walk depth 1 returns [self, parent] using real syscall.Getppid.
func TestAncestorPIDs_RealPPIDAtDepth1(t *testing.T) {
	pids := ancestorPIDs()
	if len(pids) < 2 {
		t.Fatalf("ancestorPIDs returned %d entries, want >= 2 (self + parent)", len(pids))
	}
	if pids[0] != os.Getpid() {
		t.Errorf("pids[0] = %d, want self (%d)", pids[0], os.Getpid())
	}
	if pids[1] != syscall.Getppid() {
		t.Errorf("pids[1] = %d, want parent (%d)", pids[1], syscall.Getppid())
	}
}

// AW-2: walk terminates at PID 1 (init). Inject a fake chain to drive
// deterministically.
func TestAncestorPIDs_TerminatesAtInit(t *testing.T) {
	orig := parentPIDForPID
	t.Cleanup(func() { parentPIDForPID = orig })

	// Fake chain: real ppid → 100 → 1.
	parentPIDForPID = func(pid int) (int, error) {
		switch pid {
		case syscall.Getppid():
			return 100, nil
		case 100:
			return 1, nil
		case 1:
			return 0, nil
		}
		return 0, errors.New("unknown pid in fake chain")
	}

	pids := ancestorPIDs()
	// Expect: [self, real_ppid, 100, 1]
	if len(pids) < 4 {
		t.Fatalf("expected at least 4 entries, got %d: %v", len(pids), pids)
	}
	if pids[len(pids)-1] != 1 {
		t.Errorf("chain doesn't terminate at PID 1: %v", pids)
	}
}

// AW-3: walk terminates cleanly if the platform helper errors mid-walk.
func TestAncestorPIDs_StopsOnHelperError(t *testing.T) {
	orig := parentPIDForPID
	t.Cleanup(func() { parentPIDForPID = orig })

	parentPIDForPID = func(pid int) (int, error) {
		// First non-self/non-ppid call errors out.
		return 0, errors.New("permission denied (or dead pid)")
	}

	pids := ancestorPIDs()
	// Should still have at least [self, real_ppid].
	if len(pids) < 2 {
		t.Errorf("expected at least [self, parent], got %v", pids)
	}
	// Subsequent walk hops fail → chain stops cleanly.
	if len(pids) > 3 {
		t.Errorf("walk should have stopped on error, got %v", pids)
	}
}

// AW-4: walk respects the depth cap.
func TestAncestorPIDs_RespectsDepthCap(t *testing.T) {
	orig := parentPIDForPID
	t.Cleanup(func() { parentPIDForPID = orig })

	// Build an infinite chain so only the cap stops it.
	parentPIDForPID = func(pid int) (int, error) {
		return pid + 1, nil
	}

	pids := ancestorPIDs()
	if len(pids) > MaxAncestorWalkDepth {
		t.Errorf("walk returned %d pids, cap is %d", len(pids), MaxAncestorWalkDepth)
	}
}

// AW-5: walk terminates cleanly when an ancestor pid vanishes between
// hops. Distinct from AW-3 (helper errors on first call) — here the
// helper succeeds for the first 2 hops, then the next ancestor has
// already been reaped (e.g. short-lived parent shell exited and got
// cleaned up before our walk reached its parent). Race window is
// microseconds in practice but real.
func TestAncestorPIDs_VanishMidWalk(t *testing.T) {
	orig := parentPIDForPID
	t.Cleanup(func() { parentPIDForPID = orig })

	// Hops 1-2 succeed; hop 3 returns "no such process".
	calls := 0
	parentPIDForPID = func(pid int) (int, error) {
		calls++
		switch calls {
		case 1:
			return 200, nil
		case 2:
			return 300, nil
		default:
			return 0, errors.New("no such process")
		}
	}

	pids := ancestorPIDs()
	// Should have [self, real_ppid, 200, 300] then the walk stops.
	if len(pids) < 4 {
		t.Errorf("expected at least 4 entries (self, ppid, 200, 300), got %v", pids)
	}
	if got, want := len(pids), 4; got != want {
		t.Errorf("walk should stop on vanish at hop 3, got %d entries: %v", got, pids)
	}
	// Sanity: no panic, no infinite loop — the test reaching this line
	// is the assertion.
}
