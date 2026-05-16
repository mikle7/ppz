package daemon

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// lifecycleLogFile is the per-daemon-home file that persists daemon
// start / stop events across restarts. The in-memory NATSEventRing
// vanishes when the process exits — useful for "the connection blipped
// 30s ago" introspection within one daemon's lifetime, but useless
// for "did the daemon bounce overnight?" debugging. The lifecycle log
// closes that gap: it's a tiny append-trimmed file that a fresh daemon
// reads into its ring at startup so `ppz diagnostics` reflects the
// previous instance's shutdown.
const lifecycleLogFile = "diagnostics-lifecycle.jsonl"

// lifecycleLogCap bounds the file at the same depth as the in-memory
// ring. Two lines per restart (start + stop), so 32 lines is ~16
// daemon lifetimes — plenty of history without unbounded growth.
const lifecycleLogCap = 32

// loadLifecycleLog reads the persisted daemon lifecycle events from
// `<home>/diagnostics-lifecycle.jsonl` in chronological order (oldest
// first). Returns an empty slice when the file is absent or
// unreadable — diagnostics history is best-effort, never blocking.
func loadLifecycleLog(home string) []NATSEvent {
	path := filepath.Join(home, lifecycleLogFile)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []NATSEvent
	scanner := bufio.NewScanner(f)
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
	return out
}

// appendLifecycleLog appends a single daemon lifecycle event to the
// persisted log, then trims the file so the youngest `lifecycleLogCap`
// entries survive. Failures are non-fatal — a daemon that can't write
// its lifecycle log still runs; we just lose some diagnostics history.
func appendLifecycleLog(home string, ev NATSEvent) error {
	prev := loadLifecycleLog(home)
	prev = append(prev, ev)
	if len(prev) > lifecycleLogCap {
		prev = prev[len(prev)-lifecycleLogCap:]
	}
	path := filepath.Join(home, lifecycleLogFile)
	tmp, err := os.CreateTemp(home, lifecycleLogFile+".tmp-*")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	for _, e := range prev {
		if err := enc.Encode(e); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// recordDaemonLifecycle is the joint hook for daemon start / stop
// events: appends to the in-memory ring (so the current process's
// `ppz diagnostics` includes it) AND to the on-disk log (so the next
// daemon picks it up at startup).
func (d *Daemon) recordDaemonLifecycle(typ, reason string) {
	at := time.Now()
	if d.NATSEvents != nil {
		d.NATSEvents.Append(typ, reason, at)
	}
	if d.Home == "" {
		return
	}
	_ = appendLifecycleLog(d.Home, NATSEvent{Type: typ, At: at, Reason: reason})
}
