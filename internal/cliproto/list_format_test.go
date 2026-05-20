package cliproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// `ppz ls` gains a CREATOR column — the username that created the
// (source, pipe). Layout:
//
//	PIPE  UNREAD  BUFFERED  LAST  PAYLOAD  CREATOR
//
// CREATOR is the rightmost column. That demotes PAYLOAD from "trailing
// un-padded" to a regular padded column so CREATOR aligns across rows
// (the previous "preview at most 60 chars" cap means the column is
// bounded — worst case ~60 chars of padding for short-payload rows).
//
// Per-row creator semantics:
//
//   - Source-level creator stamps every auto-provisioned pipe
//     (broadcast, inbox, stdin, stdout, stdctrl).
//   - User-created pipes (rows in `pipes`) carry their own creator,
//     which may differ from the source's creator (a member can create
//     a pipe on a source the owner created).
//
// On the wire, Source.CreatedBy is the source-level username and
// PipeInfo.CreatedBy is the pipe-level username; the renderer prefers
// PipeInfo.CreatedBy and falls back to Source.CreatedBy when empty
// (auto-pipes).

func fixedNow() time.Time {
	return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
}

func withFrozenNow(t *testing.T, when time.Time) {
	t.Helper()
	prev := timeNow
	timeNow = func() time.Time { return when }
	t.Cleanup(func() { timeNow = prev })
}

func sampleSourceFooChat(at time.Time) Source {
	return Source{
		Handle:    "chat",
		CreatedBy: "foo",
		PipeInfos: []PipeInfo{
			{Pipe: "broadcast", Total: 1, Unread: 1, LastAt: &at, Preview: "hello world"},
			{Pipe: "inbox", Total: 0, Unread: 0},
		},
	}
}

func TestPrintList_HumanColumnHeaderIsRightmost(t *testing.T) {
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-5 * time.Minute)

	var buf bytes.Buffer
	PrintList(&buf, []Source{sampleSourceFooChat(at)}, false)

	header := strings.SplitN(buf.String(), "\n", 2)[0]
	fields := strings.Fields(header)
	wantOrder := []string{"NAMESPACE", "PIPE", "UNREAD", "BUFFERED", "LAST", "PAYLOAD", "CREATOR"}
	if len(fields) != len(wantOrder) {
		t.Fatalf("header field count = %d (%v), want %d (%v)", len(fields), fields, len(wantOrder), wantOrder)
	}
	for i, want := range wantOrder {
		if fields[i] != want {
			t.Errorf("header field[%d] = %q, want %q (full header: %q)", i, fields[i], want, header)
		}
	}
}

func TestPrintList_AutoPipesInheritSourceCreator(t *testing.T) {
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-5 * time.Minute)

	var buf bytes.Buffer
	PrintList(&buf, []Source{sampleSourceFooChat(at)}, false)

	seen := 0
	for _, line := range strings.Split(buf.String(), "\n") {
		fields := strings.Fields(line)
		// Data rows: NAMESPACE PIPE UNREAD BUFFERED LAST PAYLOAD CREATOR.
		// Header has NAMESPACE in [0]; data has the namespace ("-" for
		// root) in [0] and `chat.<pipe>` in [1].
		if len(fields) < 2 || fields[0] == "NAMESPACE" {
			continue
		}
		if !strings.HasPrefix(fields[1], "chat.") {
			continue
		}
		human := fields[len(fields)-1]
		if human != "foo" {
			t.Errorf("auto-pipe row %q must carry source creator 'foo' as CREATOR; got %q", line, human)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("no chat.* rows found — the test would pass vacuously without this guard")
	}
}

func TestPrintList_PipeLevelCreatorOverridesSource(t *testing.T) {
	// Source created by foo; user-created pipe `archive` created by bar.
	// CREATOR on the auto-pipes is foo; CREATOR on chat.archive is bar.
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-5 * time.Minute)
	src := Source{
		Handle:    "chat",
		CreatedBy: "foo",
		PipeInfos: []PipeInfo{
			{Pipe: "archive", CreatedBy: "bar", Total: 0, Unread: 0},
			{Pipe: "broadcast", Total: 1, Unread: 1, LastAt: &at, Preview: "hello"},
			{Pipe: "inbox", Total: 0, Unread: 0},
		},
	}

	var buf bytes.Buffer
	PrintList(&buf, []Source{src}, false)

	wantHumanByPipe := map[string]string{
		"chat.archive":   "bar",
		"chat.broadcast": "foo",
		"chat.inbox":     "foo",
	}
	seen := 0
	for _, line := range strings.Split(buf.String(), "\n") {
		fields := strings.Fields(line)
		// Header row starts with NAMESPACE; data rows put PIPE at [1].
		if len(fields) < 2 || fields[0] == "NAMESPACE" {
			continue
		}
		want, ok := wantHumanByPipe[fields[1]]
		if !ok {
			continue
		}
		got := fields[len(fields)-1]
		if got != want {
			t.Errorf("row %q: CREATOR = %q, want %q", line, got, want)
		}
		seen++
	}
	if seen != len(wantHumanByPipe) {
		t.Fatalf("expected to verify %d rows, only saw %d — pipe lookup is missing rows", len(wantHumanByPipe), seen)
	}
}

