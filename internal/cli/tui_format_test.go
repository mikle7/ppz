package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// visibleChatLines renders the chat body and returns its plain (ANSI-stripped,
// trailing-space-trimmed) visual rows, so column/indent assertions measure what
// the eye sees rather than styling bytes or lipgloss padding.
func visibleChatLines(msgs []tMsg, w int) []string {
	var out []string
	for _, ln := range strings.Split(wrapMessages(msgs, w), "\n") {
		out = append(out, strings.TrimRight(ansi.Strip(ln), " "))
	}
	return out
}

// A multi-line payload must render as a hanging-indented block: the header row
// carries time+sender+first line, and every continuation line indents to the
// body column with no time/sender leak — the readability property `ppz read`
// has and the TUI currently lacks (DM and pipe chats both).
func TestWrapMessages_MultiLineHangingIndent(t *testing.T) {
	msgs := []tMsg{{t: "19:44", sender: "cindy", text: "line one\nline two\nline three"}}
	lines := visibleChatLines(msgs, 80)
	if len(lines) != 3 {
		t.Fatalf("want 3 rows, got %d: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "19:44") || !strings.Contains(lines[0], "cindy") {
		t.Errorf("header row missing time/sender: %q", lines[0])
	}
	idx := strings.Index(lines[0], "line one")
	if idx <= 0 {
		t.Fatalf("body 'line one' not found on header row: %q", lines[0])
	}
	indent := strings.Repeat(" ", idx)
	for i, want := range []string{"line two", "line three"} {
		cont := lines[i+1]
		if strings.Contains(cont, "19:44") {
			t.Errorf("continuation %d leaked timestamp: %q", i, cont)
		}
		if strings.Contains(cont, "cindy") {
			t.Errorf("continuation %d leaked sender: %q", i, cont)
		}
		if cont != indent+want {
			t.Errorf("continuation %d = %q, want body indented to col %d: %q", i, cont, idx, indent+want)
		}
	}
}

// A long single-line payload wraps under the body column with the same hanging
// indent, no metadata on continuation rows, and no visible row exceeds the pane.
func TestWrapMessages_LongLineWrapsToBodyColumn(t *testing.T) {
	const w = 50
	long := "this is a fairly long single line of text that should wrap across the body column several times over cleanly"
	msgs := []tMsg{{t: "19:44", sender: "cindy", text: long}}
	lines := visibleChatLines(msgs, w)
	if len(lines) < 2 {
		t.Fatalf("long line should wrap, got %d: %#v", len(lines), lines)
	}
	idx := strings.Index(lines[0], "this")
	if idx <= 0 {
		t.Fatalf("body start not found: %q", lines[0])
	}
	indent := strings.Repeat(" ", idx)
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], indent) {
			t.Errorf("wrap row %d not indented to body column: %q", i, lines[i])
		}
		if strings.Contains(lines[i], "cindy") || strings.Contains(lines[i], "19:44") {
			t.Errorf("wrap row %d leaked metadata: %q", i, lines[i])
		}
	}
	for i, ln := range lines {
		if len(ln) > w {
			t.Errorf("row %d visible width %d exceeds pane %d: %q", i, len(ln), w, ln)
		}
	}
}

// Messages from different senders share the same body column so the log reads
// as an aligned block (fixed sender column, like `ppz read`).
func TestWrapMessages_BodyColumnAlignsAcrossSenders(t *testing.T) {
	msgs := []tMsg{
		{t: "19:44", sender: "cindy", text: "a"},
		{t: "19:45", sender: "you", text: "b"},
	}
	lines := visibleChatLines(msgs, 80)
	if len(lines) < 2 {
		t.Fatalf("want >=2 rows, got %#v", lines)
	}
	c0 := strings.Index(lines[0], "a")
	c1 := strings.Index(lines[len(lines)-1], "b")
	if c0 <= 0 || c1 <= 0 {
		t.Fatalf("body tokens not found: %q / %q", lines[0], lines[len(lines)-1])
	}
	if c0 != c1 {
		t.Errorf("body columns differ across senders: %d vs %d (%q / %q)", c0, c1, lines[0], lines[len(lines)-1])
	}
}

// A blank row separates adjacent messages so you can tell where one ends and
// the next begins (the readability gap in the DM screenshots).
func TestWrapMessages_BlankLineBetweenMessages(t *testing.T) {
	msgs := []tMsg{
		{t: "19:44", sender: "cindy", text: "first"},
		{t: "19:45", sender: "cindy", text: "second"},
	}
	lines := visibleChatLines(msgs, 80)
	if len(lines) != 3 {
		t.Fatalf("want 3 rows (msg, blank, msg), got %d: %#v", len(lines), lines)
	}
	if strings.TrimSpace(lines[1]) != "" {
		t.Errorf("want a blank separator row between messages, got %q", lines[1])
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[2], "second") {
		t.Errorf("message rows misplaced: %#v", lines)
	}
}
