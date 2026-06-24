package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdWho implements `ppz who` — list every PTY source the local daemon
// has seen a heartbeat from, with online/stale/offline status derived
// from the last beat's age.
//
// Filters are client-side: the daemon dumps its raw cache, cmdWho
// trims. Status filters (--online/--stale/--offline) combine OR; the
// --harness filter combines AND. Empty filter set returns everything.
//
// Output:
//
//	default: tabular with green/amber/red status (when stdout is a TTY
//	         and NO_COLOR is unset).
//	--json:  pretty-printed JSON array, no colour, includes the derived
//	         status alongside the verbatim heartbeat payload.
func cmdWho(args []string) error {
	if wantsHelp(args) {
		printHelp(os.Stdout, "who")
		return nil
	}
	fs := flag.NewFlagSet("who", flag.ContinueOnError)
	fs.SetOutput(devNull{})
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	onlyOnline := fs.Bool("online", false, "show only online agents")
	onlyStale := fs.Bool("stale", false, "show only stale agents")
	onlyOffline := fs.Bool("offline", false, "show only offline agents")
	harness := fs.String("harness", "", "show only agents with this harness (claude/codex/copilot/agy/...)")
	owner := fs.String("owner", "", "show only agents whose source owner matches this username")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var reply cliproto.WhoReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCWho, cliproto.WhoRequest{}, &reply); err != nil {
		return err
	}

	now := time.Now()
	entries := filterWhoEntries(reply.Entries, now, whoFilter{
		Online:  *onlyOnline,
		Stale:   *onlyStale,
		Offline: *onlyOffline,
		Harness: *harness,
		Owner:   *owner,
	})

	format := "table"
	if *asJSON {
		format = "json"
	}
	out := renderWho(entries, now, whoRenderOpts{
		Format:   format,
		UseColor: shouldUseColor(*asJSON),
	})
	_, _ = os.Stdout.WriteString(out)
	return nil
}

