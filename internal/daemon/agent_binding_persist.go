package daemon

// fileAgentBindings is the persistence target. Same atomic-rename
// pattern as fileCurrent / fileNamespace.
const fileAgentBindings = "agent-bindings.json"

// agentBindingsFileVersion is the current on-disk schema version.
// Loaders reject other versions with a warning and start empty.
const agentBindingsFileVersion = 1

// agentBindingsFile is the wire / disk shape of the persistence file.
// Versioned envelope so future schema evolution can be handled
// cleanly on load.
type agentBindingsFile struct {
	Version  int            `json:"version"`
	Bindings []AgentBinding `json:"bindings"`
}

// LoadAgentBindings reads the binding table from disk and merges into
// State. Missing file is a clean fresh-install; corrupt/wrong-version
// files log a warning and yield an empty table.
//
// After loading, each entry is validated: SharePID must still be a
// live process (kill(pid, 0) == nil). Dead-pid entries are dropped
// before being made visible to lookups.
//
// Called by State.LoadFromDisk at daemon startup.
func (s *State) LoadAgentBindings() error {
	// not implemented
	return nil
}

// persistAgentBindingsLocked writes the in-memory binding table to
// disk using the atomic tmp+rename pattern. Caller must hold s.mu.
func (s *State) persistAgentBindingsLocked() error {
	// not implemented
	return nil
}

// validateAgentBindingLive reports whether a binding's SharePID is
// still a live process. Used by LoadAgentBindings (startup) and by
// LookupAgentBindingByPID (lazy on request path).
func validateAgentBindingLive(b AgentBinding) bool {
	return false
}
