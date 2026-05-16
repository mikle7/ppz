package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

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
	fs := flag.NewFlagSet("await", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON envelopes instead of payload text; emits {\"event\":\"arrival\",\"pipe\":\"...\"} before each drain")
	follow := fs.Bool("tail", false, "loop until SIGINT, draining one pipe per wakeup")
	tty := fs.Bool("tty", false, "render concatenated payloads through a virtual terminal; warns if woken pipe is not stdout-shape")
	raw := fs.Bool("raw", false, "write payload bytes verbatim with no separator")
	bare := fs.Bool("bare", false, "force legacy payload-only output for tabular-eligible pipes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"inbox"}
	}

	// Resolve any bare "inbox" → "<current>.inbox" so the daemon-side
	// matcher sees a concrete pattern. Mirrors cmdRead's currentInboxTarget.
	resolved := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if p == "inbox" {
			handle, err := effectiveCurrentHandle()
			if err != nil {
				return err
			}
			resolved = append(resolved, handle+".inbox")
			continue
		}
		resolved = append(resolved, p)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for {
		if err := awaitOnce(ctx, resolved, *asJSON, *tty, *raw, *bare); err != nil {
			return err
		}
		if !*follow {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// defaultPatternsFromSnapshot expands `ppz await` (no positional args)
// to the concrete pattern list watched by default:
//
//   - `<currentHandle>.inbox` — your inbox.
//   - every uncollared pipe at currentManifold — namespace-scoped chat
//     primitives (rooms, lobbies) you're inhabiting right now.
//
// Other handles' collared pipes (e.g. `bar.inbox`) are explicitly NOT
// included — they're addressable by name when wanted.
//
// Pure function: takes a snapshot and returns a list. The IPC wiring
// that fetches handle/manifold/uncollared lives in awaitDefaultPatterns.
// Pure factoring keeps the table-driven test honest and lets us cover
// namespace-boundary semantics without mocking IPCs.
//
// Pipes added AFTER `ppz await` starts are not retroactively watched —
// patterns are resolved once at startup. Each `--tail` iteration reuses
// the same list. Acceptable for v1; the typical agent loop is fast
// enough that creating a pipe then waiting on it is a two-call flow
// (create, then re-await).
func defaultPatternsFromSnapshot(currentHandle, currentManifold string, uncollared []cliproto.UncollaredPipe) []string {
	// RED stub.
	_, _, _ = currentHandle, currentManifold, uncollared
	return nil
}

// awaitOnce performs one (block, pick, drain) cycle. Uses awaitBlock
// (not daemon.Call) so SIGINT during the block phase closes the
// socket and lets the binary exit promptly.
func awaitOnce(ctx context.Context, patterns []string, asJSON, tty, raw, bare bool) error {
	reply, err := awaitBlock(ctx, patterns)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}

	handle, pipe, ok := pickAwaitTarget(reply, patterns)
	if !ok {
		// Daemon returned without any unread (e.g. SIGINT mid-block).
		return nil
	}

	target := pipe
	if handle != "" {
		target = handle + "." + pipe
	}

	if asJSON {
		evt := map[string]string{"event": "arrival", "pipe": target}
		line, _ := json.Marshal(evt)
		fmt.Fprintln(os.Stdout, string(line))
	} else {
		fmt.Fprintf(os.Stderr, "messages arrived on %s\n", target)
	}

	// --tty against a non-stdout channel: warn but honor the flag.
	// "channel" is the pipe's leaf name. For collared pipes that's the
	// pipe field directly; for uncollared pipes the leaf is the last
	// dotted segment (e.g. "team-a.chat" → "chat").
	channel := pipe
	if handle == "" {
		if idx := strings.LastIndex(pipe, "."); idx >= 0 {
			channel = pipe[idx+1:]
		}
	}
	if tty && channel != "stdout" {
		fmt.Fprintf(os.Stderr,
			"ppz await: --tty is only meaningful for stdout-shape pipes; rendering %s anyway\n",
			target)
	}

	return runRead(target, asJSON, false /* follow */, tty, raw, bare, false /* all */, 0, 0, 0)
}

// awaitBlock issues a single IPCListWatch request and decodes the
// reply. Owns the connection so a SIGINT-driven ctx cancel can close
// the socket and unblock the daemon-side wait.
func awaitBlock(ctx context.Context, patterns []string) (cliproto.ListReply, error) {
	var reply cliproto.ListReply
	conn, err := net.Dial("unix", ipcSocket())
	if err != nil {
		return reply, cliproto.New(cliproto.EDaemonNotRunning)
	}
	defer conn.Close()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	body, _ := json.Marshal(cliproto.ListWatchRequest{Session: sessionID(), Patterns: patterns})
	if err := json.NewEncoder(conn).Encode(map[string]any{
		"method": cliproto.IPCListWatch,
		"params": json.RawMessage(body),
	}); err != nil {
		return reply, err
	}

	var resp struct {
		Result json.RawMessage  `json:"result,omitempty"`
		Error  *cliproto.Error  `json:"error,omitempty"`
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return reply, err
	}
	if resp.Error != nil {
		return reply, resp.Error
	}
	if err := json.Unmarshal(resp.Result, &reply); err != nil {
		return reply, err
	}
	return reply, nil
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
	type candidate struct {
		handle, pipe string
		lastAt       time.Time
	}
	var cands []candidate

	for _, s := range reply.Sources {
		for _, pi := range s.PipeInfos {
			if pi.Unread == 0 {
				continue
			}
			if !matchAnyTargetCLI(s.Handle, pi.Pipe, patterns) {
				continue
			}
			var t time.Time
			if pi.LastAt != nil {
				t = *pi.LastAt
			}
			cands = append(cands, candidate{handle: s.Handle, pipe: pi.Pipe, lastAt: t})
		}
	}
	for _, uc := range reply.UncollaredPipes {
		if uc.Info.Unread == 0 {
			continue
		}
		path := uc.Info.Pipe
		if path == "" {
			path = uc.Name
			if uc.Manifold != "" {
				path = uc.Manifold + "." + uc.Name
			}
		}
		if !matchAnyTargetCLI("", path, patterns) {
			continue
		}
		var t time.Time
		if uc.Info.LastAt != nil {
			t = *uc.Info.LastAt
		}
		cands = append(cands, candidate{handle: "", pipe: path, lastAt: t})
	}

	if len(cands) == 0 {
		return "", "", false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if !cands[i].lastAt.Equal(cands[j].lastAt) {
			return cands[i].lastAt.Before(cands[j].lastAt)
		}
		ti := cands[i].handle + "." + cands[i].pipe
		tj := cands[j].handle + "." + cands[j].pipe
		return ti < tj
	})
	return cands[0].handle, cands[0].pipe, true
}

// matchAnyTargetCLI mirrors the daemon-side matchAnyTarget so the CLI
// can apply the same pattern semantics when filtering snapshots
// client-side. Keep in lockstep with internal/daemon/list_watch.go's
// matchAnyTarget.
func matchAnyTargetCLI(handle, pipe string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	target := pipe
	if handle != "" {
		target = handle + "." + pipe
	}
	for _, raw := range patterns {
		p := strings.ReplaceAll(raw, "%", "*")
		if handle != "" {
			if ok, _ := filepath.Match(p, handle); ok {
				return true
			}
		}
		if ok, _ := filepath.Match(p, pipe); ok {
			return true
		}
		if ok, _ := filepath.Match(p, target); ok {
			return true
		}
	}
	return false
}
