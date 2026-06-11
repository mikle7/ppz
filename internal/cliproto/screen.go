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
