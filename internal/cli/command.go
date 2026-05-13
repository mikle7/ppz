package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdCommand: ppz command <handle> [instruction] [--claude|--cr|--crlf|--newline|--none]
//
// Sends an optional instruction to <handle>.stdin, waits 100 ms, then sends a
// trailing control sequence.  Default sequence is \n (Enter).  Flags select
// alternatives:
//
//	--claude   \x1b[13u  (XTerm CSI-u "submit" — same as pty inbox alerting)
//	--cr       \r
//	--crlf     \r\n
//	--newline  \n        (explicit; same as the default)
//	--none               no trailing sequence — sends instruction only
func cmdCommand(args []string) error {
	// Go's flag package stops at the first non-flag argument, so we
	// pre-separate flags from positional args to support any ordering.
	fs := flag.NewFlagSet("command", flag.ContinueOnError)
	useClaude  := fs.Bool("claude", false, "trailing sequence: XTerm CSI-u submit \\x1b[13u")
	useCR      := fs.Bool("cr", false, "trailing sequence: carriage-return \\r")
	useCRLF    := fs.Bool("crlf", false, "trailing sequence: carriage-return + newline \\r\\n")
	useNewline := fs.Bool("newline", false, "trailing sequence: newline \\n (same as default)")
	useNone    := fs.Bool("none", false, "no trailing sequence — send instruction only")

	var flagArgs, rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			rest = append(rest, a)
		}
	}
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz command <handle> [instruction] [--claude|--cr|--crlf|--newline|--none]")
		os.Exit(2)
	}
	handle := rest[0]
	var instruction string
	if len(rest) > 1 {
		instruction = rest[1]
	}

	ctrlSeq := "\n"
	switch {
	case *useClaude:
		ctrlSeq = "\x1b[13u"
	case *useCR:
		ctrlSeq = "\r"
	case *useCRLF:
		ctrlSeq = "\r\n"
	case *useNewline:
		ctrlSeq = "\n"
	case *useNone:
		ctrlSeq = ""
	}

	send := func(payload string) error {
		var reply cliproto.SendReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCSend,
			cliproto.SendRequest{
				Handle:  handle,
				Channel: "stdin",
				Payload: payload,
				// Forward session id so daemon.envelope.sender resolves
				// against this tty's current source — same fix as send.go.
				Session: sessionID(),
			},
			&reply); err != nil {
			return err
		}
		cliproto.PrintBroadcast(os.Stdout, reply)
		return nil
	}

	if instruction != "" {
		if err := send(instruction); err != nil {
			return err
		}
		if ctrlSeq != "" {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if ctrlSeq != "" {
		return send(ctrlSeq)
	}
	return nil
}
