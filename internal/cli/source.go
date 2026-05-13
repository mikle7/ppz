package cli

import (
	"fmt"
	"os"
	"path"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdSourceGroup dispatches `ppz source <subverb>`.
//
// Phase 1 reshape (locked decisions #18-21): the four-verb family
// (create / switch / clear / destroy) is now two — create and
// destroy. Switch and clear were replaced by `ppz set handle` and
// `ppz unset handle`; their replacements are clean. Create and
// destroy survive because no replacement covers their semantics:
//
//   ppz source create HANDLE   — claim a bare actor identity
//                                 (message-kind source, with inbox
//                                 auto-pipe). Distinct from
//                                 `ppz terminal create` (pty pipe
//                                 set) and `ppz agent create` (agent
//                                 pipe set + harness).
//   ppz source destroy PATTERN — glob-destroy sources / pipes. The
//                                 expressive bits (glob across
//                                 handles, pipe-pattern matching
//                                 that crosses source boundaries,
//                                 clears-current on destroy) aren't
//                                 covered by `ppz pipe destroy
//                                 --recursive`.
func cmdSourceGroup(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz source {create|destroy} ...")
		os.Exit(2)
	}
	switch args[0] {
	case "create":
		return cmdSourceCreate(args[1:])
	case "destroy":
		return cmdSourceDestroy(args[1:])
	}
	fmt.Fprintf(os.Stderr, "ppz source: unknown subcommand %q\n", args[0])
	os.Exit(2)
	return nil
}

// cmdSourceCreate creates a bare message-kind source (auto-pipe set:
// inbox only) and sets it as the session's current handle. Strict:
// errors with E_HANDLE_TAKEN if the handle already exists in the
// account. Distinct from `ppz terminal create` and `ppz agent create`
// which provision richer pipe bundles.
func cmdSourceCreate(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz source create HANDLE")
		os.Exit(2)
	}
	var reply cliproto.CreateReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCCreate,
		cliproto.CreateRequest{Handle: args[0], Session: sessionID()}, &reply); err != nil {
		return err
	}
	cliproto.PrintCreate(os.Stdout, reply)
	warnIfHandleEnvOverride("updated")
	return nil
}

// cmdSourceDestroy implements `ppz source destroy PATTERN`.
//
// Bare patterns (no dot) match source handles — each matching source and all
// its pipes are destroyed. Dotted patterns (handle.pipe) match individual
// pipes across sources — only the pipes are destroyed, sources stay.
//
// Glob characters follow path.Match semantics: * matches any sequence, ?
// matches any single character, [abc] matches a character class.
//
// Error handling follows rm(1): each failure is printed to stderr, the loop
// continues, and the command exits non-zero if any operation failed.
func cmdSourceDestroy(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz source destroy PATTERN")
		os.Exit(2)
	}
	pattern := args[0]

	var listReply cliproto.ListReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCList,
		cliproto.ListRequest{Session: sessionID()}, &listReply); err != nil {
		return err
	}

	srcHandles, pipeTargets, err := resolveDestroyTargets(pattern, listReply.Sources)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppz source destroy: invalid pattern %q: %v\n", pattern, err)
		os.Exit(2)
	}

	if len(srcHandles) == 0 && len(pipeTargets) == 0 {
		h, _, _ := splitHandleName(pattern)
		if h != "" {
			fmt.Fprintln(os.Stdout, "0 pipes destroyed")
		} else {
			fmt.Fprintln(os.Stdout, "0 sources destroyed")
		}
		return nil
	}

	hadErr := false
	for _, h := range srcHandles {
		var reply cliproto.SourceDestroyReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCSourceDestroy,
			cliproto.SourceDestroyRequest{Handle: h}, &reply); err != nil {
			if e, ok := err.(*cliproto.Error); ok {
				fmt.Fprintf(os.Stderr, "error: %s: %s\n", e.Code, e.Message)
			} else {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			hadErr = true
			continue
		}
		cliproto.PrintSourceDestroy(os.Stdout, reply)
	}

	for _, p := range pipeTargets {
		var reply cliproto.PipeDestroyReply
		if err := daemon.Call(ipcSocket(), cliproto.IPCPipeDestroy, p, &reply); err != nil {
			if e, ok := err.(*cliproto.Error); ok {
				fmt.Fprintf(os.Stderr, "error: %s: %s\n", e.Code, e.Message)
			} else {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			hadErr = true
			continue
		}
		cliproto.PrintPipeDestroy(os.Stdout, reply)
	}

	if hadErr {
		os.Exit(1)
	}
	return nil
}

// resolveDestroyTargets parses pattern and matches it against sources.
//
// Bare pattern (no dot): matches source handles → returns handles to destroy.
// Dotted pattern (handle.pipe): matches across sources and their PipeInfos →
// returns pipe targets to destroy. The glob wildcard is path.Match semantics.
func resolveDestroyTargets(pattern string, sources []cliproto.Source) ([]string, []cliproto.PipeDestroyRequest, error) {
	handlePat, pipePat, err := splitHandleName(pattern)
	if err != nil {
		return nil, nil, err
	}

	if handlePat == "" {
		// Bare name: pipePat holds the source handle pattern.
		var handles []string
		for _, s := range sources {
			matched, err := path.Match(pipePat, s.Handle)
			if err != nil {
				return nil, nil, err
			}
			if matched {
				handles = append(handles, s.Handle)
			}
		}
		return handles, nil, nil
	}

	// Dotted form: handlePat = source glob, pipePat = pipe glob.
	var pipes []cliproto.PipeDestroyRequest
	for _, s := range sources {
		matched, err := path.Match(handlePat, s.Handle)
		if err != nil {
			return nil, nil, err
		}
		if !matched {
			continue
		}
		for _, p := range s.PipeInfos {
			matched, err := path.Match(pipePat, p.Pipe)
			if err != nil {
				return nil, nil, err
			}
			if matched {
				pipes = append(pipes, cliproto.PipeDestroyRequest{Handle: s.Handle, Name: p.Pipe})
			}
		}
	}
	return nil, pipes, nil
}
