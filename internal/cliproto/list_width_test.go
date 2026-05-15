package cliproto

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// RED tests for v0.31.5: `ppz ls` should adapt its payload column to
// the caller's terminal width so the table fits in narrow windows
// (e.g. half a macbook screen). The current implementation caps
// payloads at 60 bytes regardless of context, which (combined with
// fixed-padded columns for PIPE/UNREAD/BUFFERED/LAST/CREATOR) produces
// rows ~110 chars wide — too wide for a typical 80-col half-screen.

// withCOLUMNS sets the COLUMNS env var for the duration of the test and
// restores it on cleanup. COLUMNS is the conventional override for
// tty-detected terminal width and is what the formatter should consult
// at print time.
func withCOLUMNS(t *testing.T, cols string) {
	t.Helper()
	t.Setenv("COLUMNS", cols)
}

// sampleSourcesLongPayloads gives PrintList a row whose payload, when
// rendered with the current 60-byte cap, blows past an 80-column
// budget. Handle is short ("alice") so the fixed-column overhead
// stays small enough that the test contract is achievable: the
// payload column has room to shrink without truncating the PIPE
// column too.
func sampleSourcesLongPayloads(at time.Time) []Source {
	return []Source{
		{
			Handle:    "alice",
			CreatedBy: "jamesmiles",
			PipeInfos: []PipeInfo{
				{
					Pipe:    "inbox",
					Total:   1,
					Unread:  1,
					LastAt:  &at,
					Preview: "Once upon a time, a small daemon named Pipe lived between two terminals on a quiet…",
				},
			},
		},
	}
}

// TestPrintList_RespectsCOLUMNSWidth (RED): when COLUMNS=80 is set in
// the environment, every rendered line must fit in 80 columns. The
// formatter is expected to allocate a dynamic payload-column width
// from the leftover budget after the other fixed columns are sized,
// truncating Preview with a trailing "…" if needed.
//
// Today the formatter ignores COLUMNS entirely and rows are governed
// only by the 60-byte cap in TruncatePayload — so the long-payload
// fixture above produces a row > 100 chars wide and this test fails.
func TestPrintList_RespectsCOLUMNSWidth(t *testing.T) {
	withCOLUMNS(t, "80")
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-1 * time.Hour)

	var buf bytes.Buffer
	PrintList(&buf, sampleSourcesLongPayloads(at), false)

	for i, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		// Renderer is allowed to emit a trailing space/newline; measure
		// the line proper (rune count, since payloads can be UTF-8).
		runeLen := len([]rune(strings.TrimRight(line, " ")))
		if runeLen > 80 {
			t.Errorf("line %d exceeds COLUMNS=80 budget: %d runes\n%s", i, runeLen, line)
		}
	}
}

// TestPrintList_RespectsCOLUMNSWidthNarrow (RED): same contract,
// tighter budget. A 60-column window (e.g. half of a 13" macbook
// screen) must still render valid rows — the payload column shrinks
// further or drops to a single "…" placeholder rather than overflowing.
func TestPrintList_RespectsCOLUMNSWidthNarrow(t *testing.T) {
	withCOLUMNS(t, "60")
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-1 * time.Hour)

	var buf bytes.Buffer
	PrintList(&buf, sampleSourcesLongPayloads(at), false)

	for i, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		runeLen := len([]rune(strings.TrimRight(line, " ")))
		if runeLen > 60 {
			t.Errorf("line %d exceeds COLUMNS=60 budget: %d runes\n%s", i, runeLen, line)
		}
	}
}

// TestPrintList_NoColumnsEnvFallsBackToCurrentWidth (PASS / lock-in):
// when COLUMNS isn't set the formatter must still produce some output
// — i.e. width detection failure mustn't crash or empty the table.
// Locks in the fallback contract (an explicit default width when the
// env probe fails). Wide bound chosen so this test continues to pass
// regardless of the chosen default.
func TestPrintList_NoColumnsEnvFallsBackToCurrentWidth(t *testing.T) {
	t.Setenv("COLUMNS", "")
	withFrozenNow(t, fixedNow())
	at := fixedNow().Add(-1 * time.Hour)

	var buf bytes.Buffer
	PrintList(&buf, sampleSourcesLongPayloads(at), false)

	if buf.Len() == 0 {
		t.Fatal("formatter produced no output with COLUMNS unset; must fall back to a sensible default")
	}
}
