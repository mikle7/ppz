package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// subscriptions persist a per-session list of subscribed pipe subjects —
// the curated subset that `ppz subs {ls,wait,read}` operate over (an
// agent's inbox-monitor list, a human's "I'm in these rooms" set). Stored
// as a JSON array of subject strings at
//
//	<PPZ_HOME>/subs/<session>.json
//
// Mirrors the cursors subsystem deliberately: file-per-session, in-process
// mutex (only the daemon writes here), and NO in-memory cache — the e2e
// harness wipes the dir between scenarios and a cache would mask the wipe.
//
// Subjects are stored verbatim in the user-facing form the CLI accepts:
// `<handle>.<pipe>` (collared) or a bare uncollared pipe path. No glob
// expansion at storage time — if globs ship later they expand at read-time.
//
// Keying is the load-bearing design point (see docs / the subs design
// brief): auto-subscribe-on-create keys under the HANDLE, while manual
// `subs add` keys under session(req.Session) the same way cursors do.
// This file is keying-agnostic — callers pass whichever session string
// applies.
type subscriptions struct {
	dir string
	mu  sync.Mutex
}

func newSubscriptions(home string) *subscriptions {
	return &subscriptions{dir: filepath.Join(home, "subs")}
}

// List returns the session's subscribed subjects, sorted and de-duplicated.
func (s *subscriptions) List(sess string) []string {
	sess = session(sess)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(sess)
}

// Add appends subjects to the session's list, idempotently — a subject
// already present is a no-op. Persists only when something changed (no
// churn on restart-and-re-share). Empty / whitespace-only subjects are
// skipped.
func (s *subscriptions) Add(sess string, subjects ...string) error {
	sess = session(sess)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.loadLocked(sess)
	have := sliceToSet(cur)
	changed := false
	for _, subj := range subjects {
		subj = strings.TrimSpace(subj)
		if subj == "" || have[subj] {
			continue
		}
		have[subj] = true
		cur = append(cur, subj)
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveLocked(sess, cur)
}

// Remove drops subjects from the session's list, idempotently — removing an
// absent subject is a no-op. Persists only when something changed.
func (s *subscriptions) Remove(sess string, subjects ...string) error {
	sess = session(sess)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.loadLocked(sess)
	drop := sliceToSet(subjects)
	kept := make([]string, 0, len(cur))
	for _, subj := range cur {
		if !drop[subj] {
			kept = append(kept, subj)
		}
	}
	if len(kept) == len(cur) {
		return nil
	}
	return s.saveLocked(sess, kept)
}

// SweepHandle drops every subject targeting `handle`'s pipes from EVERY
// session's subs file. Called on source destroy (mirrors the cursor sweep)
// so a destroyed handle leaves no zombie subscriptions — in its own session
// file (the auto-sub) or in any user shell that subscribed to its pipes.
//
// A subject targets `handle` when it equals the handle outright or is
// collared under it (`<handle>.<pipe>`).
func (s *subscriptions) SweepHandle(handle string) error {
	if handle == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	prefix := handle + "."
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		sess := strings.TrimSuffix(name, ".json")
		cur := s.loadLocked(sess)
		kept := make([]string, 0, len(cur))
		for _, subj := range cur {
			if subj == handle || strings.HasPrefix(subj, prefix) {
				continue
			}
			kept = append(kept, subj)
		}
		if len(kept) != len(cur) {
			if err := s.saveLocked(sess, kept); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadLocked reads + parses the session file, returning subjects sorted.
// A missing or unparseable file yields an empty list (caller holds mutex).
func (s *subscriptions) loadLocked(sess string) []string {
	data, err := os.ReadFile(filepath.Join(s.dir, sess+".json"))
	if err != nil {
		return nil
	}
	var subs []string
	if json.Unmarshal(data, &subs) != nil {
		return nil
	}
	sort.Strings(subs)
	return subs
}

// saveLocked writes the list atomically (tmp + rename), same as cursors.
func (s *subscriptions) saveLocked(sess string, subs []string) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	sort.Strings(subs)
	data, err := json.Marshal(subs)
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, sess+".json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, sess+".json"))
}

func sliceToSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
