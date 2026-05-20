package daemon

import (
	"errors"
	"sort"
	"time"
)

// AgentBinding records that a `ppz terminal share <Handle>` process is
// running as the identity anchor for an agent. The daemon uses this to
// resolve the effective session for any IPC call whose ancestor pid
// chain includes the share's pid — regardless of whether the calling
// subprocess inherited PPZ_SESSION env or has a controlling tty.
//
// See docs/specs/session-binding.md for the empirical findings that
// drove the ppid-based (not tty-based) design.
type AgentBinding struct {
	Handle       string    `json:"handle"`
	SharePID     int       `json:"share_pid"`
	SessionKey   string    `json:"session_key"` // "agent:<Handle>"
	RegisteredAt time.Time `json:"registered_at"`
}

// ErrBindingConflict is returned by RegisterAgentBinding when the same
// SharePID has already been registered against a different handle.
// Caller is expected to unregister first.
var ErrBindingConflict = errors.New("agent binding: share pid already bound to a different handle")

// nowForBinding returns time.Now in UTC; overridable for tests.
var nowForBinding = func() time.Time { return time.Now().UTC() }

// RegisterAgentBinding records a new binding, or refreshes an existing
// one for the same (SharePID, Handle) tuple. Returns the canonical
// binding (a copy; mutating the result has no effect on State).
//
// Conflict resolution:
//   - Same (SharePID, Handle) → idempotent, returns existing binding.
//   - Same SharePID, different Handle → ErrBindingConflict.
//
// Persistence: writes agent-bindings.json atomically after a successful
// new register. Idempotent re-registers don't rewrite (no-op).
func (s *State) RegisterAgentBinding(handle string, sharePID int) (*AgentBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.agentBindings[sharePID]; ok {
		if existing.Handle != handle {
			return nil, ErrBindingConflict
		}
		// Idempotent re-register — return the existing binding unchanged.
		copy := *existing
		return &copy, nil
	}

	b := &AgentBinding{
		Handle:       handle,
		SharePID:     sharePID,
		SessionKey:   "agent:" + handle,
		RegisteredAt: nowForBinding(),
	}
	s.agentBindings[sharePID] = b
	if err := s.persistAgentBindingsLocked(); err != nil {
		// Persistence failure: roll back the in-mem entry to keep disk
		// and memory consistent.
		delete(s.agentBindings, sharePID)
		return nil, err
	}
	// Back-compat seed: existing recipes use `PPZ_SESSION=<handle> ppz …`
	// (e.g. the Monitor pattern at internal/cli/agent.go:196). That
	// declared session value lands as "<handle>" in the resolver's
	// precedence 1 path, so populate current["<handle>"] so those
	// invocations still resolve. The "agent:<handle>" key from the
	// new ancestor-walk path is populated lazily via auto-write on
	// first ResolveSessionWithAutoWrite call.
	if cur := s.current[handle]; cur == "" {
		s.current[handle] = handle
		_ = s.persistCurrentLocked()
	}
	out := *b
	return &out, nil
}

// UnregisterAgentBinding drops a binding by SharePID. No-op if unknown.
// Persists the updated table on actual removal.
func (s *State) UnregisterAgentBinding(sharePID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agentBindings[sharePID]; !ok {
		return
	}
	delete(s.agentBindings, sharePID)
	_ = s.persistAgentBindingsLocked()
}

// LookupAgentBindingByPID returns the binding for a registered SharePID,
// or nil if none. Pure lookup — does NOT validate pid liveness.
// Lazy validation (drop-on-lookup of dead pids) lives in the session
// resolver path; see lookupAgentBindingValidatedLocked.
func (s *State) LookupAgentBindingByPID(sharePID int) *AgentBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.agentBindings[sharePID]
	if !ok {
		return nil
	}
	out := *b
	return &out
}

// lookupAgentBindingValidated is the resolver's variant of lookup: it
// returns the binding only if the SharePID is still alive. Dead-pid
// entries are dropped from the table (lazy validation) and nil is
// returned. Persists on drop. Hot path; called per ancestor pid.
func (s *State) lookupAgentBindingValidated(sharePID int) *AgentBinding {
	s.mu.RLock()
	b, ok := s.agentBindings[sharePID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	if pidAlive(b.SharePID) {
		out := *b
		return &out
	}
	// Drop the dead entry under write lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, stillThere := s.agentBindings[sharePID]; stillThere && cur.SharePID == b.SharePID {
		delete(s.agentBindings, sharePID)
		_ = s.persistAgentBindingsLocked()
	}
	return nil
}

// SweepAgentBindingsForHandle drops every binding whose Handle equals
// the argument. Called when a source is destroyed. Returns the number
// of bindings removed.
func (s *State) SweepAgentBindingsForHandle(handle string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for pid, b := range s.agentBindings {
		if b.Handle == handle {
			delete(s.agentBindings, pid)
			removed++
		}
	}
	if removed > 0 {
		_ = s.persistAgentBindingsLocked()
	}
	return removed
}

// AgentBindingSnapshot returns a stable copy of all current bindings,
// sorted by SharePID ascending. Used by the persistence writer and tests.
func (s *State) AgentBindingSnapshot() []AgentBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentBinding, 0, len(s.agentBindings))
	for _, b := range s.agentBindings {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SharePID < out[j].SharePID })
	return out
}
