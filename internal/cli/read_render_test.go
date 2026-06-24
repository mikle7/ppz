package cli

import (
	"strings"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// captureStdout (help_test.go) redirects os.Stdout to a pipe whose fd is not
// a TTY, so it also exercises the non-interactive branch of the TTY-gated
// render helpers — exactly the `ppz read alice.inbox | cat` / agent / CI case.

// The tabular render call site (renderReadMessageTabular) must produce
// reflow-free, uncoloured output when stdout is NOT an interactive terminal
// — the agent / pipe / redirect / CI contract. This drives the real call
// site (not FormatReadMessage directly), so it catches a call site that
// computes the wrap width with the wrong, non-TTY-gated helper: a long body
// that fits today's `ppz read | cat` must stay a single row with no
// synthetic newlines an agent can't distinguish from the originals.
func TestRenderReadMessageTabular_NonTTYDoesNotWrap(t *testing.T) {
	// ~225 chars on one logical line — would wrap at any positive width
	// budget (e.g. the 80-col fallback) but must not off a TTY.
	longLine := strings.TrimSpace(strings.Repeat("word ", 45))
	msg := cliproto.ReadMessage{
		Sender:    "alice",
		CreatedAt: "2026-05-07T14:23:01Z",
		Payload:   longLine,
	}

	got := captureStdout(t, func() { renderReadMessageTabular(msg) })

	if strings.Contains(got, "\x1b[") {
		t.Errorf("non-TTY tabular render must carry no ANSI escapes: %q", got)
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("non-TTY tabular render must not wrap (got %d rows); the call "+
			"site is reflowing piped output:\n%s", len(lines), got)
	}
}
