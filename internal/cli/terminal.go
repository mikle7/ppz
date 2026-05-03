package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdTerminal dispatches `ppz terminal <subverb>`.
//
//   wrap H [-- CMD ...]   create a pty source bound to handle H and run
//                         CMD (or $SHELL) inside it.
//   watch H               follow H.stdout in alt-screen TUI (interactive,
//                         renders to the local terminal).
//   read H [flags]        wrapper for `read <H>.stdout` with --tty as the
//                         default output mode. Use this for capturable
//                         text snapshots (agents, pipes, scripts).
func cmdTerminal(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz terminal {share|watch|read} <handle> [...]")
		os.Exit(2)
	}
	switch args[0] {
	case "share":
		return cmdTerminalShare(args[1:])
	case "watch":
		return cmdTerminalView(args[1:])
	case "read":
		return cmdTerminalRead(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz terminal: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// cmdTerminalRead: ppz terminal read <handle> [reread-flags]
//
// Thin wrapper around `ppz reread <handle>.stdout`. Defaults the output
// mode to --tty (vt10x screen render) — the right answer for the agent-
// inspecting-another-agent's-screen use case: rebuild the cumulative
// screen state from every retained byte. Routes through `reread` (the
// forensic verb) so it never advances the caller's cursor — peeking at
// a pty session shouldn't mark its bytes "read" from the perspective of
// some other tool reading the same pipe.
//
// All reread flags (--raw / --json / -l / --skip / --since) work; passing
// an explicit output-mode flag suppresses the --tty default.
//
// Implementation: build a transformed argv (handle → handle.stdout,
// inject --tty when appropriate) and delegate to cmdReread.
func cmdTerminalRead(args []string) error {
	// Recognise reread's value-flags so we can step over them when finding
	// the positional handle.
	valueFlags := map[string]bool{
		"-l": true, "-skip": true, "--skip": true, "-since": true, "--since": true,
	}
	modeFlag := func(a string) bool {
		switch a {
		case "-tty", "--tty", "-raw", "--raw", "-json", "--json":
			return true
		}
		return false
	}

	var newArgs []string
	var handle string
	hasMode := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			newArgs = append(newArgs, a)
			if modeFlag(a) || strings.HasPrefix(a, "--tty=") || strings.HasPrefix(a, "--raw=") || strings.HasPrefix(a, "--json=") {
				hasMode = true
			}
			// Consume value-flag's value (next token) so we don't treat
			// it as the positional handle.
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				newArgs = append(newArgs, args[i+1])
				i++
			}
			continue
		}
		if handle != "" {
			fmt.Fprintln(os.Stderr, "usage: ppz terminal read <handle> [reread-flags]")
			os.Exit(2)
		}
		handle = a
	}

	if handle == "" {
		fmt.Fprintln(os.Stderr, "usage: ppz terminal read <handle> [reread-flags]")
		os.Exit(2)
	}
	if strings.Contains(handle, ".") {
		// Don't accept "handle.pipe" — terminal read is .stdout-only.
		return cliproto.New(cliproto.EInvalidHandle)
	}

	// Prepend the transformed target. cmdReread's splitReadArgs handles
	// the positional regardless of position relative to flags.
	newArgs = append([]string{handle + ".stdout"}, newArgs...)

	// Inject --tty unless the user picked a mode explicitly.
	if !hasMode {
		newArgs = append(newArgs, "--tty")
	}

	return cmdReread(newArgs)
}

