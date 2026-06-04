package daemon

// On-disk persistence for the NATS connection-state event stream.
//
// The in-memory ring (nats_events.go) gives the default `ppz diagnostics`
// output a cheap hot tail, but it has two failure modes that this file
// closes:
//
//   1. Aging under bursts. A rotation storm can emit 10+ events per
//      second; with a 256-cap ring, two storms wipe the surrounding
//      context the operator actually needs.
//   2. Process restart. The lifecycle log already persists daemon
//      start/stop across restarts; this file does the same for every
//      connection-state event, so post-mortem analysis works even if
//      the operator restarted the daemon before running diagnostics.
//
// File: $PPZ_HOME/nats-events.jsonl  (one JSON object per line, schema
//   versioned via NATSEvent.V — see docs/diagnostics.md).
// Rotation: when the active file exceeds natsEventLogMaxBytes, it's
//   renamed to nats-events.jsonl.1 (and the prior .1 to .2). At most
//   3 generations on disk (~30 MB worst case).
// Concurrency: a single mu per home directory serializes writes.
//   Reads (tailNATSEventLog, scanNATSEventLog) re-open and stream;
//   they're allowed to race with writes — a half-written tail line is
//   skipped, not retried.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// natsEventLogFile is the active jsonl filename inside $PPZ_HOME.
const natsEventLogFile = "nats-events.jsonl"

// natsEventLogMaxBytes is the rotation threshold for the active file.
// 10 MB ≈ 50–100k events at typical line length, which covers many
// hours of real traffic plus several burst storms. Bigger than this
// and `--since=1h` queries start feeling slow on cold disks; smaller
// and bursts age out faster than they're investigated.
const natsEventLogMaxBytes = 10 * 1024 * 1024

// natsEventLogGenerations is the rotated-file count (.1, .2). One
// active + 2 rotated = ~30 MB worst case, which is small even on
// constrained dev laptops. Older generations are discarded.
const natsEventLogGenerations = 2

// natsEventLogTailEntries is how many of the most recent events the
// daemon reseeds the in-memory ring with on startup. Chosen to be
// larger than natsEventRingCap so the ring fills up to its cap from
// the persisted tail; if the file is shorter, we load what's there.
const natsEventLogTailEntries = 1024

// fileMu serializes writers within one process. Cross-process safety
// is not required: the daemon is the only writer of its $PPZ_HOME.
var fileMu sync.Mutex