// shouldUseColor returns true when the renderer should emit ANSI
// colour codes. False for --json output (consumers parse the JSON;
// embedded escapes would break it), or when NO_COLOR is set, or when
// stdout is not a tty (file redirect, pipe, CI log capture).
func shouldUseColor(asJSON bool) bool {
	if asJSON {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// readBodyWidth returns the wrap-width budget for the tabular `ppz read`
// renderer: the terminal column count when stdout is an interactive TTY,
// else 0 (no wrapping). Word-wrapping reflows a message body with synthetic
// line breaks — a human-presentation nicety that would actively harm any
// programmatic reader (a pipe, an agent, CI) by hiding which newlines were
// in the original message. Gating on TTY keeps non-interactive output
// byte-identical to the unwrapped form. Unlike colour this ignores
// NO_COLOR: a human who disables colour still wants wrapping.
func readBodyWidth() int {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return 0
	}
	return cliproto.TerminalWidth()
}

// whoRenderOpts configures how renderWho formats the rows it receives.
// The TTY check + NO_COLOR handling happens in cmdWho before this is
// constructed — the renderer just respects the flags it's given.
type whoRenderOpts struct {
	Format   string // "table" (default) or "json"
	UseColor bool   // wrap status cells in ANSI codes when true
}

// whoFilter is the client-side filter applied to the daemon's raw
// snapshot. Multiple status flags combine OR; harness and owner each
// combine AND with whatever else is set.
type whoFilter struct {
	Online  bool
	Stale   bool
	Offline bool
	Harness string
	Owner   string
}

// filterWhoEntries returns the entries matching the filter. An empty
// filter is a pass-through (zero is the default — `ppz who` with no
// flags returns everything).
func filterWhoEntries(entries []cliproto.WhoEntry, now time.Time, f whoFilter) []cliproto.WhoEntry {
	statusFilter := f.Online || f.Stale || f.Offline
	out := make([]cliproto.WhoEntry, 0, len(entries))
	for _, e := range entries {
		var p HeartbeatPayload
		_ = json.Unmarshal([]byte(e.Payload), &p)
		if f.Harness != "" && p.Harness != f.Harness {
			continue
		}
		if f.Owner != "" && e.Owner != f.Owner {
			continue
		}
		if statusFilter {
			status := daemon.ClassifyHeartbeatStatus(e.ArrivedAt, now, p.IntervalSec)
			if !((status == "online" && f.Online) || (status == "stale" && f.Stale) || (status == "offline" && f.Offline)) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// renderWho turns a list of WhoEntry rows into the user-visible string.
// "table" → fixed-column text with optional ANSI colour on the status
// cell; "json" → marshalled list of objects including a derived status.
func renderWho(entries []cliproto.WhoEntry, now time.Time, opts whoRenderOpts) string {
	if opts.Format == "json" {
		return renderWhoJSON(entries, now)
	}
	return renderWhoTable(entries, now, opts.UseColor)
}

const (
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiAmber = "\x1b[33m"
	ansiRed   = "\x1b[31m"
)

// statusAnsiPrefix returns the ANSI colour code for status, or "" if
// the status isn't one of the three known buckets.
func statusAnsiPrefix(status string) string {
	switch status {
	case "online":
		return ansiGreen
	case "stale":
		return ansiAmber
	case "offline":
		return ansiRed
	}
	return ""
}

func renderWhoTable(entries []cliproto.WhoEntry, now time.Time, useColor bool) string {
	// tabwriter measures column widths in bytes, not visible glyphs.
	// Feeding it ANSI-coloured cells over-pads every column whose data
	// contains escape sequences — the header row (uncoloured) ends up
	// ~9 spaces wider than the data cells below it. Render uncoloured
	// first so columns align on visible width, then post-process to
	// wrap status cells. ANSI escapes are zero-width on the terminal,
	// so the inserted bytes don't shift visible alignment.
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HANDLE\tSTATUS\tHARNESS\tMODEL\tHOST\tOS/ARCH\tCREATED\tOWNER")
	statuses := make([]string, 0, len(entries))
	for _, e := range entries {
		var p HeartbeatPayload
		_ = json.Unmarshal([]byte(e.Payload), &p)
		// STATUS shows liveness combined with the beat's agent state
		// ("online|working"); offline rows drop the state as stale noise.
		status := daemon.CombineHeartbeatStatus(
			daemon.ClassifyHeartbeatStatus(e.ArrivedAt, now, p.IntervalSec),
			p.AgentState,
		)
		statuses = append(statuses, status)
		osArch := p.OS
		if p.Arch != "" {
			osArch = p.OS + "/" + p.Arch
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Handle,
			status,
			fallback(p.Harness, "-"),
			fallback(p.Model, "-"),
			fallback(p.Hostname, "-"),
			fallback(osArch, "-"),
			createdRelative(p.StartedAt, now),
			fallback(e.Owner, "-"),
		)
	}
	_ = w.Flush()

	if !useColor {
		return buf.String()
	}

	// Walk data rows, wrap the status word with ANSI codes. The
	// status word always starts at the first non-space position
	// after the handle column ends — find it via the first 2+
	// space gap from column 0. Replacing on first match keeps a
	// handle that happens to be "online" from getting recoloured.
	lines := strings.Split(buf.String(), "\n")
	for i, status := range statuses {
		rowIdx := i + 1 // line 0 is the header
		if rowIdx >= len(lines) {
			break
		}
		// Colour keys off the liveness half of a combined status —
		// "online|working" wraps green end to end.
		prefix := statusAnsiPrefix(strings.SplitN(status, "|", 2)[0])
		if prefix == "" {
			continue
		}
		// Anchor on the first occurrence of " <status>" (one space +
		// the status word) so we don't accidentally match a substring
		// inside the handle column when the handle is e.g. "online".
		needle := " " + status
		idx := strings.Index(lines[rowIdx], needle)
		if idx < 0 {
			continue
		}
		start := idx + 1 // skip the anchor space
		end := start + len(status)
		lines[rowIdx] = lines[rowIdx][:start] + prefix + status + ansiReset + lines[rowIdx][end:]
	}
	return strings.Join(lines, "\n")
}

// whoJSONRow is the per-row shape `ppz who --json` emits. Includes the
// derived `status` and the owner the daemon resolved (so consumers
// don't have to re-implement the classifier or hit /api/v1/sources
// themselves).
type whoJSONRow struct {
	Handle    string           `json:"handle"`
	Status    string           `json:"status"`
	Owner     string           `json:"owner"`
	ArrivedAt time.Time        `json:"arrived_at"`
	Heartbeat HeartbeatPayload `json:"heartbeat"`
}

func renderWhoJSON(entries []cliproto.WhoEntry, now time.Time) string {
	rows := make([]whoJSONRow, 0, len(entries))
	for _, e := range entries {
		var p HeartbeatPayload
		_ = json.Unmarshal([]byte(e.Payload), &p)
		rows = append(rows, whoJSONRow{
			Handle:    e.Handle,
			Status:    daemon.ClassifyHeartbeatStatus(e.ArrivedAt, now, p.IntervalSec),
			Owner:     e.Owner,
			ArrivedAt: e.ArrivedAt,
			Heartbeat: p,
		})
	}
	raw, _ := json.MarshalIndent(rows, "", "  ")
	return string(raw) + "\n"
}

func fallback(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

// createdRelative renders the CREATED column for one row: a relative
// duration (matching `ppz ls`'s "N seconds ago" style via the shared
// cliproto.RelativeTime helper) computed from the heartbeat payload's
// started_at field. An unparseable or empty started_at (no payload,
// not-yet-fully-populated cache) falls back to "-" so the cell is
// always present and the column never collapses.
func createdRelative(startedAt string, now time.Time) string {
	if startedAt == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "-"
	}
	return cliproto.RelativeTime(t, now)
}
