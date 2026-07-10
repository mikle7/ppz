package cliproto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// firstDiffLine locates where two renders diverge so a fixture
// mismatch reports one line, not two 12KB screen dumps.
func firstDiffLine(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	n := len(al)
	if len(bl) < n {
		n = len(bl)
	}
	for i := 0; i < n; i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n  live: %q\n  want: %q", i, al[i], bl[i])
		}
	}
	return fmt.Sprintf("line counts differ: live %d, want %d", len(al), len(bl))
}

// The phase-3 pre-work pin (docs/specs/agent-detection.md): a
// LiveScreen fed the same bytes incrementally — at any chunking,
// including one byte at a time, which splits every escape sequence and
// every multi-byte rune — must render identically to the one-shot
// RenderTerminal path that ppz read --tty already trusts. The wrapper
// feeds LiveScreen straight from 4096-byte PTY reads, so chunk
// boundaries land anywhere.
func TestLiveScreen_ChunkedFeedMatchesOneShot(t *testing.T) {
	fixtures := []string{"claude-session.bin", "claude-inline-session.bin", "apple-session.bin"}
	chunkSizes := []int{1, 7, 4096}
	for _, fx := range fixtures {
		raw, err := os.ReadFile(filepath.Join("testdata", fx))
		if err != nil {
			t.Fatalf("%s: %v", fx, err)
		}
		want := RenderTerminal(raw, DefaultRenderCols, DefaultRenderRows)
		for _, size := range chunkSizes {
			s := NewLiveScreen(DefaultRenderCols, DefaultRenderRows)
			for off := 0; off < len(raw); off += size {
				end := off + size
				if end > len(raw) {
					end = len(raw)
				}
				if _, err := s.Write(raw[off:end]); err != nil {
					t.Fatalf("%s chunk=%d: write: %v", fx, size, err)
				}
			}
			if got := s.Text(); got != want {
				t.Errorf("%s chunk=%d: live screen diverges from one-shot render\n%s",
					fx, size, firstDiffLine(got, want))
			}
		}
	}
}

// The explicit pin for the hazard RenderTerminal's contract warns
// about: a multi-byte rune straddling two Writes must survive —
// LiveScreen carries the partial sequence, it doesn't hand vt10x a
// torn rune.
func TestLiveScreen_CarriesPartialUTF8AcrossWrites(t *testing.T) {
	s := NewLiveScreen(80, 24)
	b := []byte("héllo ─ box") // é and ─ are multi-byte
	for i := 0; i < len(b); i++ {
		_, _ = s.Write(b[i : i+1])
	}
	if got := s.Text(); !strings.Contains(got, "héllo ─ box") {
		t.Errorf("Text() = %q, want it to contain %q", got, "héllo ─ box")
	}
}

// InputModeReassertSeq mirrors the wrapped program's live input-mode
// state so a late-joining viewer (whose retention replay may have lost
// the original enable sequences) re-enters mouse/focus/app-cursor mode
// and forwards wheel/click input (mikle7/muster#17). Empty when no such
// mode is set; each set mode contributes its idempotent enable sequence;
// a later reset clears it.
func TestLiveScreen_InputModeReassertSeq(t *testing.T) {
	s := NewLiveScreen(80, 24)
	if got := s.InputModeReassertSeq(); got != "" {
		t.Fatalf("fresh screen: got %q, want \"\"", got)
	}

	// claude's typical enable: SGR-extended button+motion mouse reporting.
	_, _ = s.Write([]byte("\x1b[?1002h\x1b[?1006h"))
	got := s.InputModeReassertSeq()
	for _, want := range []string{"\x1b[?1002h", "\x1b[?1006h"} {
		if !strings.Contains(got, want) {
			t.Errorf("after mouse enable: %q missing %q", got, want)
		}
	}

	// Focus + application-cursor keys also round-trip.
	_, _ = s.Write([]byte("\x1b[?1004h\x1b[?1h"))
	got = s.InputModeReassertSeq()
	for _, want := range []string{"\x1b[?1004h", "\x1b[?1h"} {
		if !strings.Contains(got, want) {
			t.Errorf("after focus/app-cursor enable: %q missing %q", got, want)
		}
	}

	// Disabling mouse reporting drops it from the reassert.
	_, _ = s.Write([]byte("\x1b[?1002l\x1b[?1006l"))
	if got := s.InputModeReassertSeq(); strings.Contains(got, "\x1b[?1002h") || strings.Contains(got, "\x1b[?1006h") {
		t.Errorf("after mouse disable: %q still asserts mouse", got)
	}
}

// Text uses RenderTerminal's exact trim semantics so the two paths
// stay interchangeable: per-row trailing whitespace stripped, trailing
// blank rows dropped, single trailing newline, empty screen → "".
func TestLiveScreen_TextTrimSemantics(t *testing.T) {
	s := NewLiveScreen(80, 24)
	if got := s.Text(); got != "" {
		t.Errorf("empty screen Text() = %q, want \"\"", got)
	}

	in := []byte("one\r\ntwo\r\nthree")
	_, _ = s.Write(in)
	want := RenderTerminal(in, 80, 24)
	if got := s.Text(); got != want {
		t.Errorf("Text() = %q, want RenderTerminal parity %q", got, want)
	}
}

// BottomText returns the last n lines of the visible text — the region
// screen detectors match (dialogs render at the bottom). Asking for
// more lines than exist returns everything.
func TestLiveScreen_BottomText(t *testing.T) {
	s := NewLiveScreen(80, 24)
	_, _ = s.Write([]byte("one\r\ntwo\r\nthree"))

	if got := s.BottomText(2); got != "two\nthree\n" {
		t.Errorf("BottomText(2) = %q, want %q", got, "two\nthree\n")
	}
	if got := s.BottomText(50); got != "one\ntwo\nthree\n" {
		t.Errorf("BottomText(50) = %q, want %q", got, "one\ntwo\nthree\n")
	}
}

// Resize applies to subsequent rendering: a line longer than the old
// width must not wrap after the grid has grown (the wrapper mirrors
// every SIGWINCH into the screen model, so the model always renders at
// the child's real geometry).
func TestLiveScreen_ResizeApplies(t *testing.T) {
	s := NewLiveScreen(20, 5)
	s.Resize(60, 10)
	line := "a line longer than twenty characters"
	_, _ = s.Write([]byte(line))

	if got := s.Text(); !strings.Contains(got, line) {
		t.Errorf("Text() after resize = %q, want unwrapped %q", got, line)
	}
}
