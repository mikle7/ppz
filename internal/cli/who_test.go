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
	beat := func(handle, harness, model, owner string, age time.Duration, seq uint64) cliproto.WhoEntry {
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
			Owner:     owner,
			Payload:   string(raw),
			ArrivedAt: now.Add(-age),
		}
	}
	return []cliproto.WhoEntry{
		beat("alice", "claude", "opus", "alice-owner", 10*time.Second, 5),    // online
		beat("bob", "codex", "", "bob-owner", 120*time.Second, 2),            // stale
		beat("carol", "agy", "", "", 10*time.Minute, 99),                     // offline, no owner
		beat("dave", "claude", "sonnet", "alice-owner", 5*time.Second, 1),    // online, same owner as alice
	}
}

// Table renderer must show every entry, status mapped via the daemon's
// tri-state rule. Default render: no colours (renderWho's TTY/colour
// choice is the caller's responsibility — opts.UseColor controls it).
func TestRenderWho_TablePlainText(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: false})

	for _, want := range []string{"HANDLE", "STATUS", "HARNESS", "MODEL", "HOST", "OS/ARCH", "CREATED", "OWNER",
		"alice", "online", "claude", "opus", "alice-owner",
		"bob", "stale", "codex", "bob-owner",
		"carol", "offline", "agy",
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

// The CREATED column should show when the agent was started — derived
// from the heartbeat payload's started_at — as a relative-time string
// matching `ppz ls` semantics ("just now" / "N seconds ago" / "N
// minutes ago" / ...). A healthy agent beats every 60s, so the prior
// "time since last beat" reading bounced between 0–60s and duplicated
// information already in STATUS. CREATED matches `kubectl get pods`'s
// AGE column / `docker ps`'s CREATED column.
func TestRenderWho_CreatedColumnIsRelativeTimeFromStartedAt(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	beat := func(handle string, startedAgo, arrivedAgo time.Duration) cliproto.WhoEntry {
		payload := HeartbeatPayload{
			TS:          now.Add(-arrivedAgo).Format(time.RFC3339),
			Seq:         1,
			Hostname:    "h",
			OS:          "linux",
			Arch:        "amd64",
			StartedAt:   now.Add(-startedAgo).Format(time.RFC3339),
			IntervalSec: 60,
		}
		raw, _ := json.Marshal(payload)
		return cliproto.WhoEntry{
			Handle:    handle,
			Payload:   string(raw),
			ArrivedAt: now.Add(-arrivedAgo),
		}
	}
	entries := []cliproto.WhoEntry{
		// fresh beat (5s ago) but long-lived agent (2h uptime) → "2 hours ago".
		beat("long-lived", 2*time.Hour, 5*time.Second),
		// stale-ish beat (90s ago) on a young agent (90s uptime) → "1 minute ago".
		beat("just-booted", 90*time.Second, 90*time.Second),
	}
	out := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: false})

	for _, want := range []string{"CREATED", "long-lived", "just-booted", "2 hours ago", "1 minute ago"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered table missing %q; got:\n%s", want, out)
		}
	}
	// Make sure we're not falling back to ArrivedAt: an arrival 5
	// seconds ago on long-lived would show "5 seconds ago" and betray
	// the bug.
	if strings.Contains(out, "5 seconds ago") {
		t.Errorf("CREATED column shows beat freshness (5 seconds ago) instead of uptime; got:\n%s", out)
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

// Empty Owner renders as "-" (matches HARNESS/MODEL/HOST fallback) so
// the column never collapses and pre-ownership-resolution rows stay
// visually consistent with the rest.
func TestRenderWho_OwnerEmptyShowsDash(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "table", UseColor: false})

	// carol has Owner == "" in sampleWhoEntries — its data row should
	// carry a "-" in the OWNER column.
	var carolLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "carol ") {
			carolLine = line
			break
		}
	}
	if carolLine == "" {
		t.Fatalf("carol row missing in output:\n%s", out)
	}
	// OWNER is the last column. After the trailing "CREATED" value,
	// the line ends with " - " or just "-" depending on tab alignment.
	// Assert the line ends with "-" (with optional trailing whitespace).
	trimmed := strings.TrimRight(carolLine, " \t")
	if !strings.HasSuffix(trimmed, "-") {
		t.Errorf("carol's OWNER cell should be '-' for empty owner; line was:\n%s", carolLine)
	}
}

// --owner=X filters in AND-combination with --harness, symmetric with
// how --harness works today.
func TestFilterWhoEntries_ByOwner(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	got := filterWhoEntries(entries, now, whoFilter{Owner: "alice-owner"})
	if h := handlesOf(got); !equalStrSlice(h, []string{"alice", "dave"}) {
		t.Errorf("--owner=alice-owner: want [alice dave], got %v", h)
	}
	got = filterWhoEntries(entries, now, whoFilter{Owner: "nobody"})
	if len(got) != 0 {
		t.Errorf("--owner=nobody: want empty, got %v", handlesOf(got))
	}
}

// JSON wire shape carries the owner verbatim so consumers don't have
// to make a separate lookup.
func TestRenderWho_JSONIncludesOwner(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	entries := sampleWhoEntries(now)
	out := renderWho(entries, now, whoRenderOpts{Format: "json", UseColor: false})
	for _, owner := range []string{"alice-owner", "bob-owner"} {
		if !strings.Contains(out, `"owner": "`+owner+`"`) && !strings.Contains(out, `"owner":"`+owner+`"`) {
			t.Errorf("JSON missing owner %q; got:\n%s", owner, out)
		}
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
