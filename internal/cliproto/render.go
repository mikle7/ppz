package cliproto

import (
	"strings"

	vt10x "github.com/pipescloud/ppz/internal/thirdparty/vt10x"
)

// Default emulator dimensions for peek/read text rendering. Wide enough
// that most TUIs (Claude Code, htop, vim) won't reflow text awkwardly;
// tall enough to fit a typical scrollback target. If a wrap session ran
// at narrower dimensions, its cursor-positioning bytes still land at
// the right (col, row) inside this larger grid — they just leave more
// trailing whitespace.
//
// Configurable later via flags / env / per-source metadata; hardcoded
// for now is fine since the goal is "more readable than nothing" rather
// than "byte-perfect replica of the original session".
const (
	DefaultRenderCols = 200
	DefaultRenderRows = 60
)

// RenderTerminal feeds raw PTY bytes through a virtual terminal of the
// given dimensions, then dumps the resulting screen as plain-text rows.
//
// The bytes MUST be a single contiguous buffer — vt10x's `Write` is
// UTF-8-boundary-sensitive and can lose intermediate bytes if a multi-
// byte rune straddles two `Write` calls. Callers feeding chunked input
// (multiple JetStream messages, multiple PTY reads, etc.) should
// concatenate first, then call this once.
//
// Behaviour:
//   - Cursor moves resolve into cell positions; OSC/CSI/charset noise
//     is consumed by the emulator (zero leakage in the output).
//   - Each row's trailing whitespace is trimmed.
//   - Trailing all-blank rows are dropped so a mostly-empty screen
//     produces a short snapshot rather than 60 newlines.
//   - Output ends with a single trailing `\n` if it's non-empty.
//   - Empty input produces an empty string.
//
// Limitation: alternate-screen state. If the captured byte stream is
// from a TUI that is currently *on* alt-screen (Claude Code, vim mid-
// session), vt10x renders the alt-screen content — which is what you
// want. If the TUI exited and switched back to main screen, you get the
// main-screen content (typically the shell prompt). This matches what
// `cat session.bin` would render in your terminal.
func RenderTerminal(in []byte, cols, rows int) string {
	if len(in) == 0 {
		return ""
	}
	term := vt10x.New(vt10x.WithSize(cols, rows))
	_, _ = term.Write(in)

	term.Lock()
	defer term.Unlock()

	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		var b strings.Builder
		b.Grow(cols)
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y)
			if g.Char == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(g.Char)
			}
		}
		lines[y] = strings.TrimRight(b.String(), " ")
	}

	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	if end == 0 {
		return ""
	}
	return strings.Join(lines[:end], "\n") + "\n"
}