// cmdTerminalShare: ppz terminal share [<handle>] [-- <cmd> [args...]]
//
// "Share" because the operation is bidirectional: the wrapped pty's stdout
// is published to <handle>.stdout for subscribers to read, AND messages
// published to <handle>.stdin are fed back into the pty as input. So the
// terminal is genuinely shared with whatever consumers attach.
//
// Two invocation shapes:
//
//	ppz terminal share            uses the daemon's current source. Auto-
//	                              creates stdin + stdout pipes on it if
//	                              missing (via the same idempotent path as
//	                              `pipe create`). Errors with
//	                              E_NO_CURRENT_SOURCE if no current is set.
//	ppz terminal share H          creates source H as kind=pty (broadcast +
//	                              stdin + stdout auto-provisioned), then
//	                              shares.
//
// The share loop:
//   - allocates a real PTY,
//   - runs the child command (or $SHELL) inside it,
//   - publishes the PTY master's byte stream to <handle>.stdout,
//   - subscribes to <handle>.stdin and forwards messages to the PTY master,
//   - exports PPZ_CURRENT_HANDLE=<handle> so the child's `ppz broadcast`
//     targets this terminal's source.
//
// Foreground only — blocks until the child exits.
func cmdTerminalShare(args []string) error {
	// Detect bare invocation: no positional handle (either no args, or args
	// start with "--" indicating only a child command was given).
	bare := len(args) == 0 || args[0] == "--"

	var handle string
	var rest []string
	if bare {
		// Resolve current source from daemon.
		var st cliproto.StatusReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCStatus,
			cliproto.StatusRequest{Session: sessionID()}, &st); err != nil {
			return err
		}
		if st.Current == "" {
			return cliproto.New(cliproto.ENoCurrentSource)
		}
		handle = st.Current
		rest = args
	} else {
		handle = args[0]
		rest = args[1:]
	}
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}

	cmdArgs := rest
	if len(cmdArgs) == 0 {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmdArgs = []string{shell}
	}

	if bare {
		// Source already exists. Ensure stdin + stdout + stdctrl pipes are
		// provisioned via the same path users would invoke manually.
		// Idempotent on E_PIPE_TAKEN — the source might already have these
		// (pty kind, or previously shared).
		for _, name := range []string{"stdin", "stdout", "stdctrl"} {
			var reply cliproto.PipeCreateReply
			err := daemon.Call(ipcSocket(), cliproto.IPCPipeCreate,
				cliproto.PipeCreateRequest{Handle: handle, Name: name}, &reply)
			if err != nil {
				if e, ok := err.(*cliproto.Error); ok && e.Code == cliproto.EPipeTaken {
					continue
				}
				return err
			}
		}
	} else {
		// Provision a fresh pty source: server-side creates the row + the
		// auto-provisioned streams (broadcast, stdin, stdout).
		var createReply cliproto.CreateReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCCreate,
			cliproto.CreateRequest{Handle: handle, Kind: string(cliproto.KindPTY)},
			&createReply); err != nil {
			return err
		}
	}

	// Allocate PTY ourselves (instead of pty.Start) so we can configure
	// termios on the *slave* before the child inherits it. macOS doesn't
	// honour termios ioctls on the master fd; setting them on the slave
	// is the canonical, cross-platform approach (it's what tmux/screen do).
	ptmx, tty, err := pty.Open()
	if err != nil {
		return fmt.Errorf("pty open: %w", err)
	}

	// Disable just the input-echo bits on the slave. Keep OPOST (\n→\r\n)
	// and ICANON intact so `ls` etc. render normally and shells with
	// readline still get cooked input where appropriate.
	disablePTYEcho(tty.Fd())

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(),
		"PPZ_CURRENT_HANDLE="+handle,
	)
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = tty.Close()
		_ = ptmx.Close()
		return fmt.Errorf("pty start: %w", err)
	}
	// Child has its own copy of the slave fd via fork+dup; release ours so
	// EOF on master read fires when the child exits.
	_ = tty.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If our stdin is a real terminal, switch it to raw mode (so keystrokes
	// pass through unbuffered + un-echoed by the local tty) and propagate
	// resizes via SIGWINCH to the wrapped child's PTY. Both are no-ops
	// when stdin is a pipe/file/devnull (test runners, scripted use).
	//
	// When stdin isn't a tty there's no source size to inherit, so we set
	// a sensible default (80x24) — that way subscribers always see a
	// concrete pty size on .stdctrl, and the wrapped child renders for
	// some real geometry instead of the kernel's 0×0 default.
	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))
	if stdinIsTTY {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
		}
		_ = pty.InheritSize(os.Stdin, ptmx)
		publishWinsize(handle, ptmx)

		winch := make(chan os.Signal, 1)
		signal.Notify(winch, syscall.SIGWINCH)
		go func() {
			for range winch {
				_ = pty.InheritSize(os.Stdin, ptmx)
				publishWinsize(handle, ptmx)
			}
		}()
	} else {
		_ = pty.Setsize(ptmx, &pty.Winsize{Cols: 80, Rows: 24})
		publishWinsize(handle, ptmx)
	}

	var wg sync.WaitGroup

	// Local stdin → PTY master with a control-response filter. The local
	// terminal emits DA / cursor-position / focus replies into our stdin
	// in response to queries the wrapped child (or its outer shell) made;
	// passing those through unmodified lets shells with self-inserting
	// readline echo them to the .stdout stream. See filterControlResponses
	// for the matched shapes.
	go func() {
		buf := make([]byte, 1024)
		var pending []byte
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				input := buf[:n]
				if len(pending) > 0 {
					input = append(pending, input...)
					pending = nil
				}
				filtered, p := filterControlResponses(input)
				pending = p
				if len(filtered) > 0 {
					_, _ = ptmx.Write(filtered)
				}
			}
			if err != nil {
				if len(pending) > 0 {
					_, _ = ptmx.Write(pending)
				}
				return
			}
		}
	}()

	// PTY master → fan out: (1) local stdout so the user sees the wrapped
	// child like a normal terminal, (2) per-line publisher to <handle>.stdout.
	wg.Add(1)
	go func() {
		defer wg.Done()
		publishAndDisplayStdout(handle, ptmx, os.Stdout)
	}()

	// Subscribe to <handle>.stdin → write to PTY master (external `ppz send`
	// reaches the wrapped child via this path).
	wg.Add(1)
	go func() {
		defer wg.Done()
		forwardStdin(ctx, handle, ptmx)
	}()

	// Wait for child. Give the kernel + reader a brief window to drain any
	// final bytes still in the PTY buffer; then close the master so the
	// reader's blocking Read returns and goroutines exit.
	_ = cmd.Wait()
	time.Sleep(200 * time.Millisecond)
	_ = ptmx.Close()
	cancel()
	wg.Wait()
	return nil
}

