package cli

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
	"github.com/pipescloud/ppz/internal/version"
)

// cmdDiagnostics implements `ppz diagnostics`. The verb deliberately
// does NOT require login — an operator hitting a sick daemon (login
// fails, NATS unreachable) needs diagnostics to work; that's the
// entire point. The IPC handler tolerates a credential-less daemon
// and returns whatever introspection is available.
//
// Flags (see also docs/diagnostics.md):
//
//	(no flags)         Default human-readable output: summary block,
//	                   pattern warnings, recent event tail, next-step
//	                   hints. Tuned for "the operator just ran this
//	                   without reading any docs" — pit of success.
//	--since=DURATION   Scope events to the trailing window, reading
//	                   the on-disk jsonl rather than just the in-memory
//	                   ring. Example: --since=2h.
//	--json             Emit DiagReply as JSON for machine consumers
//	                   (AI agents, scripts). Patterns are first-class
//	                   in the JSON output.
//	--bundle           Write a support tarball (~/ppz-diag-<ts>.tgz)
//	                   containing every persisted log + the current
//	                   diagnostics snapshot + version + uname. Print
//	                   the path; the operator attaches it to a bug
//	                   report.
func cmdDiagnostics(args []string) error {
	if wantsHelp(args) {
		printHelp(os.Stdout, "diagnostics")
		return nil
	}
	var (
		asJSON   bool
		bundle   bool
		sinceStr string
	)
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "--bundle":
			bundle = true
		case strings.HasPrefix(a, "--since="):
			sinceStr = strings.TrimPrefix(a, "--since=")
		default:
			return fmt.Errorf("unknown flag: %s", a)
		}
	}

	req := cliproto.DiagRequest{}
	if sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		req.SinceUnix = time.Now().Add(-d).Unix()
	}

	var reply cliproto.DiagReply
	err := daemon.Call(ipcSocket(), cliproto.IPCDiag, req, &reply)
	if err != nil {
		return err
	}

	if bundle {
		return writeBundle(reply)
	}
	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(reply)
	}
	renderDefault(os.Stdout, reply, sinceStr)
	return nil
}

// renderDefault is the pit-of-success formatter. Layout (mirrors
// docs/diagnostics.md, "Reading the default output"):
//
//	<summary block>      — current state, refresh timing, URL
//	<pattern warnings>   — detector hits, ⚠ per line; absent if clean
//	<events>             — chronological tail, columns aligned
//	<hint footer>        — --since / --bundle pointers, always shown
func renderDefault(w io.Writer, reply cliproto.DiagReply, sinceStr string) {
	// Back-compat with daemons predating Phase 0: Summary may be the
	// zero value while NATSState is set. Promote the legacy field so
	// the operator still sees the right state during rollouts.
	summary := reply.Summary
	if summary.State == "" {
		summary.State = reply.NATSState
	}
	renderSummary(w, summary, reply.NATSDropsLastHour)
	if len(reply.Patterns) > 0 {
		fmt.Fprintln(w)
		for _, p := range reply.Patterns {
			fmt.Fprintf(w, "⚠ %s  %s\n   %s\n",
				p.Name,
				p.At.Local().Format("15:04:05"),
				p.Detail,
			)
		}
	}
	fmt.Fprintln(w)
	renderEvents(w, reply.NATSEvents, reply.OnDiskCount, sinceStr)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "→ Full history:  ppz diagnostics --since=1h")
	fmt.Fprintln(w, "→ Bug report:    ppz diagnostics --bundle  (writes ~/ppz-diag-<ts>.tgz)")
}

// renderSummary prints the top block. Zero values render as "—" so the
// output stays interpretable on a fresh daemon that hasn't connected
// yet (no point lying with "1970-01-01T00:00:00Z").
func renderSummary(w io.Writer, s cliproto.DiagSummary, dropsLastHour int) {
	state := s.State
	if state == "" {
		state = "unknown"
	}
	since := "—"
	if !s.StateSince.IsZero() {
		since = fmt.Sprintf("since %s, %s ago",
			s.StateSince.Local().Format("15:04:05"),
			time.Since(s.StateSince).Round(time.Second),
		)
	}
	fmt.Fprintf(w, "nats: %s  (%s)  drops_last_hour=%d\n", state, since, dropsLastHour)

	refresh := "—"
	if !s.RefreshLastAt.IsZero() {
		refresh = fmt.Sprintf("last %s (%s ago)",
			s.RefreshLastAt.Local().Format("15:04:05"),
			time.Since(s.RefreshLastAt).Round(time.Second),
		)
		if !s.RefreshNextDueAt.IsZero() {
			dueIn := time.Until(s.RefreshNextDueAt).Round(time.Second)
			refresh += fmt.Sprintf(", next due in %s", dueIn)
		}
	}
	fmt.Fprintf(w, "refresh: %s\n", refresh)

	url := s.URL
	if url == "" {
		url = "—"
	}
	fmt.Fprintf(w, "url: %s\n", url)
}

