package cliproto

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// All printers below match the pinned stdout in WIRE.md §8 byte-for-byte.
// Tests diff against expected.txt after normalization — do not change spacing
// or wording without updating WIRE.md and the test fixtures.

// RelativeTime renders the gap from t→now as a coarse human duration:
// "just now" / "N seconds ago" / "N minutes ago" / "N hours ago" / "N days ago".
// Used in `ppz ls` output and the server GUI org page so operators see
// freshness at a glance without parsing absolute timestamps. Negative
// durations (clock skew, future timestamps) clamp to "just now".
func RelativeTime(t, now time.Time) string {
	d := now.Sub(t)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		n := int(d / time.Second)
		return fmt.Sprintf("%d %s ago", n, pluralUnit(n, "second"))
	}
	if d < time.Hour {
		n := int(d / time.Minute)
		return fmt.Sprintf("%d %s ago", n, pluralUnit(n, "minute"))
	}
	if d < 24*time.Hour {
		n := int(d / time.Hour)
		return fmt.Sprintf("%d %s ago", n, pluralUnit(n, "hour"))
	}
	n := int(d / (24 * time.Hour))
	return fmt.Sprintf("%d %s ago", n, pluralUnit(n, "day"))
}

func pluralUnit(n int, unit string) string {
	if n == 1 {
		return unit
	}
	return unit + "s"
}

func PrintDaemonStarted(w io.Writer, pid int) {
	fmt.Fprintf(w, "daemon started pid=%d\n", pid)
}

func PrintDaemonAlreadyRunning(w io.Writer, pid int) {
	fmt.Fprintf(w, "daemon already running pid=%d\n", pid)
}

func PrintStatus(w io.Writer, s StatusReply) {
	PrintStatusWithEnv(w, s, "", "", false)
}

// statusColors carries ANSI palette closures. When color is false they
// return the input string unchanged so test fixtures and piped output
// stay byte-identical to the no-color case.
type statusColors struct {
	green func(string) string // healthy state, e.g. "logged in", current handle
	red   func(string) string // bad state, e.g. "not running", "authentication error"
	dim   func(string) string // muted, e.g. "-" placeholder for unset current
}

func newStatusColors(enabled bool) statusColors {
	wrap := func(code string) func(string) string {
		if !enabled {
			return func(s string) string { return s }
		}
		return func(s string) string { return "\x1b[" + code + "m" + s + "\x1b[0m" }
	}
	return statusColors{
		green: wrap("32"),
		red:   wrap("31"),
		dim:   wrap("2"),
	}
}

// PrintStatusWithEnv prints `ppz status`. Four states map to the top
// `daemon:` line; the rest of the body depends on which state we're in:
//
//   - not running: just the one line.
//   - not logged in: pid + a hint pointing at `ppz login`.
//   - authentication error: pid + server URL + a hint to refresh the
//     credential. The daemon learned this from a server call returning
//     E_INVALID_API_KEY.
//   - logged in: pid + server URL + org name + current source.
//
// envCurrent / currentJsonPath are the CLI-side facts surfaced for the
// env-vs-daemon disagreement annotation (see source-current-env-precedence
// for the contract). Only consulted when we're in the "logged in" state.
//
// color toggles ANSI colour escapes. The CLI flips it on for interactive
// stdout and off for pipes / NO_COLOR / e2e fixtures.
func PrintStatusWithEnv(w io.Writer, s StatusReply, envCurrent, currentJsonPath string, color bool) {
	PrintStatusWithEnvAndCLIVersion(w, s, envCurrent, currentJsonPath, color, "")
}