// publishAndDisplayStdout reads the PTY master in chunks. Each chunk is
// fanned out two ways:
//   (a) written verbatim to `display` (the user's local stdout) so the
//       wrapped terminal looks normal,
//   (b) published verbatim to <handle>.stdout — one message per chunk, no
//       transformation. ANSI escapes survive intact, so `ppz terminal view`
//       can replay the session in a terminal emulator and `ppz read
//       <h>.stdout --json` gives byte-faithful access to log consumers.
//
// One read, two consumers — same fan-out as `script(1)` / `screen`.
func publishAndDisplayStdout(handle string, master io.Reader, display io.Writer) {
	buf := make([]byte, 4096)
	var pending []byte // trailing partial UTF-8 sequence carried across reads
	for {
		n, err := master.Read(buf)
		if n > 0 {
			// Display verbatim — local stdout is our user's terminal,
			// which can handle partial bytes correctly because they get
			// completed by the next chunk's bytes arriving immediately.
			_, _ = display.Write(buf[:n])

			// For the publish path, splice carry-over from the previous
			// read in front, then split off any trailing incomplete
			// UTF-8 sequence so JSON marshalling doesn't rewrite
			// truncated bytes as U+FFFD on .stdout consumers.
			merged := buf[:n]
			if len(pending) > 0 {
				merged = append(pending, merged...)
				pending = nil
			}
			complete, partial := splitOnUTF8Boundary(merged)
			pending = partial

			if len(complete) > 0 {
				_ = sendStreamLine(handle, "stdout", string(complete))
			}
		}
		if err != nil {
			// Flush any final partial bytes (even if invalid) — better
			// to ship them than drop, parity with legacy behaviour.
			if len(pending) > 0 {
				_ = sendStreamLine(handle, "stdout", string(pending))
				pending = nil
			}
			return
		}
	}
}

// sendStreamLine publishes one message to <handle>.<channel> via daemon IPC.
// Errors are swallowed — the publisher loop is best-effort and shouldn't
// abort the whole terminal session if NATS hiccups.
func sendStreamLine(handle, channel, payload string) error {
	var reply cliproto.BroadcastReply
	return daemon.Call(ipcSocket(), cliproto.IPCBroadcast,
		cliproto.BroadcastRequest{Handle: handle, Channel: channel, Payload: payload},
		&reply)
}

// publishWinsize reads the current pty size and publishes a JSON resize
// event to <handle>.stdctrl. Best-effort: a Getsize/publish failure
// shouldn't abort the share. Subscribers (currently the GUI WebSocket
// viewer) read the latest stdctrl message + follow updates to keep
// xterm.js sized to match the source pty — bytes laid out for one width
// can't render right at another.
func publishWinsize(handle string, ptmx *os.File) {
	rows, cols, err := pty.Getsize(ptmx)
	if err != nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"type": "resize",
		"cols": cols,
		"rows": rows,
	})
	if err != nil {
		return
	}
	_ = sendStreamLine(handle, "stdctrl", string(payload))
}

// forwardStdin opens an IPC Read with follow=true on <handle>.stdin and
// pipes every received message into the PTY master (with a trailing \n).
// The receiver appends \n because shell programs reading from a tty expect
// newline-terminated input lines.
func forwardStdin(ctx context.Context, handle string, master *os.File) {
	conn, err := net.Dial("unix", ipcSocket())
	if err != nil {
		return
	}
	defer conn.Close()

	body, _ := json.Marshal(cliproto.ReadRequest{
		Handle:    handle,
		Channel:   "stdin",
		Follow:    true,
		NoAdvance: true,
	})
	if err := json.NewEncoder(conn).Encode(map[string]any{"method": cliproto.IPCRead, "params": json.RawMessage(body)}); err != nil {
		return
	}

	// Close the connection when the parent ctx ends so the daemon stops
	// streaming.
	go func() { <-ctx.Done(); _ = conn.Close() }()

	dec := bufio.NewScanner(conn)
	dec.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for dec.Scan() {
		var evt cliproto.ReadEvent
		if err := json.Unmarshal(dec.Bytes(), &evt); err != nil {
			continue
		}
		if evt.Message == nil {
			continue
		}
		// Append a newline if the payload doesn't already end with one —
		// stdin readers in shells expect line-delimited input.
		payload := evt.Message.Payload
		if !strings.HasSuffix(payload, "\n") {
			payload += "\n"
		}
		_, _ = io.WriteString(master, payload)
	}
}

