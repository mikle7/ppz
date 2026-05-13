package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Cursors persist per-session "highest-read JetStream sequence number"
// per channel, keyed by `<accountID>.<handle>.<channel>`. Stored as JSON at
//
//	<PPZ_HOME>/cursors/<session>.json
//
// Locked by an in-process mutex so concurrent IPC calls from one session
// don't tear the file. Cross-process exclusion isn't needed — only the
// daemon writes here. We deliberately do NOT cache the parsed map in
// memory: the test harness wipes the cursors directory between scenarios,
// and an in-memory cache would mask that wipe and cause false-negative
// "unread" counts. File I/O on a small JSON blob per call is cheap.
type cursors struct {
	dir string
	mu  sync.Mutex
}

func newCursors(home string) *cursors {
	return &cursors{
		dir: filepath.Join(home, "cursors"),
	}
}

// session normalises an empty session id to "default" so callers can pass
// whatever the CLI handed them without checking.
func session(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

// Get returns the cursor for (session, key). 0 means "never read".
func (c *cursors) Get(s, key string) uint64 {
	s = session(s)
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.loadLocked(s)
	if err != nil {
		return 0
	}
	return m[key]
}

// Advance sets the cursor for (session, key) to the max of its current
// value and seq. Persists the file.
func (c *cursors) Advance(s, key string, seq uint64) error {
	if seq == 0 {
		return nil
	}
	s = session(s)
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.loadLocked(s)
	if err != nil {
		return err
	}
	if seq > m[key] {
		m[key] = seq
		return c.saveLocked(s, m)
	}
	return nil
}

func (c *cursors) loadLocked(s string) (map[string]uint64, error) {
	path := filepath.Join(c.dir, s+".json")
	data, err := os.ReadFile(path)
	m := map[string]uint64{}
	if err == nil {
		_ = json.Unmarshal(data, &m)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

func (c *cursors) saveLocked(s string, m map[string]uint64) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := filepath.Join(c.dir, s+".json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	final := filepath.Join(c.dir, s+".json")
	return os.Rename(tmp, final)
}

// CursorKey builds the per-channel key from the canonical subject parts.
func CursorKey(accountID, handle, channel string) string {
	return accountID + "." + handle + "." + channel
}