// appendNATSEventLog appends one event to the active jsonl, rotating
// when the file crosses natsEventLogMaxBytes. Returns an error only on
// I/O failure that would lose the event; the caller treats failures as
// best-effort (observability never blocks the daemon's hot path).
func appendNATSEventLog(home string, ev NATSEvent) error {
	if home == "" {
		return nil
	}
	if ev.V == 0 {
		ev.V = NATSEventSchemaVersion
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	path := filepath.Join(home, natsEventLogFile)
	if err := rotateIfTooBigLocked(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = f.Write(line)
	return err
}

// rotateIfTooBigLocked rotates the active file when it's grown past
// the threshold. Caller holds fileMu. Rotation shifts .1 → .2 and
// active → .1; the active file is then absent (next append recreates
// it). Failures abort silently — better to keep writing into an
// oversized file than to lose events.
func rotateIfTooBigLocked(activePath string) error {
	fi, err := os.Stat(activePath)
	if err != nil {
		return nil // file doesn't exist yet — nothing to rotate
	}
	if fi.Size() < natsEventLogMaxBytes {
		return nil
	}
	// Shift .(N-1) → .N, .(N-2) → .(N-1), ..., active → .1.
	for gen := natsEventLogGenerations; gen >= 1; gen-- {
		src := activePath
		if gen > 1 {
			src = fmt.Sprintf("%s.%d", activePath, gen-1)
		}
		dst := fmt.Sprintf("%s.%d", activePath, gen)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.Remove(dst)        // tolerate prior partial rotation
		_ = os.Rename(src, dst)   // best-effort
	}
	return nil
}

// tailNATSEventLog returns the youngest n events from the active file
// in chronological order (oldest first). Used by the daemon at startup
// to reseed the in-memory ring so `ppz diagnostics` shows context
// across restarts. Returns nil on any read failure — best-effort.
//
// Implementation note: we read the whole active file (bounded to
// natsEventLogMaxBytes) and slice the tail in memory. For 10 MB that's
// ~25 ms on commodity SSDs, paid once per daemon startup; not worth
// the complexity of seeking-from-end with newline detection.
func tailNATSEventLog(home string, n int) []NATSEvent {
	if home == "" || n <= 0 {
		return nil
	}
	all := readNATSEventLogFile(filepath.Join(home, natsEventLogFile))
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

// scanNATSEventLog returns all events in [since, now] across all
// generations (oldest → youngest, chronological). Used by
// `ppz diagnostics --since=DURATION` and the bundle command.
//
// Reads the rotated files (.2, .1) before the active file so that
// chronological order is preserved across rotations. Events with an
// older timestamp than `since` are filtered out; the file scan is full
// because timestamps aren't strictly monotonic across processes.
func scanNATSEventLog(home string, since time.Time) []NATSEvent {
	if home == "" {
		return nil
	}
	var out []NATSEvent
	for gen := natsEventLogGenerations; gen >= 1; gen-- {
		path := fmt.Sprintf("%s.%d", filepath.Join(home, natsEventLogFile), gen)
		for _, ev := range readNATSEventLogFile(path) {
			if ev.At.Before(since) {
				continue
			}
			out = append(out, ev)
		}
	}
	for _, ev := range readNATSEventLogFile(filepath.Join(home, natsEventLogFile)) {
		if ev.At.Before(since) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// readNATSEventLogFile parses one jsonl file into NATSEvents.
// Malformed lines are skipped (a writer crash mid-line leaves a
// partial JSON object — drop it, keep the rest of the file usable).
// Returns nil on stat/open failure.
func readNATSEventLogFile(path string) []NATSEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []NATSEvent
	scanner := bufio.NewScanner(f)
	// Default Scanner buffer is 64KB; one event is comfortably under
	// 1 KB but bumping to 1 MB is cheap insurance against future
	// schema growth (long error messages, etc.).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev NATSEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		out = append(out, ev)
	}
	// Discard scanner errors — best-effort read.
	_ = scanner.Err()
	return out
}

// countLogLines counts the non-empty lines in a jsonl file without
// unmarshaling them — used for the "X older events on disk" hint, which
// only needs an approximate count and runs on every `ppz diagnostics`.
// A malformed tail line from a writer crash is counted; the hint is an
// estimate, not an exact parseable-event count. Returns 0 on open
// failure.
func countLogLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			n++
		}
	}
	_ = scanner.Err()
	return n
}

// natsEventLogPaths returns every existing log file under $PPZ_HOME,
// active first, then rotated generations from newest (.1) to oldest.
// Used by the bundle command to include all generations in the
// support archive.
func natsEventLogPaths(home string) []string {
	if home == "" {
		return nil
	}
	var out []string
	active := filepath.Join(home, natsEventLogFile)
	if _, err := os.Stat(active); err == nil {
		out = append(out, active)
	}
	for gen := 1; gen <= natsEventLogGenerations; gen++ {
		p := fmt.Sprintf("%s.%d", active, gen)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// reseedRingFromDisk is the daemon's startup hook: it primes the
// in-memory ring from the two on-disk logs so a fresh process's
// `ppz diagnostics` shows context from before the restart.
//
// Lifecycle events (daemon_start / daemon_stop) are persisted to BOTH
// the lifecycle log and nats-events.jsonl — the former is a small
// bounded log a fresh daemon can always reseed from, the latter unifies
// the --since history. Loading both naively double-counts every
// lifecycle event, so we merge the two sources, dedup by identity, and
// replay in chronological order.
//
// The lifecycle log is still read separately rather than trusting
// nats-events.jsonl alone, so the first daemon after an upgrade from a
// pre-Phase-0 binary — which wrote lifecycle events ONLY to the
// lifecycle log — still surfaces the prior daemon's stop event.
func (d *Daemon) reseedRingFromDisk() {
	if d.NATSEvents == nil || d.Home == "" {
		return
	}
	merged := append(loadLifecycleLog(d.Home), tailNATSEventLog(d.Home, natsEventLogTailEntries)...)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].At.Before(merged[j].At) })
	seen := make(map[string]struct{}, len(merged))
	for _, ev := range merged {
		// A lifecycle event's two on-disk copies are marshaled from the
		// same in-memory value, so (Type, At, Reason) is a stable
		// identity for dedup. Normalize to UTC so the key is independent
		// of the location pointer JSON unmarshal attaches.
		key := ev.Type + "\x00" + ev.At.UTC().Format(time.RFC3339Nano) + "\x00" + ev.Reason
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		d.NATSEvents.Append(ev)
	}
}

