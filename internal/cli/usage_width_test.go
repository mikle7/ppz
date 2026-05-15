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

// TestUsage_FitsInCOLUMNS60 (RED): same contract for a tighter
// half-macbook target. Picked 60 because the existing help has a
// 32-char prefix indent on continuation lines — anything narrower
// than that can't carry text at all without restructuring.
func TestUsage_FitsInCOLUMNS60(t *testing.T) {
	t.Setenv("COLUMNS", "60")

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
		if runeLen := len([]rune(measured)); runeLen > 60 {
			t.Errorf("usage line %d exceeds COLUMNS=60 budget: %d runes\n%s", i+1, runeLen, line)
		}
	}
}
