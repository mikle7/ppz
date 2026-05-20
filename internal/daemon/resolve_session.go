package daemon

// MaxPPIDWalkDepth caps how far the CLI walks the parent process tree
// when collecting its ancestor pid chain. Eight hops covers realistic
// agent → bash → harness → Monitor → bash chains without unbounded
// /proc reads. Tunable.
const MaxPPIDWalkDepth = 8

// ResolvedSession is the daemon's verdict on which session a caller
// belongs to. SessionKey is the lookup key for all per-session state
// (current handle, cursors, namespace, future subs). BoundHandle is a
// non-empty handle when the resolution method already knows whose
// agent the caller is part of (ancestor-match precedence) — used by
// auto-write-current so the agent doesn't need to call `ppz set
// handle` after spawn. Empty BoundHandle means "fall through to
// State.Current[SessionKey]".
type ResolvedSession struct {
	SessionKey  string
	BoundHandle string
}

// ResolveSession determines the effective session for an IPC call.
//
// `declaredSession` is the client-supplied PPZ_SESSION env value (or
// the legacy CLI's local sessionID() output). Non-empty short-circuits
// resolution to honor the explicit override.
//
// `ancestorPIDs` is the caller's process ancestor chain, walked
// client-side by the CLI: [self_pid, parent_pid, grandparent_pid, …]
// up to MaxPPIDWalkDepth. The resolver scans this in order, returning
// the first agent binding whose SharePID appears in the chain.
//
// Resolution precedence (docs/specs/session-binding.md):
//
//  1. declaredSession != ""           → ("declaredSession", "")
//  2. ancestorPIDs hits a binding     → (binding.SessionKey, binding.Handle)
//  3. fallback                        → ("default" or similar, "")
func (s *State) ResolveSession(declaredSession string, ancestorPIDs []int) ResolvedSession {
	// not implemented
	return ResolvedSession{}
}

// ResolveSessionWithAutoWrite wraps ResolveSession with the daemon's
// auto-write-current contract: if BoundHandle is non-empty AND
// State.Current(SessionKey) is currently empty, atomically populate
// it so subsequent send/read paths see the binding as the "current
// handle" without the caller needing to call `ppz set handle` after
// spawn. Idempotent — repeated calls don't churn disk.
//
// This is what handler functions should call. Pure resolution (no
// side effects) is exposed via ResolveSession for tests.
func (s *State) ResolveSessionWithAutoWrite(declaredSession string, ancestorPIDs []int) ResolvedSession {
	// not implemented
	return ResolvedSession{}
}
