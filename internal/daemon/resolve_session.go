package daemon

import (
	"fmt"
	"os"
)

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
//  1. ancestor hits a binding   → (binding.SessionKey, binding.Handle)
//  2. declaredSession != ""     → ("declaredSession", "")
//  3. fallback                  → ("default", "")
//
// Inversion rationale: the CLI sends both `Session` (the legacy
// sessionID() output — sid-N or PPZ_SESSION env) AND `AncestorPIDs`
// in every request. If we let declared win, the resolver would never
// engage for any call from a shell that hasn't explicitly unset
// PPZ_SESSION. By putting the binding first, in-pty subprocesses
// always resolve to their agent regardless of what their inherited
// session id happens to be — which is the entire point of Layer 1.
//
// Cost: explicit `PPZ_SESSION=foo ppz …` from inside an agent's pty
// is ignored; the binding wins. To operate under a synthetic
// session, run from outside the binding's process tree.
func (s *State) ResolveSession(declaredSession string, ancestorPIDs []int) ResolvedSession {
	for _, pid := range ancestorPIDs {
		if pid <= 1 {
			continue
		}
		if b := s.lookupAgentBindingValidated(pid); b != nil {
			fmt.Fprintf(os.Stderr, "[debug] ResolveSession declared=%q ancestors=%v → matched pid=%d handle=%s sessionKey=%s\n", declaredSession, ancestorPIDs, pid, b.Handle, b.SessionKey)
			return ResolvedSession{SessionKey: b.SessionKey, BoundHandle: b.Handle}
		}
	}
	if declaredSession != "" {
		fmt.Fprintf(os.Stderr, "[debug] ResolveSession declared=%q ancestors=%v → declared fallback\n", declaredSession, ancestorPIDs)
		return ResolvedSession{SessionKey: declaredSession}
	}
	fmt.Fprintf(os.Stderr, "[debug] ResolveSession declared=%q ancestors=%v → default fallback\n", declaredSession, ancestorPIDs)
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
