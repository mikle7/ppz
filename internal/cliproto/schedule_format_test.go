package cliproto

// RED — docs/specs/schedule.md. Rendering pins for `ppz schedule ls`
// and the schedule verb output lines. The table follows the `ppz ls`
// conventions exactly: header only when rows exist, two-space gaps,
// display-width padding, "-" for missing values, relative time by
// default with --iso flipping to RFC3339 UTC, trailing CREATOR column
// unpadded, payload preview truncated at the TruncatePayload bound.

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

// --- RelativeFuture -------------------------------------------------------

// RelativeFuture is the forward-looking sibling of RelativeTime, used
// by the NEXT column. Same unit boundaries and pluralisation.
func TestRelativeFuture_UnitsAndPlurals(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "now"},
		{time.Second, "in 1 second"},
		{30 * time.Second, "in 30 seconds"},
		{time.Minute, "in 1 minute"},
		{5 * time.Minute, "in 5 minutes"},
		{90 * time.Minute, "in 1 hour"},
		{3 * time.Hour, "in 3 hours"},
		{25 * time.Hour, "in 1 day"},
		{40 * 24 * time.Hour, "in 40 days"},
	}
	for _, c := range cases {
		if got := RelativeFuture(now.Add(c.d), now); got != c.want {
			t.Errorf("RelativeFuture(+%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// --- verb output lines ----------------------------------------------------

func TestPrintScheduleCreate_Line(t *testing.T) {
	var buf bytes.Buffer
	PrintScheduleCreate(&buf, ScheduleCreateReply{
		ID:     "a1b2c3d4",
		Target: "bob.inbox",
		NextAt: time.Date(2026, 7, 8, 8, 55, 0, 0, time.UTC),
	})
	want := "scheduled id=a1b2c3d4 to=bob.inbox next=2026-07-08T08:55:00Z\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestPrintScheduleRemove_Line(t *testing.T) {
	var buf bytes.Buffer
	PrintScheduleRemove(&buf, ScheduleRemoveReply{ID: "a1b2c3d4"})
	want := "removed schedule=a1b2c3d4\n"
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

// --- table ------------------------------------------------------------------

func scheduleFixtureRows(t *testing.T) []ScheduleInfo {
	t.Helper()
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	last := now.Add(-11 * time.Minute)
	atSpec, err := time.Parse(time.RFC3339, "2026-07-08T09:55:00+01:00")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	// Deliberately NOT sorted by NextAt — the printer must sort.
	return []ScheduleInfo{
		{
			ID: "bbbb2222", Namespace: "team1", Handle: "team", Pipe: "broadcast",
			Kind: "cron", Spec: "0 10 * * MON", TZ: "Europe/London",
			NextAt: now.Add(48 * time.Hour), LastAt: nil,
			Payload: "weekly sync", Creator: "jimmy",
		},
		{
			ID: "aaaa1111", Namespace: "", Handle: "", Pipe: "alerts",
			Kind: "every", Spec: "15m", TZ: "",
			NextAt: now.Add(4 * time.Minute), LastAt: &last,
			Payload: "heartbeat check", Creator: "bot-a",
		},
		{
			ID: "cccc3333", Namespace: "", Handle: "bob", Pipe: "inbox",
			Kind: "at", Spec: "2026-07-08T09:55:00+01:00", TZ: "",
			NextAt: atSpec.UTC(), LastAt: nil,
			Payload: "standup in 5", Creator: "jimmy",
		},
	}
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestPrintScheduleList_TableRowsAndOrder(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	withFrozenNow(t, now)
	var buf bytes.Buffer
	PrintScheduleList(&buf, scheduleFixtureRows(t), false)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want header + 3 rows:\n%s", len(lines), buf.String())
	}
	if got := collapseWS(lines[0]); got != "ID NAMESPACE PIPE SCHEDULE NEXT LAST PAYLOAD CREATOR" {
		t.Fatalf("header = %q", lines[0])
	}
	// Sorted by NEXT ascending regardless of input order: every(+4m),
	// at(+20h55m), cron(+48h).
	wantRows := []string{
		"aaaa1111 - alerts every 15m in 4 minutes 11 minutes ago heartbeat check bot-a",
		"cccc3333 - bob.inbox at 2026-07-08T09:55:00+01:00 in 20 hours - standup in 5 jimmy",
		"bbbb2222 team1 team.broadcast cron 0 10 * * MON Europe/London in 2 days - weekly sync jimmy",
	}
	for i, want := range wantRows {
		if got := collapseWS(lines[i+1]); got != want {
			t.Errorf("row %d:\n got  %q\n want %q", i+1, got, want)
		}
	}
}

func TestPrintScheduleList_ColumnsAlignByDisplayWidth(t *testing.T) {
	// Every non-trailing column must start at the same rune offset on
	// every line — the same dispWidth-padded alignment as `ppz ls`.
	// The CREATOR column is the trailing bare column.
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	withFrozenNow(t, now)
	var buf bytes.Buffer
	PrintScheduleList(&buf, scheduleFixtureRows(t), false)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("no rows:\n%s", buf.String())
	}
	// Column starts derived from the header; every row must place its
	// creator cell at the same offset as the CREATOR header.
	creatorCol := strings.Index(lines[0], "CREATOR")
	if creatorCol < 0 {
		t.Fatalf("no CREATOR header in %q", lines[0])
	}
	creators := []string{"bot-a", "jimmy", "jimmy"}
	for i, want := range creators {
		row := lines[i+1]
		if got := strings.Index(row, want); got != creatorCol {
			t.Errorf("row %d: creator %q starts at col %d, want %d (misaligned):\n%s",
				i+1, want, got, creatorCol, buf.String())
		}
	}
}

func TestPrintScheduleList_ISOTimestamps(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	withFrozenNow(t, now)
	var buf bytes.Buffer
	PrintScheduleList(&buf, scheduleFixtureRows(t), true)
	out := buf.String()
	// NEXT and LAST flip to RFC3339 UTC; never-fired LAST stays "-".
	for _, want := range []string{
		"2026-07-07T12:04:00Z", // every row NEXT
		"2026-07-07T11:49:00Z", // every row LAST
		"2026-07-08T08:55:00Z", // at row NEXT normalised to UTC
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--iso output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "in 4 minutes") || strings.Contains(out, "minutes ago") {
		t.Errorf("--iso output still contains relative time:\n%s", out)
	}
}

func TestPrintScheduleList_EmptyPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	PrintScheduleList(&buf, nil, false)
	if buf.Len() != 0 {
		t.Fatalf("empty schedule list must print nothing (no header), got %q", buf.String())
	}
}

func TestPrintScheduleList_TruncatesPayloadPreview(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	long := strings.Repeat("x", 200)
	rows := []ScheduleInfo{{
		ID: "aaaa1111", Pipe: "alerts", Kind: "every", Spec: "15m",
		NextAt: now.Add(time.Hour), Payload: long, Creator: "bot-a",
	}}
	withFrozenNow(t, now)
	var buf bytes.Buffer
	PrintScheduleList(&buf, rows, false)
	if strings.Contains(buf.String(), long) {
		t.Fatal("table must truncate the payload preview (TruncatePayload), not print 200 chars")
	}
	if !strings.Contains(buf.String(), "…") {
		t.Fatalf("truncated preview should carry the ellipsis marker:\n%s", buf.String())
	}
}

// --- JSON -------------------------------------------------------------------

func TestPrintScheduleListJSON_KeysFullPayloadNullLastAt(t *testing.T) {
	long := strings.Repeat("x", 200)
	rows := scheduleFixtureRows(t)
	rows[1].Payload = long // the "every" row — full payload must survive in JSON

	var buf bytes.Buffer
	PrintScheduleListJSON(&buf, rows)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want one JSONL row per schedule, got %d:\n%s", len(lines), buf.String())
	}
	out := buf.String()
	for _, key := range []string{
		`"id":`, `"namespace":`, `"handle":`, `"pipe":`, `"schedule":`,
		`"spec":`, `"tz":`, `"next_at":`, `"last_at":`, `"payload":`, `"creator":`,
	} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON output missing key %s:\n%s", key, out)
		}
	}
	if !strings.Contains(out, `"last_at":null`) {
		t.Errorf("never-fired rows must carry last_at:null:\n%s", out)
	}
	if !strings.Contains(out, long) {
		t.Error("JSON must carry the full untruncated payload")
	}
	if !strings.Contains(out, `"schedule":"cron"`) || !strings.Contains(out, `"tz":"Europe/London"`) {
		t.Errorf("cron row must carry schedule=cron and its IANA tz:\n%s", out)
	}
	// next_at is RFC3339 UTC like ls --json's last_at.
	if !regexp.MustCompile(`"next_at":"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z"`).MatchString(out) {
		t.Errorf("next_at must be RFC3339 UTC:\n%s", out)
	}
}

// Guard against accidental reliance on input order in the JSON path
// too — agents diff this output.
func TestPrintScheduleListJSON_SortedByNextAsc(t *testing.T) {
	rows := scheduleFixtureRows(t)
	var buf bytes.Buffer
	PrintScheduleListJSON(&buf, rows)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	var ids []string
	re := regexp.MustCompile(`"id":"([a-z0-9]{8})"`)
	for _, l := range lines {
		if m := re.FindStringSubmatch(l); m != nil {
			ids = append(ids, m[1])
		}
	}
	want := []string{"aaaa1111", "cccc3333", "bbbb2222"}
	for i := range want {
		if i >= len(ids) || ids[i] != want[i] {
			t.Fatalf("JSON rows not sorted by next_at asc: got %v, want %v", ids, want)
		}
	}
}
