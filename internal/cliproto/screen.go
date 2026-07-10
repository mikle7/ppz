package cliproto

import (
	"strings"
	"sync"
	"unicode/utf8"

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
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := p
	if len(s.pending) > 0 {
		merged = append(s.pending, p...)
		s.pending = nil
	}
	complete, partial := splitTrailingPartialRune(merged)
	// Copy the carry: `merged` may alias the caller's buffer, which the
	// PTY read loop reuses for the next chunk.
	s.pending = append([]byte(nil), partial...)
	if len(complete) > 0 {
		_, _ = s.term.Write(complete)
	}
	return len(p), nil
}

// splitTrailingPartialRune splits off an incomplete multi-byte UTF-8
// sequence at the end of b, so vt10x never sees a torn rune (its Write
// decodes runes and would mangle the fragment). Anything that can't be
// the start of a longer sequence passes through as-is — invalid bytes
// are vt10x's problem, only *incomplete* ones are held back.
func splitTrailingPartialRune(b []byte) (complete, partial []byte) {
	for back := 1; back <= utf8.UTFMax && back <= len(b); back++ {
		i := len(b) - back
		c := b[i]
		if c < utf8.RuneSelf { // ASCII can't start or continue a sequence
			return b, nil
		}
		if c&0xC0 == 0x80 { // continuation byte: keep scanning back
			continue
		}
		// c is a leading byte at i. Complete if its sequence fits.
		if r, size := utf8.DecodeRune(b[i:]); r != utf8.RuneError || size > 1 {
			return b, nil
		}
		return b[:i], b[i:]
	}
	return b, nil
}

// InputModeReassertSeq returns an escape sequence that re-establishes the
// wrapped program's current input-affecting private modes — mouse reporting
// (X10/button/motion/any + SGR extended coords), focus events, and
// application-cursor keys — on a freshly-connected viewer. Returns "" when
// none of those modes is set.
//
// Why it exists: a viewer that joins mid-session (ppz terminal attach/watch)
// only replays whatever's still in JetStream retention. On a long-running
// agent, the program's original mode-enable sequences (emitted once at
// startup) have long since aged out of the ring, so the viewer's local
// terminal never learns the program wants mouse/focus input — wheel scrolls
// and clicks are swallowed as local scrollback instead of being forwarded
// (mikle7/muster#17). The source, which sees every byte, always knows the
// live mode state; re-emitting it to .stdout on connect closes the gap. All
// sequences are idempotent, so re-asserting to viewers already in sync is
// harmless.
func (s *LiveScreen) InputModeReassertSeq() string {
	s.mu.Lock()
	m := s.term.Mode()
	s.mu.Unlock()

	var b strings.Builder
	if m&vt10x.ModeMouseX10 != 0 {
		b.WriteString("\x1b[?9h")
	}
	if m&vt10x.ModeMouseButton != 0 {
		b.WriteString("\x1b[?1000h")
	}
	if m&vt10x.ModeMouseMotion != 0 {
		b.WriteString("\x1b[?1002h")
	}
	if m&vt10x.ModeMouseMany != 0 {
		b.WriteString("\x1b[?1003h")
	}
	if m&vt10x.ModeMouseSgr != 0 {
		b.WriteString("\x1b[?1006h")
	}
	if m&vt10x.ModeFocus != 0 {
		b.WriteString("\x1b[?1004h")
	}
	if m&vt10x.ModeAppCursor != 0 {
		b.WriteString("\x1b[?1h")
	}
	return b.String()
}

// Resize changes the grid size (wrapper calls this on SIGWINCH,
// mirroring the size it propagates to the child PTY).
func (s *LiveScreen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.term.Resize(cols, rows)
}

// Text returns the current visible screen as plain text with
// RenderTerminal's trim semantics: per-row trailing whitespace
// stripped, trailing blank rows dropped, single trailing newline when
// non-empty.
func (s *LiveScreen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return terminalText(s.term)
}

// BottomText returns the last n lines of Text() — the region harness
// screen detectors match against (prompts and dialogs render at the
// bottom of the screen).
func (s *LiveScreen) BottomText(n int) string {
	text := s.Text()
	if text == "" || n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if n < len(lines) {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}
