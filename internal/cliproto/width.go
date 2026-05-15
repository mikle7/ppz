package cliproto

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// TerminalWidth reports the caller's terminal width in columns,
// suitable for layout decisions in `ppz ls`, `ppz --help`, and any
// other adaptive renderer. Resolution order:
//
//  1. COLUMNS env var — the conventional override, honoured first
//     because it's the only knob that works for pipes / scripts /
//     unit tests where stdout isn't a tty.
//  2. term.GetSize(stdout) — actual tty width when running interactively.
//  3. defaultWidth (80) — universally compatible fallback when
//     neither signal is available.
func TerminalWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}