func PrintStatusWithEnvAndCLIVersion(w io.Writer, s StatusReply, envCurrent, currentJsonPath string, color bool, cliVersion string) {
	c := newStatusColors(color)
	if s.DaemonPID == 0 {
		fmt.Fprintf(w, "daemon: %s\n", c.red("not running"))
		return
	}
	if !s.LoggedIn {
		fmt.Fprintf(w, "daemon: %s (pid=%d)%s\n", c.red("not logged in"), s.DaemonPID, daemonVersionSuffix(c, s.DaemonVersion, cliVersion))
		fmt.Fprintln(w, "hint: run 'ppz login URL -apikey K'")
		return
	}
	if s.LoginCheck == LoginCheckInvalid {
		fmt.Fprintf(w, "daemon: %s (pid=%d)%s\n", c.red("authentication error"), s.DaemonPID, daemonVersionSuffix(c, s.DaemonVersion, cliVersion))
		fmt.Fprintf(w, "server: %s\n", s.URL)
		fmt.Fprintln(w, "hint: run 'ppz login URL -apikey K' to refresh")
		return
	}
	// LoginCheck is "ok" or "" (probe failed for transient reasons —
	// don't lie, but also don't refuse to render). Treat both as the
	// happy path; the next server-touching call will refresh the cache.
	fmt.Fprintf(w, "daemon: %s (pid=%d)%s\n", c.green("logged in"), s.DaemonPID, daemonVersionSuffix(c, s.DaemonVersion, cliVersion))
	if s.LastTokenRefreshAt != nil {
		fmt.Fprintf(w, "last token refresh: %s\n", coloredTokenRefreshAge(c, *s.LastTokenRefreshAt, timeNow()))
	} else {
		fmt.Fprintf(w, "last token refresh: %s\n", c.dim("-"))
	}
	fmt.Fprintf(w, "server: %s\n", c.green(s.URL))
	if name := s.OrgName; name != "" {
		fmt.Fprintf(w, "org: %s\n", c.green(name))
	} else {
		fmt.Fprintf(w, "org: %s\n", c.green(s.OrgID))
	}

	daemonCurrent := s.Current
	switch {
	case envCurrent != "" && daemonCurrent != "" && envCurrent != daemonCurrent:
		fmt.Fprintf(w, "current source: %s (env PPZ_CURRENT_HANDLE)\n", c.green(envCurrent))
		fmt.Fprintf(w, "current source: %s (pid=%d, %s)\n", c.green(daemonCurrent), s.DaemonPID, currentJsonPath)
		fmt.Fprintln(w, "warning: current source is set twice, env takes precedence")
	case envCurrent != "":
		fmt.Fprintf(w, "current source: %s\n", c.green(envCurrent))
	case daemonCurrent != "":
		fmt.Fprintf(w, "current source: %s\n", c.green(daemonCurrent))
	default:
		fmt.Fprintf(w, "current source: %s\n", c.dim("-"))
	}
}

func daemonVersionSuffix(c statusColors, daemonVersion, cliVersion string) string {
	if strings.TrimSpace(cliVersion) == "" {
		return ""
	}
	display := strings.TrimSpace(daemonVersion)
	if display == "" {
		display = "version unknown"
	}
	if versionsMatch(display, cliVersion) {
		return fmt.Sprintf(", %s (latest)", c.green(display))
	}
	return fmt.Sprintf(", %s (not latest)", c.red(display))
}

func versionsMatch(a, b string) bool {
	return normaliseVersionForCompare(a) == normaliseVersionForCompare(b)
}