// cmdTerminalView: ppz terminal watch <handle>
// (function name preserved for git-history continuity; the verb is `watch`).
//
// A proper TUI viewer for the wrapped pty's .stdout channel. On startup it
// enters the alternate screen (\x1b[?1049h) so the user's existing scroll-
// back is preserved; while running it drains stdin so any escape responses
// the local terminal emits (DA1, focus, cursor-position) are absorbed
// rather than leaking to the post-view shell session; on Ctrl-C / SIGTERM
// it restores the alternate screen and exits cleanly.
//
// Trade-offs vs an `attach`-style viewer (deferred):
//   - User keystrokes during view are discarded (drained to /dev/null).
//     If you want to type into the wrapped session, that's a separate
//     `terminal attach` mode, not built yet.
//   - No grid emulation: viewers with different terminal sizes from the
//     source still render at the source's geometry. Mid-session join
//     replays whatever's in JetStream retention; your local terminal
//     converges on the right state but you may see flicker.
//   - No SIGWINCH propagation back to the source.
func cmdTerminalView(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz terminal watch <handle>")
		os.Exit(2)
	}
	handle := args[0]
	if handle == "" || strings.Contains(handle, ".") {
		// view takes a bare handle — channel is implicit (.stdout).
		return cliproto.New(cliproto.EInvalidHandle)
	}

	conn, err := net.Dial("unix", ipcSocket())
	if err != nil {
		return cliproto.New(cliproto.EDaemonNotRunning)
	}
	defer conn.Close()

	body, _ := json.Marshal(cliproto.ReadRequest{
		Handle:    handle,
		Channel:   "stdout",
		Follow:    true,
		Session:   sessionID(),
		NoAdvance: true,
	})
	if err := json.NewEncoder(conn).Encode(map[string]any{
		"method": cliproto.IPCRead,
		"params": json.RawMessage(body),
	}); err != nil {
		return err
	}

	// Put the local tty in raw mode so user keystrokes aren't echoed
	// locally by the terminal's line discipline. Drained below — but the
	// echo is the emulator's job, only stoppable by clearing ECHO. No-op
	// when stdin isn't a tty (test runner, scripted use). Tested by
	// TestSetLocalRawMode_*.
	restoreRaw := setLocalRawMode(os.Stdin.Fd())
	defer restoreRaw()

	// Enter alt screen; ensure we exit it no matter how we leave this
	// function (normal completion, error, panic). Sequence:
	//   \x1b[?1049h  enter alt screen, save state
	//   \x1b[H       cursor home
	//   \x1b[2J      erase screen
	//   ...payload bytes flow here...
	//   \x1b[?1049l  exit alt screen, restore previous content + cursor
	_, _ = io.WriteString(os.Stdout, "\x1b[?1049h\x1b[H\x1b[2J")
	defer func() {
		_, _ = io.WriteString(os.Stdout, "\x1b[?1049l")
	}()

	// SIGINT / SIGTERM → close socket → daemon stops sending → we drain
	// any remaining events and exit. The deferred alt-screen-exit and
	// raw-mode-restore then run.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() { <-ctx.Done(); _ = conn.Close() }()

	// Drain stdin while we're alive. In raw mode Ctrl-C arrives as byte
	// 0x03 (not a signal) so we look for it explicitly and trigger the
	// same exit path as SIGINT. All other bytes are silently consumed —
	// view is read-only by design (see WIRE.md notes; an `attach` mode
	// that forwards keystrokes is a separate verb).
	go func() {
		buf := make([]byte, 256)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				switch buf[i] {
				case 0x03, 0x04: // Ctrl-C, Ctrl-D
					cancel()
					return
				}
			}
		}
	}()

	dec := bufio.NewScanner(conn)
	dec.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for dec.Scan() {
		var evt cliproto.ReadEvent
		if err := json.Unmarshal(dec.Bytes(), &evt); err != nil {
			continue
		}
		if evt.Error != nil {
			return evt.Error
		}
		if evt.Message != nil {
			_, _ = io.WriteString(os.Stdout, evt.Message.Payload)
		}
	}
	return nil
}

// silence unused import errors when we trim things out during dev.
var _ = errors.New
