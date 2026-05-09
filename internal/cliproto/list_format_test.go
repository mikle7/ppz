package cliproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// `ppz ls` gains a HUMAN column — the username that created the
// (source, pipe). Layout:
//
//	PIPE  UNREAD  BUFFERED  LAST  PAYLOAD  HUMAN
//
// HUMAN is the rightmost column. That demotes PAYLOAD from "trailing
// un-padded" to a regular padded column so HUMAN aligns across rows
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
	wantOrder := []string{"PIPE", "UNREAD", "BUFFERED", "LAST", "PAYLOAD", "HUMAN"}
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

	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "chat.") {
			continue
		}
		fields := strings.Fields(line)
		human := fields[len(fields)-1]
		if human != "foo" {
			t.Errorf("auto-pipe row %q must carry source creator 'foo' as HUMAN; got %q", line, human)
		}
	}
}

func TestPrintList_PipeLevelCreatorOverridesSource(t *testing.T) {
	// Source created by foo; user-created pipe `archive` created by bar.
	// HUMAN on the auto-pipes is foo; HUMAN on chat.archive is bar.
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
	for _, line := range strings.Split(buf.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "PIPE" {
			continue
		}
		want, ok := wantHumanByPipe[fields[0]]
		if !ok {
			continue
		}
		got := fields[len(fields)-1]
		if got != want {
			t.Errorf("row %q: HUMAN = %q, want %q", line, got, want)
		}
	}
}

func TestPrintList_ColumnAlignmentWithVaryingUsernames(t *testing.T) {
	// HUMAN width must adapt to the widest username in the rendered
	// rows. The header HUMAN must align with every data row's HUMAN
	// cell (or be the last token on each row, which is also fine
	// since HUMAN is rightmost).
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
		if fields[0] == "PIPE" {
			continue
		}
		last := fields[len(fields)-1]
		if last != "foo" && last != "longer-username" {
			t.Errorf("row %q: trailing field %q is not a known username", line, last)
		}
	}
}

func TestPrintList_PayloadColumnNoLongerSwallowsHuman(t *testing.T) {
	// Regression guard for the "PAYLOAD un-padded, HUMAN last" layout:
	// even when the preview contains internal spaces, HUMAN must remain
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

	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "s.broadcast") {
			continue
		}
		fields := strings.Fields(line)
		if got := fields[len(fields)-1]; got != "foo" {
			t.Errorf("HUMAN must be the last token on the row; got %q (line=%q)", got, line)
		}
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
		got, _ := obj["human"].(string)
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
