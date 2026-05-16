package cli

import (
	"errors"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// cmdAwait: ppz await [PATTERN...] [--tail --json --bare --tty --raw]
//
// Block until a matching pipe has unread messages, then drain ONE pipe
// (the one whose latest message is oldest — FIFO-ish across pipes) by
// invoking the same engine `ppz read` uses. Default pattern is `inbox`
// (resolved to `<current>.inbox`). Patterns OR-combine; globs use `*`
// (shell-quoted) or `%` (SQL-LIKE-style unquoted alias).
//
//	--tail   loop until SIGINT, draining one pipe per wakeup. After
//	         each drain the loop re-enters the block phase; level-
//	         triggered semantics in `ls --watch` mean any still-unread
//	         pipes return immediately on the next iteration, so no
//	         message gets starved.
//	--json   emit `{"event":"arrival","pipe":"..."}` on stdout before
//	         each pipe's JSON envelopes (which themselves come from
//	         runRead --json).
//	--bare   force legacy payload-only output for tabular-eligible
//	         pipes (script-stable opt-out).
//	--tty    accepted but warns to stderr when the woken pipe's
//	         channel != "stdout" (the warning is informational; the
//	         flag is still honored).
//	--raw    accepted; single-pipe drain makes byte concat
//	         well-defined.
//
// Banner (default mode) goes to stderr: "messages arrived on <pipe>".
// JSON mode emits the arrival event to stdout instead so jq-style
// pipelines work without 2>/dev/null.
func cmdAwait(args []string) error {
	// RED stub — green PR replaces this with the full impl.
	_ = args
	return errors.New("ppz await: not implemented")
}

// pickAwaitTarget selects the single pipe to drain from a ListReply
// snapshot. Returns the pipe whose LastAt is OLDEST among those with
// Unread > 0 that also match the user's patterns. Returns ok=false
// when no candidate is found.
//
// Tie-break for equal LastAt: lexicographic on `<handle>.<pipe>` so
// repeated invocations are deterministic.
//
// The (handle, pipe) tuple maps directly to runRead targets:
//   - collared source pipe → (handle, pipe), target = handle+"."+pipe
//   - uncollared pipe      → (handle="", pipe=<pipe-path>), target =
//     <pipe-path> (where <pipe-path> may include a manifold prefix,
//     e.g. "team-a.chat")
func pickAwaitTarget(reply cliproto.ListReply, patterns []string) (handle, pipe string, ok bool) {
	// RED stub.
	_, _ = reply, patterns
	return "", "", false
}
