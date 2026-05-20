package daemon

import (
	"errors"
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

// RegisterAgentBinding records a new binding, or refreshes an existing
// one for the same (SharePID, Handle) tuple. Returns the canonical
// binding (a pointer into State; do not mutate).
//
// Conflict resolution: same (SharePID, Handle) → no-op, returns existing.
// SharePID matches but Handle differs → ErrBindingConflict.
//
// Persistence: writes agent-bindings.json atomically after register.
func (s *State) RegisterAgentBinding(handle string, sharePID int) (*AgentBinding, error) {
	return nil, errors.New("RegisterAgentBinding: not implemented")
}

// UnregisterAgentBinding drops a binding by SharePID. No-op if unknown.
// Persists the updated table.
func (s *State) UnregisterAgentBinding(sharePID int) {
	// not implemented
}

// LookupAgentBindingByPID returns the binding for an active SharePID,
// or nil if none registered. Hot path: called by the session resolver
// when walking the caller's ancestor chain.
//
// Lazy validation: if the binding's SharePID is no longer alive
// (process exited but daemon hasn't cleaned up yet), the binding is
// dropped from the table and nil is returned.
func (s *State) LookupAgentBindingByPID(sharePID int) *AgentBinding {
	return nil
}

// SweepAgentBindingsForHandle drops every binding whose Handle equals
// the argument. Called when a source is destroyed. Returns the number
// of bindings removed.
func (s *State) SweepAgentBindingsForHandle(handle string) int {
	return 0
}

// AgentBindingSnapshot returns a stable copy of all current bindings,
// sorted by SharePID ascending. Used by the persistence writer and tests.
func (s *State) AgentBindingSnapshot() []AgentBinding {
	return nil
}
