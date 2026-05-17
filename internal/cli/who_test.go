package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Build a deterministic set of who-rows for renderer tests. Times are
// expressed relative to a fixed `now` so the AGE column is stable.
func sampleWhoEntries(now time.Time) []cliproto.WhoEntry {
	beat := func(handle, harness, model string, age time.Duration, seq uint64) cliproto.WhoEntry {
		payload := HeartbeatPayload{
			TS:          now.Add(-age).Format(time.RFC3339),
			Seq:         seq,
			Harness:     harness,
			Model:       model,
			Hostname:    "jimmy-mbp",
			OS:          "darwin",
			Arch:        "arm64",
			PID:         12345,
			PPZVersion:  "0.32.0",
			StartedAt:   now.Add(-1 * time.Hour).Format(time.RFC3339),
			IntervalSec: 60,
		}
		raw, _ := json.Marshal(payload)
		return cliproto.WhoEntry{
			Handle:    handle,
			Payload:   string(raw),
			ArrivedAt: now.Add(-age),
		}
	}
	return []cliproto.WhoEntry{
		beat("alice", "claude", "opus", 10*time.Second, 5),    // online
		beat("bob", "codex", "", 120*time.Second, 2),          // stale
		beat("carol", "gemini", "pro", 10*time.Minute, 99),    // offline
		beat("dave", "claude", "sonnet", 5*time.Second, 1),    // online
	}
}

// Table renderer must show every entry, status mapped via the daemon's
// tri-state rule. Default render: no colours (renderWho's TTY/colour
// choice is the caller's responsibility — opts.UseColor controls it).
func TestRenderWho_TablePlainText(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: false})

	for _, want := range []string{"HANDLE", "STATUS", "HARNESS", "MODEL", "HOST", "OS/ARCH", "AGE",
		"alice", "online", "claude", "opus",
		"bob", "stale", "codex",
		"carol", "offline", "gemini", "pro",
		"dave", "sonnet"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered table missing %q; got:\n%s", want, out)
		}
	}
	// Plain-text mode must not embed ANSI escape sequences.
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain-text render contains ANSI escapes:\n%s", out)
	}
}

// Colour mode wraps online/stale/offline cells with green/amber/red
// ANSI codes. Caller decides whether to enable it (TTY check + NO_COLOR
// happen in cmdWho before calling the renderer).
func TestRenderWho_TableColorWrapsStatus(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: true})

	// Each status word should appear with its colour code in front of it.
	if !strings.Contains(out, "\x1b[32monline\x1b[0m") {
		t.Errorf("expected green-wrapped online, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[33mstale\x1b[0m") {
		t.Errorf("expected amber-wrapped stale, got:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[31moffline\x1b[0m") {
		t.Errorf("expected red-wrapped offline, got:\n%s", out)
	}
}

// tabwriter measures column widths in bytes, not visible glyphs. If
// the renderer feeds it cells that already contain ANSI escape
// sequences, the STATUS column gets padded by the escape-byte
// overhead — the header line ends up ~9 spaces wider than the data
// cell beneath it. Fix: render uncolored first so tabwriter pads on
// visible width, then inject ANSI codes into the status cell as a
// post-pass. Pin the contract by asserting that stripping ANSI from
// the colored render yields exactly the plain render.
func TestRenderWho_ColorRenderAlignsWithPlainAfterStrip(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	plain := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: false})
	colored := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: true})

	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	stripped := ansi.ReplaceAllString(colored, "")
	if stripped != plain {
		t.Fatalf("ANSI-stripped colored render must equal plain render — tabwriter is over-padding because escape bytes count toward column width.\n--- plain ---\n%s\n--- stripped ---\n%s", plain, stripped)
	}
}

// JSON format dumps the raw entries — never wraps status in colour.
func TestRenderWho_JSONFormatNoColor(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "json", UseColor: true})

	if strings.Contains(out, "\x1b[") {
		t.Errorf("JSON output contains ANSI escapes:\n%s", out)
	}
	// Each handle should appear once.
	for _, h := range []string{"alice", "bob", "carol", "dave"} {
		if !strings.Contains(out, `"`+h+`"`) {
			t.Errorf("JSON missing handle %q; got:\n%s", h, out)
		}
	}
	// Each row should carry a derived status. MarshalIndent inserts a
	// space after each colon, so match either compact or indented form.
	for _, status := range []string{"online", "stale", "offline"} {
		if !strings.Contains(out, `"status": "`+status+`"`) && !strings.Contains(out, `"status":"`+status+`"`) {
			t.Errorf("JSON missing status %q; got:\n%s", status, out)
		}
	}
}

// Filters trim the result before render: --online keeps only online,
// --stale keeps only stale, --offline keeps only offline. Filters are
// inclusive when omitted (no filter → everything).
func TestFilterWhoEntries_ByStatus(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)

	got := filterWhoEntries(entries, now, whoFilter{Online: true})
	if len(got) != 2 || got[0].Handle != "alice" || got[1].Handle != "dave" {
		t.Errorf("--online filter: want [alice dave], got %v", handlesOf(got))
	}
	got = filterWhoEntries(entries, now, whoFilter{Stale: true})
	if len(got) != 1 || got[0].Handle != "bob" {
		t.Errorf("--stale filter: want [bob], got %v", handlesOf(got))
	}
	got = filterWhoEntries(entries, now, whoFilter{Offline: true})
	if len(got) != 1 || got[0].Handle != "carol" {
		t.Errorf("--offline filter: want [carol], got %v", handlesOf(got))
	}
}

func TestFilterWhoEntries_ByHarness(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	got := filterWhoEntries(entries, now, whoFilter{Harness: "claude"})
	if len(got) != 2 || got[0].Handle != "alice" || got[1].Handle != "dave" {
		t.Errorf("--harness=claude: want [alice dave], got %v", handlesOf(got))
	}
}

// Multiple status filters combine OR (online + offline → exclude stale).
func TestFilterWhoEntries_OnlineAndOfflineUnion(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	got := filterWhoEntries(entries, now, whoFilter{Online: true, Offline: true})
	want := []string{"alice", "carol", "dave"}
	if h := handlesOf(got); !equalStrSlice(h, want) {
		t.Errorf("union filter: want %v, got %v", want, h)
	}
}

// No filter set → no entries dropped.
func TestFilterWhoEntries_EmptyFilterPassesAll(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	got := filterWhoEntries(entries, now, whoFilter{})
	if len(got) != len(entries) {
		t.Errorf("empty filter dropped entries: got %d, want %d", len(got), len(entries))
	}
}

func handlesOf(entries []cliproto.WhoEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Handle
	}
	return out
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