// renderEvents prints the chronological tail. Format is space-
// separated, not table-bordered, so it's grep-friendly. The per-line
// shape is:
//
//	<type> <RFC3339-timestamp> caller=<name> nc=<id> reason="<text>"
//
// Type is at column 0 by contract — e2e tests grep for
// ^(disconnect|reconnect|daemon_start|daemon_stop) at line start
// (tests/daemon/diagnostics-shows-lifecycle-events,
// tests/reliability/nats-events-recorded-in-diagnostics). This shape
// also matches the pre-Phase-0 format so external scrapers don't
// break.
func renderEvents(w io.Writer, events []cliproto.DiagEvent, onDiskCount int, sinceStr string) {
	if len(events) == 0 {
		fmt.Fprintln(w, "Recent events: (none)")
		return
	}
	scope := "ring"
	if sinceStr != "" {
		scope = "--since=" + sinceStr
	}
	older := onDiskCount - len(events)
	if older < 0 {
		older = 0
	}
	fmt.Fprintf(w, "Recent events (%d shown, scope=%s, %d older on disk):\n", len(events), scope, older)
	for _, ev := range events {
		caller := ev.Caller
		if caller == "" {
			caller = "?"
		}
		nc := ev.NCID
		if nc == "" {
			nc = "—"
		}
		fmt.Fprintf(w, "%-12s %s  caller=%-28s nc=%s  reason=%q\n",
			ev.Type,
			ev.At.UTC().Format(time.RFC3339),
			caller,
			nc,
			ev.Reason,
		)
	}
}

// writeBundle produces a gzipped tar at ~/ppz-diag-<ts>.tgz containing:
//
//   - nats-events.jsonl + rotated generations (.1, .2)
//   - diagnostics-lifecycle.jsonl
//   - daemon.pid
//   - state.json (live login + cursors state — verbatim, per
//     "no PII constraints" decision; revisit if that changes)
//   - diagnostics.json (the IPC reply at bundle time)
//   - version.txt + uname.txt
//
// Returns the bundle path on stdout. Failures print to stderr but
// don't delete partial files — the operator can inspect what was
// captured even if one input was unreadable.
func writeBundle(reply cliproto.DiagReply) error {
	ts := time.Now().Format("20060102-150405")
	dest := bundleDestination(ts)
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	h := home()
	// Every persisted log + state file. Missing inputs are skipped
	// silently; the manifest below records what was attempted.
	candidates := []string{
		"nats-events.jsonl",
		"nats-events.jsonl.1",
		"nats-events.jsonl.2",
		"diagnostics-lifecycle.jsonl",
		"daemon.pid",
		"state.json",
	}
	manifest := strings.Builder{}
	manifest.WriteString("ppz diagnostics bundle\ngenerated_at: " + ts + "\nhome: " + h + "\n\nincluded:\n")
	for _, name := range candidates {
		path := filepath.Join(h, name)
		if err := addFileToTar(tw, path, name); err == nil {
			manifest.WriteString("  + " + name + "\n")
		} else {
			manifest.WriteString("  - " + name + " (" + err.Error() + ")\n")
		}
	}
	// The live diagnostics snapshot — captures patterns + summary at
	// bundle time, so the reader has the analysed view without re-
	// running ppz diagnostics against a now-different state.
	if b, err := json.MarshalIndent(reply, "", "  "); err == nil {
		_ = addBytesToTar(tw, "diagnostics.json", b)
		manifest.WriteString("  + diagnostics.json\n")
	}
	// Version + uname for cross-referencing against release notes.
	_ = addBytesToTar(tw, "version.txt", []byte(version.Version+"\n"))
	manifest.WriteString("  + version.txt\n")
	if u := unameOutput(); u != "" {
		_ = addBytesToTar(tw, "uname.txt", []byte(u))
		manifest.WriteString("  + uname.txt\n")
	}
	// runtime.GOOS / GOARCH are always available even when uname -a
	// isn't (e.g. minimal containers).
	_ = addBytesToTar(tw, "platform.txt", []byte(fmt.Sprintf("goos=%s goarch=%s\n", runtime.GOOS, runtime.GOARCH)))
	manifest.WriteString("  + platform.txt\n")

	_ = addBytesToTar(tw, "MANIFEST", []byte(manifest.String()))

	// Flush in order: tar → gzip → file. Any buffered data that hasn't
	// been written to disk yet is flushed here; a Close error means the
	// archive on disk is incomplete/corrupt, so remove it rather than
	// printing a path the user will trust.
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("bundle tar flush: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("bundle gzip flush: %w", err)
	}

	fmt.Println(dest)
	return nil
}

// bundleDestination picks a path the operator can find. Prefer the
// user's home dir; fall back to CWD if HOME is unset (CI containers).
func bundleDestination(ts string) string {
	name := "ppz-diag-" + ts + ".tgz"
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, name)
	}
	return name
}

func addFileToTar(tw *tar.Writer, path, archiveName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:    archiveName,
		Size:    int64(len(data)),
		Mode:    0o600,
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func addBytesToTar(tw *tar.Writer, archiveName string, data []byte) error {
	hdr := &tar.Header{
		Name:    archiveName,
		Size:    int64(len(data)),
		Mode:    0o600,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// unameOutput is best-effort. uname -a is universally useful for
// "what OS/kernel was this on?" but isn't worth blocking the bundle
// over if it fails.
func unameOutput() string {
	cmd := exec.Command("uname", "-a")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
