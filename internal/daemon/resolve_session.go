package daemon

import "fmt"

// MaxPPIDWalkDepth caps how far the CLI walks the parent process tree
// when collecting its ancestor pid chain. Eight hops covers realistic
// agent → bash → harness → Monitor → bash chains without unbounded
// /proc reads. Tunable.
const MaxPPIDWalkDepth = 8

// ResolvedSession is the daemon's verdict on which session a caller
// belongs to. SessionKey is the lookup key for all per-session state
// (current handle, cursors, namespace, future subs). BoundHandle is a
// non-empty handle when the resolution method already knows whose
// agent the caller is part of (ancestor-match precedence). Empty
// BoundHandle means "fall through to State.Current[SessionKey]".
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
// client-side: [self_pid, parent_pid, grandparent_pid, …]. The
// resolver scans in order, returning the first agent binding whose
// SharePID appears in the chain.
//
// Resolution precedence (docs/specs/session-binding.md):
//
//  1. declaredSession != ""        → ("declaredSession", "")
//  2. ancestor hits a binding      → (binding.SessionKey, binding.Handle)
//  3. fallback                     → (synthetic session key, "")
func (s *State) ResolveSession(declaredSession string, ancestorPIDs []int) ResolvedSession {
	if declaredSession != "" {
		return ResolvedSession{SessionKey: declaredSession}
	}
	for _, pid := range ancestorPIDs {
		if pid <= 1 {
			continue
		}
		if b := s.lookupAgentBindingValidated(pid); b != nil {
			return ResolvedSession{SessionKey: b.SessionKey, BoundHandle: b.Handle}
		}
	}
	// Fallback: synthesize a session key from the caller's pid (first
	// entry of ancestorPIDs). Mirrors the legacy `sid-N` shape so
	// callers that already have state keyed by such a label continue
	// to land on it.
	if len(ancestorPIDs) > 0 && ancestorPIDs[0] > 0 {
		return ResolvedSession{SessionKey: fmt.Sprintf("pid-%d", ancestorPIDs[0])}
	}
	return ResolvedSession{SessionKey: "default"}
}

// ResolveSessionWithAutoWrite wraps ResolveSession with the daemon's
// auto-write-current contract: if BoundHandle is non-empty AND
// State.Current(SessionKey) is currently empty, atomically populate
// it so subsequent send/read paths see the binding as the "current
// handle" without the caller needing to call `ppz set handle` after
// spawn. Idempotent — repeated calls don't churn disk because
// SetCurrent is a no-op when the value is unchanged.
func (s *State) ResolveSessionWithAutoWrite(declaredSession string, ancestorPIDs []int) ResolvedSession {
	r := s.ResolveSession(declaredSession, ancestorPIDs)
	if r.BoundHandle == "" {
		return r
	}
	if cur := s.Current(r.SessionKey); cur == "" {
		_ = s.SetCurrent(r.SessionKey, r.BoundHandle)
	}
	return r
}
