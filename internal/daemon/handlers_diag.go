package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// handleDiag returns the daemon's introspection snapshot — connection
// state, refresh timing, recent event ring or disk scan, and pattern
// detector hits. CRITICAL: this handler must succeed even when the
// daemon has no credentials and no NATS connection. The whole point
// of `ppz diagnostics` is being able to introspect a sick daemon —
// if the verb itself requires login or a live NATS connection, an
// operator can't use it in the failure modes it's designed to surface.
//
// When req.SinceUnix is non-zero, events are read from the on-disk
// jsonl (scanNATSEventLog) instead of the in-memory ring. This is the
// path `ppz diagnostics --since=DURATION` exercises; the daemon ring
// is bypassed because it holds only the hot tail (natsEventRingCap
// entries — typically the last few minutes under load).
func (d *Daemon) handleDiag(ctx context.Context, conn net.Conn, params json.RawMessage) {
	_ = ctx
	var req cliproto.DiagRequest
	_ = json.Unmarshal(params, &req)

	state, drops, _ := d.natsStatusSnapshot()
	events := d.diagEventWindow(req)
	natsEvents := make([]cliproto.DiagEvent, 0, len(events))
	for _, ev := range events {
		natsEvents = append(natsEvents, cliproto.DiagEvent{
			V:      ev.V,
			Type:   ev.Type,
			At:     ev.At,
			Caller: ev.Caller,
			NCID:   ev.NCID,
			JWTExp: ev.JWTExp,
			Reason: ev.Reason,
		})
	}

	patterns := detectPatterns(events)
	patternsOut := make([]cliproto.DiagPattern, 0, len(patterns))
	for _, p := range patterns {
		patternsOut = append(patternsOut, cliproto.DiagPattern{
			Name:   p.Name,
			At:     p.At,
			Detail: p.Detail,
		})
	}

	writeIPC(conn, cliproto.DiagReply{
		Summary:           d.diagSummary(state),
		Patterns:          patternsOut,
		NATSState:         state,
		NATSDropsLastHour: drops,
		NATSEvents:        natsEvents,
		OnDiskCount:       countNATSEventLog(d.Home),
	})
}

// diagEventWindow chooses between the in-memory ring (default — fast,
// covers the hot tail) and the on-disk scan (when SinceUnix is set —
// covers the full retained history). Both return events in
// chronological order, so callers don't have to know which path ran.
func (d *Daemon) diagEventWindow(req cliproto.DiagRequest) []NATSEvent {
	if req.SinceUnix > 0 {
		return scanNATSEventLog(d.Home, time.Unix(req.SinceUnix, 0))
	}
	if d.NATSEvents == nil {
		return nil
	}
	return d.NATSEvents.Snapshot()
}

// diagSummary populates DiagSummary from whatever live daemon state is
// available. Missing fields stay zero — the CLI renders zero values as
// "—" rather than erroring.
//
// StateSince comes from the most recent event matching the current
// state ("connect"/"reconnect" if state=="connected", "disconnect"/
// "closed" otherwise). Approximate but useful: a state-since of "30
// min ago" tells the operator the connection has been stable; "2s
// ago" says something just flapped.
func (d *Daemon) diagSummary(state string) cliproto.DiagSummary {
	out := cliproto.DiagSummary{
		State: state,
		URL:   d.NATSURL,
	}
	if d.Refresh != nil {
		out.RefreshLastAt = d.Refresh.LastRefreshAt()
		if exp := d.Refresh.JWTExp(); exp > 0 {
			out.RefreshNextDueAt = time.Unix(exp, 0).Add(-time.Duration(skewSeconds) * time.Second)
		}
	}
	if d.NATSEvents != nil {
		events := d.NATSEvents.Snapshot()
		out.StateSince, _ = stateSinceFrom(state, events)
	}
	return out
}

// stateSinceFrom finds the timestamp at which the daemon entered the
// given state, by scanning events backward for the most recent
// transition into that state. Returns zero (and "") when no such event
// is in the ring (fresh daemon, or state predates the ring's oldest
// entry).
//
// The returned event-type string is the token that anchored the
// timestamp — "connect" / "reconnect" / "disconnect" / "closed". The
// CLI uses it on the `ppz status` `nats:` line to colour the state
// token: a first-connect renders green regardless of age, while a
// recent reconnect is amber for the first minute (the "just recovered,
// might re-flap" signal). See cliproto.formatNATSLine.
func stateSinceFrom(state string, events []NATSEvent) (time.Time, string) {
	want := stateEntryTypes(state)
	if len(want) == 0 {
		return time.Time{}, ""
	}
	for i := len(events) - 1; i >= 0; i-- {
		for _, t := range want {
			if events[i].Type == t {
				return events[i].At, events[i].Type
			}
		}
	}
	return time.Time{}, ""
}

// stateEntryTypes maps a high-level state string to the event types
// whose firing marks entry to that state. "connecting" maps to nothing
// because it's a transient state nats.go enters between transitions;
// the surrounding "connect" / "reconnect" / "disconnect" bracket it.
func stateEntryTypes(state string) []string {
	switch state {
	case "connected":
		return []string{"connect", "reconnect"}
	case "disconnected":
		return []string{"disconnect", "closed", "daemon_start"}
	}
	return nil
}

// countNATSEventLog returns the line count of the active jsonl. Used
// for the "X older events on disk" hint in CLI output. Reads the
// file rather than maintaining a counter so a daemon that crashed
// mid-write reports the truth, not a stale cached value.
func countNATSEventLog(home string) int {
	if home == "" {
		return 0
	}
	return len(readNATSEventLogFile(natsEventLogActivePath(home)))
}

// natsEventLogActivePath returns the active file path, factored out
// so future helpers (rotation reporters, tail readers) can reference
// the same single source of truth.
func natsEventLogActivePath(home string) string {
	return filepath.Join(home, natsEventLogFile)
}