func normaliseVersionForCompare(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

func coloredTokenRefreshAge(c statusColors, t, now time.Time) string {
	age := now.Sub(t)
	text := RelativeTime(t, now)
	if age >= 5*time.Minute {
		return c.red(text)
	}
	return c.green(text)
}

func PrintLogin(w io.Writer, r LoginReply) {
	fmt.Fprintf(w, "logged in url=%s key=%s org=%s\n", r.URL, r.KeyPrefix, r.OrgID)
}

func PrintCreate(w io.Writer, r CreateReply) {
	fmt.Fprintf(w, "created handle=%s subject=%s\n", r.Handle, r.Subject)
	fmt.Fprintf(w, "current handle=%s\n", r.Handle)
}

func PrintSwitch(w io.Writer, r SwitchReply) {
	fmt.Fprintf(w, "current handle=%s\n", r.Handle)
}

func PrintBroadcast(w io.Writer, r BroadcastReply) {
	fmt.Fprintf(w, "sent id=%s subject=%s bytes=%d\n", r.ID, r.Subject, r.Bytes)
}

func PrintConnect(w io.Writer, r ConnectReply) {
	fmt.Fprintf(w, "connected source=%s\n", r.Handle)
}

func PrintDisconnect(w io.Writer) {
	fmt.Fprintln(w, "disconnected")
}

// PrintPipeCreate prints the pinned line:
//
//	created pipe=<H>.<N> retention=ttl=<dur>,msgs=<n>,bytes=<b>
//
// `dur` is rendered via time.Duration's String() so 24h/168h round-trip
// without manual zero-pad. `bytes` is the raw integer.
func PrintPipeCreate(w io.Writer, r PipeCreateReply) {
	dur := time.Duration(r.TTLSeconds) * time.Second
	fmt.Fprintf(w, "created pipe=%s.%s retention=ttl=%s,msgs=%d,bytes=%d\n",
		r.Handle, r.Name, dur.String(), r.MaxMsgs, r.MaxBytes)
}

func PrintPipeDestroy(w io.Writer, r PipeDestroyReply) {
	fmt.Fprintf(w, "destroyed pipe=%s.%s\n", r.Handle, r.Name)
}

func PrintSourceDestroy(w io.Writer, r SourceDestroyReply) {
	fmt.Fprintf(w, "destroyed source=%s\n", r.Handle)
}

// PrintList prints `ppz ls` output: one line per (source, pipe), sorted
// by handle then pipe name. Format:
//
//	<handle>.<pipe>  <unread>  <buffered>  <last_at|->  <preview60|->
//
// UNREAD comes before BUFFERED — agents typically only need the unread
// count to decide whether to call `ppz read`; BUFFERED (the total
// retained in the pipe) is secondary and useful for forensics.
//
// Empty list (no sources) produces no output.
// listRow flattens one (source, pipe) pair into the columns the printers
// align on. iso=true switches the LAST column from relative time to
// RFC3339; the last column otherwise displays "just now" / "5 minutes ago".
type listRow struct {
	pipeColumn string // "<handle>.<pipe>"
	unread     uint64
	buffered   uint64 // total retained messages currently in the stream
	last       string // either RFC3339, relative duration, or "-"
	payload    string // truncated preview (already includes "…" if cut)
}

// PrintList renders sources as an aligned table with a header row. Default
// time format is relative duration ("5 minutes ago" / "just now"); pass
// iso=true for RFC3339 timestamps in the LAST column instead.
//
// The PAYLOAD column is the last — it isn't padded, so a long preview
// extending past the alignment grid doesn't break the row count.
func PrintList(w io.Writer, sources []Source, iso bool) {
	now := timeNow()
	rows := make([]listRow, 0)
	for _, s := range sources {
		for _, p := range s.PipeInfos {
			rows = append(rows, listRow{
				pipeColumn: s.Handle + "." + p.Pipe,
				unread:     p.Unread,
				buffered:   p.Total,
				last:       lastColumn(p.LastAt, now, iso),
				payload:    payloadColumn(p.Preview),
			})
		}
	}
	writeListTable(w, rows)
}

// PrintListJSON emits one JSON object per (source, pipe) row in the same
// shape as the API + a `last_at` ISO string. Full untruncated payload —
// `ppz ls` is the only path that surfaces the latest payload without
// going through `ppz read`, so agents reading --json get the real bytes.
func PrintListJSON(w io.Writer, sources []Source) {
	for _, s := range sources {
		for _, p := range s.PipeInfos {
			obj := map[string]any{
				"handle":  s.Handle,
				"pipe":    p.Pipe,
				"total":   p.Total,
				"unread":  p.Unread,
				"payload": p.Payload,
			}
			if p.LastAt != nil {
				obj["last_at"] = p.LastAt.UTC().Format(time.RFC3339)
			} else {
				obj["last_at"] = nil
			}
			line, _ := json.Marshal(obj)
			fmt.Fprintln(w, string(line))
		}
	}
}

func lastColumn(t *time.Time, now time.Time, iso bool) string {
	if t == nil {
		return "-"
	}
	if iso {
		return t.UTC().Format(time.RFC3339)
	}
	return RelativeTime(*t, now)
}

func payloadColumn(preview string) string {
	if preview == "" {
		return "-"
	}
	return preview
}

// writeListTable computes max widths for the left columns and prints
// header + rows aligned. The PAYLOAD column (last) is left un-padded so
// long previews don't force every other row to gain trailing whitespace.
//
// Empty input → empty output (no orphan header). Matches the convention
// where `ls` for an empty namespace just prints nothing.
func writeListTable(w io.Writer, rows []listRow) {
	if len(rows) == 0 {
		return
	}
	headers := []string{"PIPE", "UNREAD", "BUFFERED", "LAST", "PAYLOAD"}
	widths := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
	unreads := make([]string, len(rows))
	buffereds := make([]string, len(rows))
	for i, r := range rows {
		unreads[i] = fmt.Sprintf("%d", r.unread)
		buffereds[i] = fmt.Sprintf("%d", r.buffered)
		if w := len(r.pipeColumn); w > widths[0] {
			widths[0] = w
		}
		if w := len(unreads[i]); w > widths[1] {
			widths[1] = w
		}
		if w := len(buffereds[i]); w > widths[2] {
			widths[2] = w
		}
		if w := len(r.last); w > widths[3] {
			widths[3] = w
		}
	}
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %s\n",
		widths[0], headers[0],
		widths[1], headers[1],
		widths[2], headers[2],
		widths[3], headers[3],
		headers[4],
	)
	for i, r := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %s\n",
			widths[0], r.pipeColumn,
			widths[1], unreads[i],
			widths[2], buffereds[i],
			widths[3], r.last,
			r.payload,
		)
	}
}

