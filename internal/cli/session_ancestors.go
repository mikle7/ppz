package cli

import (
	"errors"
	"os"
	"syscall"
)

// MaxAncestorWalkDepth caps how far the CLI walks its own ppid chain.
// Mirrors the daemon-side MaxPPIDWalkDepth so client + server share
// the same depth budget. See docs/specs/session-binding.md.
const MaxAncestorWalkDepth = 8

// ancestorPIDs returns the calling process's ancestor pid chain,
// starting with the current pid and walking up via os.Getppid() for
// depth 1 and parentPIDForPID() for deeper hops. Stops at PID 1
// (init), at depth MaxAncestorWalkDepth, or when the platform helper
// errors out.
//
// Used to populate the AncestorPIDs field on session-using IPC
// requests so the daemon can resolve the caller's agent binding.
func ancestorPIDs() []int {
	pids := []int{os.Getpid()}
	if ppid := syscall.Getppid(); ppid > 0 {
		pids = append(pids, ppid)
	}
	for len(pids) < MaxAncestorWalkDepth {
		last := pids[len(pids)-1]
		if last <= 1 {
			break
		}
		parent, err := parentPIDForPID(last)
		if err != nil || parent <= 0 {
			break
		}
		pids = append(pids, parent)
	}
	return pids
}

// parentPIDForPID returns the parent pid of the given pid via platform-
// specific introspection. Platform variants live in
// session_ancestors_{linux,darwin}.go.
//
// Exposed as a package var so tests can inject a fake without
// spawning real processes.
var parentPIDForPID = func(pid int) (int, error) {
	return 0, errors.New("parentPIDForPID: not implemented for this platform")
}
