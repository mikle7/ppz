package cliproto

import (
	"sync"

	vt10x "github.com/pipescloud/ppz/internal/thirdparty/vt10x"
)

// LiveScreen is an incrementally-fed terminal screen model: the
// terminal-share wrapper tees the PTY output bytes into it so harness
// detection can read what's currently visible (permission prompts,
// forms) without re-rendering pipe history. It is the live counterpart
// of RenderTerminal — same emulator, same text-extraction semantics —
// but accepts chunked writes: RenderTerminal's one-contiguous-buffer
// contract exists because vt10x.Write loses bytes when a multi-byte
// rune straddles two calls, so LiveScreen carries trailing partial
// UTF-8 across Writes itself.
//
// Safe for concurrent use: the wrapper writes from the PTY read
// goroutine while the detection loop reads Text/BottomText.
type LiveScreen struct {
	mu      sync.Mutex
	term    vt10x.Terminal
	pending []byte // trailing partial UTF-8 sequence carried across writes
}

// NewLiveScreen returns a LiveScreen with the given grid size.
func NewLiveScreen(cols, rows int) *LiveScreen {
	return &LiveScreen{term: vt10x.New(vt10x.WithSize(cols, rows))}
}

// Write feeds PTY bytes into the screen model. Always reports the full
// length as written — a screen-model parse hiccup must never
// backpressure the real output path.
func (s *LiveScreen) Write(p []byte) (int, error) {
	return len(p), nil // RED skeleton — implemented after test review
}

// Resize changes the grid size (wrapper calls this on SIGWINCH,
// mirroring the size it propagates to the child PTY).
func (s *LiveScreen) Resize(cols, rows int) {
	// RED skeleton — implemented after test review
}

// Text returns the current visible screen as plain text with
// RenderTerminal's trim semantics: per-row trailing whitespace
// stripped, trailing blank rows dropped, single trailing newline when
// non-empty.
func (s *LiveScreen) Text() string {
	return "" // RED skeleton — implemented after test review
}

// BottomText returns the last n lines of Text() — the region harness
// screen detectors match against (prompts and dialogs render at the
// bottom of the screen).
func (s *LiveScreen) BottomText(n int) string {
	return "" // RED skeleton — implemented after test review
}