// timeNow is overridable in tests if we ever need deterministic relative
// times. Production: just time.Now.
var timeNow = func() time.Time { return time.Now() }

// TruncatePayload renders a payload for display in `ppz ls`: strip ANSI
// CSI escapes (ESC `[` … final-byte) and all C0 controls + DEL so the
// preview can never alter the user's terminal state, replace newlines with
// spaces, trim trailing whitespace, then cap at 60 bytes (UTF-8 safe).
//
// Without this, a wrapped terminal's last .stdout chunk — typically a
// shell prompt with cursor moves and colour escapes — would render
// verbatim mid-listing and clear the screen, set bold, etc.
func TruncatePayload(s string) string {
	s = stripANSI(s)
	s = stripControlBytes(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimRight(s, " \t")
	if len(s) > 60 {
		// Cap at 60 bytes but don't slice mid-rune. The trailing "…"
		// signals to humans that the payload was cut — full text is
		// available via `ppz read` or `ppz ls --json`.
		end := 60
		for end > 0 && (s[end-1]&0xC0) == 0x80 {
			end--
		}
		s = s[:end] + "…"
	}
	return s
}

// stripANSI removes CSI escape sequences (ESC `[` <params> <final-byte>),
// where final-byte is in the range 0x40-0x7E. Other ESC sequences (single-
// shot, OSC strings) aren't fully parsed — we just drop the bare ESC byte
// in stripControlBytes, which covers the common shell-prompt case without
// implementing a full terminal emulator.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3F {
				j++
			}
			if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7E {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// stripControlBytes drops C0 controls (< 0x20) and DEL (0x7F), preserving
// only \n / \r / \t — the caller normalises those separately.
func stripControlBytes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' || c == '\r' || c == '\t' || (c >= 0x20 && c != 0x7F) {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// FprintError writes the standard one-line error to stderr.
func FprintError(w io.Writer, err *Error) {
	fmt.Fprintf(w, "error: %s: %s\n", err.Code, err.Message)
}