func TestPrintList_ColumnAlignmentWithVaryingUsernames(t *testing.T) {
	// CREATOR width must adapt to the widest username in the rendered
	// rows. The header CREATOR must align with every data row's CREATOR
	// cell (or be the last token on each row, which is also fine
	// since CREATOR is rightmost).
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-5 * time.Minute)
	sources := []Source{
		{Handle: "a", CreatedBy: "foo", PipeInfos: []PipeInfo{
			{Pipe: "broadcast", Total: 1, Unread: 0, LastAt: &at, Preview: "x"},
		}},
		{Handle: "b", CreatedBy: "longer-username", PipeInfos: []PipeInfo{
			{Pipe: "broadcast", Total: 1, Unread: 0, LastAt: &at, Preview: "y"},
		}},
	}
	var buf bytes.Buffer
	PrintList(&buf, sources, false)

	out := buf.String()
	// Each non-header line ends in either "foo" or "longer-username".
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "NAMESPACE" {
			continue
		}
		last := fields[len(fields)-1]
		if last != "foo" && last != "longer-username" {
			t.Errorf("row %q: trailing field %q is not a known username", line, last)
		}
	}
}

func TestPrintList_PayloadColumnNoLongerSwallowsHuman(t *testing.T) {
	// Regression guard for the "PAYLOAD un-padded, CREATOR last" layout:
	// even when the preview contains internal spaces, CREATOR must remain
	// a separable trailing token.
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-5 * time.Minute)
	src := Source{
		Handle: "s", CreatedBy: "foo",
		PipeInfos: []PipeInfo{
			{Pipe: "broadcast", Total: 1, Unread: 1, LastAt: &at, Preview: "hello world from foo"},
		},
	}
	var buf bytes.Buffer
	PrintList(&buf, []Source{src}, false)

	seen := 0
	for _, line := range strings.Split(buf.String(), "\n") {
		fields := strings.Fields(line)
		// PIPE column lives at index 1 once NAMESPACE owns index 0.
		if len(fields) < 2 || fields[0] == "NAMESPACE" || fields[1] != "s.broadcast" {
			continue
		}
		if got := fields[len(fields)-1]; got != "foo" {
			t.Errorf("CREATOR must be the last token on the row; got %q (line=%q)", got, line)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("no s.broadcast row found — guard against the previous HasPrefix(line, ...) form passing vacuously")
	}
}

func TestPrintListJSON_IncludesHumanField(t *testing.T) {
	at := fixedNow().Add(-5 * time.Minute)
	src := Source{
		Handle: "chat", CreatedBy: "foo",
		PipeInfos: []PipeInfo{
			{Pipe: "archive", CreatedBy: "bar"},
			{Pipe: "broadcast", Total: 1, Unread: 1, LastAt: &at, Payload: "hello"},
			{Pipe: "inbox"},
		},
	}
	var buf bytes.Buffer
	PrintListJSON(&buf, []Source{src})

	wantHuman := map[string]string{
		"archive":   "bar",
		"broadcast": "foo",
		"inbox":     "foo",
	}
	seen := 0
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("non-json output line %q: %v", line, err)
		}
		pipe, _ := obj["pipe"].(string)
		want, ok := wantHuman[pipe]
		if !ok {
			t.Errorf("unexpected pipe %q in output", pipe)
			continue
		}
		got, _ := obj["creator"].(string)
		if got != want {
			t.Errorf("pipe %q: human=%q want %q (line=%q)", pipe, got, want, line)
		}
		// payload key must remain present alongside human.
		if _, has := obj["payload"]; !has {
			t.Errorf("pipe %q: missing required 'payload' key (line=%q)", pipe, line)
		}
		seen++
	}
	if seen != 3 {
		t.Errorf("expected 3 JSON rows, saw %d", seen)
	}
}

func TestPrintList_EmptyInputEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	PrintList(&buf, nil, false)
	if buf.Len() != 0 {
		t.Errorf("empty input must produce zero output (no orphan header); got %q", buf.String())
	}
}
