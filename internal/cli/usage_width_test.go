package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// RED tests for v0.31.5: `ppz --help` / `ppz` (no-args usage) is a
// big static string with pre-wrapped lines that exceed 80 columns —
// so on a half-screen 13" macbook terminal the help text wraps at
// arbitrary points and becomes hard to read. The renderer should
// consult the caller's terminal width (via COLUMNS env or
// term.GetSize) and reflow descriptions to fit.

// TestUsage_FitsInCOLUMNS80 (RED): with COLUMNS=80, no line in the
// rendered usage may exceed 80 runes. Today the static block contains
// multiple ~95-char lines so this fails.
func TestUsage_FitsInCOLUMNS80(t *testing.T) {
	t.Setenv("COLUMNS", "80")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	for i, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		// Strip trailing whitespace before measuring — alignment padding
		// doesn't count as content overflow.
		measured := strings.TrimRight(line, " \t")
		if runeLen := len([]rune(measured)); runeLen > 80 {
			t.Errorf("usage line %d exceeds COLUMNS=80 budget: %d runes\n%s", i+1, runeLen, line)
		}
	}
}

// TestUsage_FitsInCOLUMNS100 (RED): same contract at a half-16"
// macbook target. The current static block contains a small number
// of 100+ char lines (notably the multi-flag agent harness rows).
// Tighter targets (e.g. 60) aren't satisfiable today because some
// continuation indents push past 68 columns — restructuring those
// is a separate concern from the wrap-to-width fix.
func TestUsage_FitsInCOLUMNS100(t *testing.T) {
	t.Setenv("COLUMNS", "100")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	for i, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		measured := strings.TrimRight(line, " \t")
		if runeLen := len([]rune(measured)); runeLen > 100 {
			t.Errorf("usage line %d exceeds COLUMNS=100 budget: %d runes\n%s", i+1, runeLen, line)
		}
	}
}
