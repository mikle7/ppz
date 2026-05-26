package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// Tombstone tests for the removal of `ppz await`.
//
// `await` was the wrong abstraction for agents: it blocked until a
// matching pipe had unread, then DRAINED one pipe (advancing the
// cursor) as a side effect of waking. Wiring it into an idle loop
// raced any later `ppz read inbox` — the agent reported it had acted
// while the user's read showed nothing. Awareness is now `ppz ls
// --watch` (non-destructive wake signal); consumption is `ppz read`.
//
// Companion e2e fixture: tests/await/await-returns-unknown-command.

// TestUsage_OmitsAwaitVerb asserts the top-level usage text no longer
// advertises `ppz await`. Explanatory prose about waiting/awareness
// may remain — the rule is no live `ppz await` invocation example.
func TestUsage_OmitsAwaitVerb(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	for _, banned := range []string{
		"ppz await ",
		"ppz await\n",
		"ppz await[",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — `ppz await` was removed", banned)
		}
	}
}

// TestTopLevelVerbs_ExcludesAwait guards against the completion engine
// re-advertising the removed verb (it was never registered, but a
// future edit could add it back by reflex).
func TestTopLevelVerbs_ExcludesAwait(t *testing.T) {
	for _, v := range topLevelVerbs {
		if v == "await" {
			t.Errorf("topLevelVerbs still contains %q — `ppz await` was removed", v)
		}
	}
}
