package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdBroadcast accepts the payload as:
//
//	ppz broadcast -m "text"
//	ppz broadcast "text"             (positional shortcut)
//	echo "text" | ppz broadcast      (line-streaming default)
//	cat block.txt | ppz broadcast --eof  (atomic, one message)
//
// Stdin is only consumed when neither -m nor a positional arg is supplied
// AND stdin is not a tty — otherwise we'd hang on an interactive terminal.
//
// Default stdin mode is line-streaming: each `\n`-terminated line becomes
// its own message. This matches Unix pipe convention and avoids the
// `tail -f log | ppz broadcast` deadlock the previous atomic-default
// produced. Empty lines are skipped (no point publishing nothing).
// Each successful publish prints "sent id=…" — users wanting silent
// streaming can redirect (`… | ppz broadcast >/dev/null`).
//
// `--eof` opts back into atomic mode: read all of stdin, publish a
// single message (trailing `\n` stripped, mirroring previous behaviour).
func cmdBroadcast(args []string) error {
	fs := flag.NewFlagSet("broadcast", flag.ExitOnError)
	msg := fs.String("m", "", "payload (otherwise positional or stdin)")
	eof := fs.Bool("eof", false, "with stdin: read until EOF and publish as one atomic message (default: line-streaming, one message per line)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) > 1 {
		fmt.Fprintln(os.Stderr, `usage: ppz broadcast [-m TEXT] ["TEXT" | --eof | <stdin>]`)
		os.Exit(2)
	}

	// PPZ_CURRENT_HANDLE overrides the daemon's "current handle" — used by
	// `ppz terminal` to bind the wrapped child's broadcast to the
	// terminal's pipe regardless of what the user's daemon happens to
	// have set elsewhere.
	base := cliproto.BroadcastRequest{Session: sessionID()}
	if h := os.Getenv("PPZ_CURRENT_HANDLE"); h != "" {
		base.Handle = h
	}
	publish := func(payload string) error {
		req := base
		req.Payload = payload
		var reply cliproto.BroadcastReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCBroadcast, req, &reply); err != nil {
			return err
		}
		cliproto.PrintBroadcast(os.Stdout, reply)
		return nil
	}

	switch {
	case *msg != "":
		return publish(*msg)
	case len(rest) == 1:
		return publish(rest[0])
	}

	// Stdin path. Refuse interactive tty — would hang silently.
	fi, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		fmt.Fprintln(os.Stderr, "ppz broadcast: no payload (use -m TEXT, a positional arg, or pipe stdin)")
		os.Exit(2)
	}

	if *eof {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		return publish(strings.TrimRight(string(b), "\n"))
	}

	// Line-streaming default. Buffer max is 1 MiB; the daemon will
	// reject any single line larger than the 64 KiB envelope cap with
	// E_PAYLOAD_TOO_LARGE. Each line publishes a "sent id=…" line —
	// quiet mode is a `>/dev/null` away.
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if err := publish(line); err != nil {
			return err
		}
	}
	return sc.Err()
}
