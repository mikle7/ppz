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
	fs := flag.NewFlagSet("who", flag.ContinueOnError)
	fs.SetOutput(devNull{})
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	onlyOnline := fs.Bool("online", false, "show only online agents")
	onlyStale := fs.Bool("stale", false, "show only stale agents")
	onlyOffline := fs.Bool("offline", false, "show only offline agents")
	harness := fs.String("harness", "", "show only agents with this harness (claude/codex/copilot/gemini/...)")
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

// whoRenderOpts configures how renderWho formats the rows it receives.
// The TTY check + NO_COLOR handling happens in cmdWho before this is
// constructed — the renderer just respects the flags it's given.
type whoRenderOpts struct {
	Format   string // "table" (default) or "json"
	UseColor bool   // wrap status cells in ANSI codes when true
}

// whoFilter is the client-side filter applied to the daemon's raw
// snapshot. Multiple status flags combine OR; harness combines AND.
type whoFilter struct {
	Online  bool
	Stale   bool
	Offline bool
	Harness string
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

func colorStatus(status string, useColor bool) string {
	if !useColor {
		return status
	}
	switch status {
	case "online":
		return ansiGreen + status + ansiReset
	case "stale":
		return ansiAmber + status + ansiReset
	case "offline":
		return ansiRed + status + ansiReset
	}
	return status
}

func renderWhoTable(entries []cliproto.WhoEntry, now time.Time, useColor bool) string {
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "HANDLE\tSTATUS\tHARNESS\tMODEL\tHOST\tOS/ARCH\tAGE")
	for _, e := range entries {
		var p HeartbeatPayload
		_ = json.Unmarshal([]byte(e.Payload), &p)
		status := daemon.ClassifyHeartbeatStatus(e.ArrivedAt, now, p.IntervalSec)
		osArch := p.OS
		if p.Arch != "" {
			osArch = p.OS + "/" + p.Arch
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Handle,
			colorStatus(status, useColor),
			fallback(p.Harness, "-"),
			fallback(p.Model, "-"),
			fallback(p.Hostname, "-"),
			fallback(osArch, "-"),
			formatAge(now.Sub(e.ArrivedAt)),
		)
	}
	_ = w.Flush()
	return buf.String()
}

// whoJSONRow is the per-row shape `ppz who --json` emits. Includes the
// derived `status` so consumers don't have to re-implement the
// classifier.
type whoJSONRow struct {
	Handle     string           `json:"handle"`
	Status     string           `json:"status"`
	ArrivedAt  time.Time        `json:"arrived_at"`
	Heartbeat  HeartbeatPayload `json:"heartbeat"`
}

func renderWhoJSON(entries []cliproto.WhoEntry, now time.Time) string {
	rows := make([]whoJSONRow, 0, len(entries))
	for _, e := range entries {
		var p HeartbeatPayload
		_ = json.Unmarshal([]byte(e.Payload), &p)
		rows = append(rows, whoJSONRow{
			Handle:    e.Handle,
			Status:    daemon.ClassifyHeartbeatStatus(e.ArrivedAt, now, p.IntervalSec),
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

// formatAge renders the duration since the last beat in a compact
// human form: <60s as "Ns", <60min as "Nm", else "Nh". Heartbeats
// arriving at the daemon are always recent (typically <interval) so
// the longer buckets are rarely seen in healthy fleets.
func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
