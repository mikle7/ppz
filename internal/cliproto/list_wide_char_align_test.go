package cliproto

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// RED tests for the `ppz ls` / `ppz subs ls` wide-character alignment bug.
//
// Symptom (observed in `ppz ls`): a row whose PAYLOAD preview contains an
// embedded icon/emoji (e.g. 📊 in a status-report payload) shifts the
// trailing CREATOR column out of alignment with the plain-ASCII rows above
// and below it.
//
// Root cause: writeListTable / writeSubsTable size every column with len()
// (BYTE count) while fmt's "%-*s" pads by RUNE count — and neither accounts
// for terminal DISPLAY width. A 📊 is 4 bytes, 1 rune, and 2 display columns,
// so the byte-driven width budget and the rune-driven padding both disagree
// with what the terminal actually draws, and CREATOR drifts.
//
// These tests assert the user-visible contract directly: in a fixed-width
// table, every data row must occupy the same number of DISPLAY columns up to
// the (rightmost, identical) CREATOR cell — i.e. CREATOR lines up vertically
// regardless of what the payload contains.

// displayWidthForTest approximates a terminal's rendered width of s in
// columns. ASCII and most BMP runes are one column; the emoji used by these
// fixtures (📊, U+1F4CA) is two. This is deliberately tiny — it only needs to
// be correct for the characters the fixtures use — and it is the yardstick the
// production formatter must agree with once the bug is fixed.
func displayWidthForTest(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1F000: // emoji / pictographic supplementary planes → 2 cols
			w += 2
		default:
			w += 1
		}
	}
	return w
}

// dataRowDisplayWidths returns the display width of each non-header,
// non-blank line in the rendered table, trailing spaces trimmed.
func dataRowDisplayWidths(t *testing.T, out string) []int {
	t.Helper()
	var widths []int
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "NAMESPACE" {
			continue
		}
		widths = append(widths, displayWidthForTest(strings.TrimRight(line, " ")))
	}
	return widths
}

// TestPrintList_CreatorAlignsWithEmojiPayload (RED): two rows that are
// identical in every column except PAYLOAD — one plain ASCII, one carrying an
// embedded 📊 icon — and the same CREATOR. Because CREATOR is the rightmost,
// un-padded column with identical content on both rows, the two lines must
// have the same display width. Today the emoji row is wider (its emoji renders
// 2 columns each but is budgeted/padded as 1 rune), so CREATOR misaligns.
func TestPrintList_CreatorAlignsWithEmojiPayload(t *testing.T) {
	withCOLUMNS(t, "200") // wide enough that PAYLOAD is never truncated
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-2 * time.Minute)

	sources := []Source{
		{Handle: "alpha", CreatedBy: "jamesmiles", PipeInfos: []PipeInfo{
			{Pipe: "stdout", Total: 1, Unread: 1, LastAt: &at, Preview: "plain ascii status report here"},
		}},
		{Handle: "bravo", CreatedBy: "jamesmiles", PipeInfos: []PipeInfo{
			{Pipe: "stdout", Total: 1, Unread: 1, LastAt: &at, Preview: "📊 ClashProd status report here"},
		}},
	}

	var buf bytes.Buffer
	PrintList(&buf, sources, false)

	widths := dataRowDisplayWidths(t, buf.String())
	if len(widths) != 2 {
		t.Fatalf("expected 2 data rows, got %d:\n%s", len(widths), buf.String())
	}
	if widths[0] != widths[1] {
		t.Errorf("rows misaligned: display widths %d vs %d — embedded emoji shifts CREATOR.\n%s",
			widths[0], widths[1], buf.String())
	}
}

// TestPrintSubsList_CreatorAlignsWithEmojiPayload (RED): same contract for the
// `subs ls` tree. writeSubsTable sizes the PAYLOAD column to its widest cell
// by byte length, so the emoji row's CREATOR drifts relative to the ASCII row.
func TestPrintSubsList_CreatorAlignsWithEmojiPayload(t *testing.T) {
	withCOLUMNS(t, "200")
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-2 * time.Minute)

	sources := []Source{
		{Handle: "alpha", CreatedBy: "jamesmiles", PipeInfos: []PipeInfo{
			{Pipe: "stdout", Total: 1, Unread: 1, LastAt: &at, Preview: "plain ascii status report here"},
		}},
		{Handle: "bravo", CreatedBy: "jamesmiles", PipeInfos: []PipeInfo{
			{Pipe: "stdout", Total: 1, Unread: 1, LastAt: &at, Preview: "📊 ClashProd status report here"},
		}},
	}
	subscriptions := []string{"alpha.stdout", "bravo.stdout"}

	var buf bytes.Buffer
	PrintSubsList(&buf, sources, nil, subscriptions, false)

	widths := dataRowDisplayWidths(t, buf.String())
	if len(widths) != 2 {
		t.Fatalf("expected 2 data rows, got %d:\n%s", len(widths), buf.String())
	}
	if widths[0] != widths[1] {
		t.Errorf("subs rows misaligned: display widths %d vs %d — embedded emoji shifts CREATOR.\n%s",
			widths[0], widths[1], buf.String())
	}
}
