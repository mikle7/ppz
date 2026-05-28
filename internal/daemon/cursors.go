package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// cursorEntry is the persisted read watermark for one (session, key).
//
//   - Seq is the highest JetStream sequence the session has consumed.
//   - StreamCreated is the stream's Created timestamp (unix nanos) observed
//     when Seq was last advanced. It is the stream-identity fingerprint
//     that makes the cursor safe across source destroy+recreate: recreating
//     a source under the same handle builds a brand-new stream whose seq
//     restarts at 1 but whose Created differs. A cursor stamped against the
//     old incarnation is then detectably stale (see effectiveCursor) and
//     must not gate reads against the fresh stream — otherwise `read` / `ls`
//     silently skip or under-count the new messages.
//
// Legacy cursor files predate StreamCreated and serialise each entry as a
// bare integer; UnmarshalJSON accepts that form (StreamCreated = 0,
// "identity unknown") and the file is rewritten in object form on the next
// Advance.
type cursorEntry struct {
	Seq           uint64 `json:"seq"`
	StreamCreated int64  `json:"created,omitempty"`
}

// UnmarshalJSON accepts both the current object form ({"seq":N,"created":T})
// and the legacy bare-integer form (N). The default struct marshaller
// produces the object form, so no custom MarshalJSON is needed.
func (e *cursorEntry) UnmarshalJSON(data []byte) error {
	if d := bytes.TrimSpace(data); len(d) > 0 && d[0] != '{' {
		var n uint64
		if err := json.Unmarshal(d, &n); err != nil {
			return err
		}
		e.Seq, e.StreamCreated = n, 0
		return nil
	}
	type alias cursorEntry // shed UnmarshalJSON to avoid recursion
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = cursorEntry(a)
	return nil
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

// GetEntry returns the persisted entry for (session, key). A zero entry
// means "never read".
func (c *cursors) GetEntry(s, key string) cursorEntry {
	s = session(s)
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.loadLocked(s)
	if err != nil {
		return cursorEntry{}
	}
	return m[key]
}

// Advance records that (session, key) has now read up to seq on the stream
// whose Created time is streamCreated (unix nanos; 0 if unknown). Persists
// the file.
//
// Within one stream incarnation (StreamCreated matches) it keeps the
// existing monotonic-max contract. When the identity differs — a recreated
// source, or the first time we stamp a legacy entry — the stored seq
// belonged to a different stream and must NOT be carried over, so we
// overwrite outright. That overwrite is what un-wedges a cursor left ahead
// of a freshly recreated stream.
func (c *cursors) Advance(s, key string, seq uint64, streamCreated int64) error {
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
	cur := m[key]
	if streamCreated == cur.StreamCreated && seq <= cur.Seq {
		return nil
	}
	m[key] = cursorEntry{Seq: seq, StreamCreated: streamCreated}
	return c.saveLocked(s, m)
}

func (c *cursors) loadLocked(s string) (map[string]cursorEntry, error) {
	path := filepath.Join(c.dir, s+".json")
	data, err := os.ReadFile(path)
	m := map[string]cursorEntry{}
	if err == nil {
		_ = json.Unmarshal(data, &m)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

func (c *cursors) saveLocked(s string, m map[string]cursorEntry) error {
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

// effectiveCursor maps a persisted entry to the seq baseline that read /
// unread computations should use for a stream with the given current
// Created time and LastSeq. It returns 0 ("treat the whole stream as
// unread") when the entry is stale:
//
//   - identity mismatch: the entry was stamped against a different stream
//     incarnation (source destroyed + recreated under the same handle),
//     detected by StreamCreated differing from the live stream's Created.
//   - regressed seq: the entry sits ahead of LastSeq, which is impossible
//     for a healthy monotonic stream. This catches entries with no usable
//     identity stamp (legacy files, or a stream whose Created we couldn't
//     read) where the seq alone betrays a reset.
//
// Otherwise the stored Seq is the baseline, exactly as before — so all
// non-recreate flows are unchanged.
func effectiveCursor(e cursorEntry, streamCreated int64, lastSeq uint64) uint64 {
	if e.Seq == 0 {
		return 0
	}
	if e.StreamCreated != 0 && streamCreated != 0 && e.StreamCreated != streamCreated {
		return 0
	}
	if e.Seq > lastSeq {
		return 0
	}
	return e.Seq
}

// createdNanos is the unix-nanos identity stamp for a stream's Created
// time, normalising the zero time to 0 ("unknown") so effectiveCursor can
// skip the identity check rather than treat a missing timestamp as a
// distinct incarnation.
func createdNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// CursorKey builds the per-channel key from the canonical subject parts.
func CursorKey(accountID, handle, channel string) string {
	return accountID + "." + handle + "." + channel
}
