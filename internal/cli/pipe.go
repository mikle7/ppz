package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdPipeGroup dispatches `ppz pipe <subverb>` to create / destroy.
func cmdPipeGroup(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz pipe {create|destroy} <name> [...]")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		return cmdPipeCreate(args[1:])
	case "destroy":
		return cmdPipeDestroy(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz pipe: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// cmdPipeCreate parses `ppz pipe create [<handle>.]<name> [--ttl=DUR] [--max-msgs=N] [--max-bytes=B]`.
//
// Bare `<name>` uses the current source from daemon state (resolved daemon-
// side via the `Handle` field on the request being empty — the daemon then
// fills it from State.Current).
//
// `<handle>.<name>` is the explicit form.
//
// `--ttl` accepts a Go duration string (24h, 168h, 30m, …).
// `--max-bytes` accepts plain ints, or sizes like "64MiB" / "1GB".
func cmdPipeCreate(args []string) error {
	target, flagArgs := splitTargetAndFlags(args)
	fs := flag.NewFlagSet("pipe create", flag.ExitOnError)
	ttl := fs.Duration("ttl", 0, "stream MaxAge override (e.g. 24h, 168h)")
	maxMsgs := fs.Int("max-msgs", 0, "stream MaxMsgs override")
	maxBytesS := fs.String("max-bytes", "", "stream MaxBytes override (int, or 1KB/1MB/1GiB/...)")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: ppz pipe create [<handle>.]<name> [--ttl=DUR --max-msgs=N --max-bytes=B]")
		os.Exit(2)
	}

	handle, name, err := splitHandleName(target)
	if err != nil {
		return cliproto.New(cliproto.EInvalidPipe)
	}
	if handle == "" {
		// Phase 1.5: bare name with no explicit handle. Try the effective
		// current handle from daemon state; if there isn't one, fall
		// through with handle="" — the daemon then routes to the
		// sourceless endpoint (uncollared pipe at the root manifold).
		// Pre-Phase-1.5 this errored as "no current source set".
		resolved, err := effectiveCurrentHandle()
		if err == nil {
			handle = resolved
		}
	}

	req := cliproto.PipeCreateRequest{Handle: handle, Name: name, Session: sessionID()}
	if *ttl > 0 {
		secs := int(*ttl / time.Second)
		req.TTLSeconds = &secs
	}
	if *maxMsgs > 0 {
		req.MaxMsgs = maxMsgs
	}
	if *maxBytesS != "" {
		b, err := parseBytes(*maxBytesS)
		if err != nil {
			return cliproto.New(cliproto.EInvalidPipe)
		}
		req.MaxBytes = &b
	}

	var reply cliproto.PipeCreateReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCPipeCreate, req, &reply); err != nil {
		return err
	}
	cliproto.PrintPipeCreate(os.Stdout, reply)
	return nil
}

// cmdPipeDestroy: `ppz pipe destroy [<handle>.]<name>` or
// `ppz pipe destroy --recursive HANDLE`.
//
// The --recursive form destroys every pipe under the handle (and the
// handle's underlying source row). Replaces `ppz source destroy HANDLE`
// from pre-Phase 1 (locked decision #21). Without --recursive, the
// target must be a single pipe name — passing a bare handle would
// previously have routed through the source-destroy glob; that path
// is gone, so a bare handle without --recursive errors out.
func cmdPipeDestroy(args []string) error {
	fs := flag.NewFlagSet("pipe destroy", flag.ExitOnError)
	recursive := fs.Bool("recursive", false, "destroy all pipes under HANDLE (and the handle itself)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz pipe destroy [<handle>.]<name>")
		fmt.Fprintln(os.Stderr, "       ppz pipe destroy --recursive HANDLE")
		os.Exit(2)
	}

	if *recursive {
		// Bare handle (no dot). Use IPCSourceDestroy to clear all
		// pipes under it. The IPC verb name retains "source" for
		// now — the user-facing surface is the --recursive flag.
		var reply cliproto.SourceDestroyReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCSourceDestroy,
			cliproto.SourceDestroyRequest{Handle: rest[0]}, &reply); err != nil {
			return err
		}
		cliproto.PrintSourceDestroy(os.Stdout, reply)
		return nil
	}

	handle, name, err := splitHandleName(rest[0])
	if err != nil {
		return cliproto.New(cliproto.EInvalidPipe)
	}
	if handle == "" {
		resolved, err := effectiveCurrentHandle()
		if err != nil {
			return err
		}
		handle = resolved
	}

	var reply cliproto.PipeDestroyReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCPipeDestroy,
		cliproto.PipeDestroyRequest{Handle: handle, Name: name}, &reply); err != nil {
		return err
	}
	cliproto.PrintPipeDestroy(os.Stdout, reply)
	return nil
}

// splitTargetAndFlags lets the user write the target before OR after flags.
// Returns the first positional and the remaining args (which are flag-only).
func splitTargetAndFlags(args []string) (target string, flagArgs []string) {
	valueFlags := map[string]bool{
		"-ttl": true, "--ttl": true,
		"-max-msgs": true, "--max-msgs": true,
		"-max-bytes": true, "--max-bytes": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			if strings.Contains(a, "=") || !valueFlags[a] {
				continue
			}
			if i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		if target == "" {
			target = a
		}
	}
	return target, flagArgs
}

// splitHandleName parses "name" or "handle.name". Returns ("", name, nil)
// for bare names; ("handle", "name", nil) for the dotted form. An empty
// component on either side is an error.
func splitHandleName(s string) (handle, name string, err error) {
	idx := strings.LastIndex(s, ".")
	if idx < 0 {
		if s == "" {
			return "", "", fmt.Errorf("empty target")
		}
		return "", s, nil
	}
	if idx == 0 || idx == len(s)-1 {
		return "", "", fmt.Errorf("empty side of dotted target")
	}
	return s[:idx], s[idx+1:], nil
}

// parseBytes accepts a plain int (literal byte count) or one of the common
// SI/IEC suffixes. Case-insensitive on the suffix.
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	// Find the split between digits and the suffix.
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no leading digits")
	}
	num, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, err
	}
	suffix := strings.ToLower(strings.TrimSpace(s[i:]))
	switch suffix {
	case "", "b":
		return num, nil
	case "k", "kb":
		return num * 1000, nil
	case "ki", "kib":
		return num * 1024, nil
	case "m", "mb":
		return num * 1000 * 1000, nil
	case "mi", "mib":
		return num * 1024 * 1024, nil
	case "g", "gb":
		return num * 1000 * 1000 * 1000, nil
	case "gi", "gib":
		return num * 1024 * 1024 * 1024, nil
	}
	return 0, fmt.Errorf("unknown size suffix %q", suffix)
}
