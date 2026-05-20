package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

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
// State. Missing file is a clean fresh-install; corrupt / wrong-
// version files yield an empty table without error.
//
// After loading, each entry is validated: SharePID must still be a
// live process. Dead-pid entries are dropped before being made
// visible to lookups.
//
// Called by State.LoadFromDisk at daemon startup.
func (s *State) LoadAgentBindings() error {
	path := filepath.Join(s.home, fileAgentBindings)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Fresh install / never written — clean empty table.
			s.mu.Lock()
			s.agentBindings = map[int]*AgentBinding{}
			s.mu.Unlock()
			return nil
		}
		return err
	}

	var file agentBindingsFile
	if err := json.Unmarshal(data, &file); err != nil {
		// Corrupt file (truncated, garbage, empty) — start empty, do
		// not propagate the error. Recovery happens on the share's
		// next register IPC.
		s.mu.Lock()
		s.agentBindings = map[int]*AgentBinding{}
		s.mu.Unlock()
		return nil
	}
	if file.Version != agentBindingsFileVersion {
		// Unknown version — start empty.
		s.mu.Lock()
		s.agentBindings = map[int]*AgentBinding{}
		s.mu.Unlock()
		return nil
	}

	// Validate-on-load: drop entries whose SharePID is no longer alive.
	live := make(map[int]*AgentBinding, len(file.Bindings))
	for i := range file.Bindings {
		b := file.Bindings[i]
		if !pidAlive(b.SharePID) {
			continue
		}
		// Normalize SessionKey in case the persisted file was written
		// by a different version that derived it differently.
		if b.SessionKey == "" {
			b.SessionKey = "agent:" + b.Handle
		}
		live[b.SharePID] = &b
	}

	s.mu.Lock()
	s.agentBindings = live
	// If any entries were dropped, rewrite the file so disk matches
	// memory. Cheap, only happens at startup.
	wroteOK := true
	if len(live) != len(file.Bindings) {
		if err := s.persistAgentBindingsLocked(); err != nil {
			wroteOK = false
		}
	}
	s.mu.Unlock()
	if !wroteOK {
		// Non-fatal — in-mem is authoritative until next write succeeds.
		return nil
	}
	return nil
}

// persistAgentBindingsLocked writes the in-memory binding table to
// disk using the atomic tmp+rename pattern. Caller must hold s.mu.
func (s *State) persistAgentBindingsLocked() error {
	bindings := make([]AgentBinding, 0, len(s.agentBindings))
	for _, b := range s.agentBindings {
		bindings = append(bindings, *b)
	}
	file := agentBindingsFile{
		Version:  agentBindingsFileVersion,
		Bindings: bindings,
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	target := filepath.Join(s.home, fileAgentBindings)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// pidAlive reports whether the given pid is a live process. Uses the
// portable kill(pid, 0) idiom — sends a no-op signal, which returns
// ESRCH if the process no longer exists.
//
// Exposed as a package var so tests with synthetic pids can override
// without spawning real processes.
var pidAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but we can't signal it (e.g.,
	// owned by another user). Still "alive" for our purposes.
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
