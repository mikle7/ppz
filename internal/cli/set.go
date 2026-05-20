package cli

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
)

// cmdSet implements `ppz set <key> <value>`.
//
// Day-one settable surface:
//
//	ppz set handle HANDLE   — switch the daemon's current handle
//	                          (was `ppz source switch HANDLE` pre-Phase 1)
//
// Future keys (log-level, default-writers, telemetry opt-in, …) hang
// off the same dispatch. Unknown keys return exit 2 with a clear
// message so users get an obvious nudge to `--help` if they mistype.
//
// Locked decision #20 in OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md.
func cmdSet(args []string) error {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz set <key> <value>")
		os.Exit(2)
	}
	key := args[0]
	switch key {
	case "handle":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: ppz set handle HANDLE")
			os.Exit(2)
		}
		return setHandle(args[1])
	case "namespace":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: ppz set namespace PATH")
			os.Exit(2)
		}
		return setNamespace(args[1])
	}
	fmt.Fprintf(os.Stderr, "ppz set: unknown key %q (try: handle, namespace)\n", key)
	os.Exit(2)
	return nil
}

// cmdUnset implements `ppz unset <key>`.
//
// Day-one:
//
//	ppz unset handle       — clear the daemon's current handle
//	                         (was `ppz source clear` pre-Phase 1)
func cmdUnset(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz unset <key>")
		os.Exit(2)
	}
	key := args[0]
	switch key {
	case "handle":
		return unsetHandle()
	case "namespace":
		return unsetNamespace()
	}
	fmt.Fprintf(os.Stderr, "ppz unset: unknown key %q (try: handle, namespace)\n", key)
	os.Exit(2)
	return nil
}

// cmdGet implements `ppz get <key>`.
//
// Returns the value to stdout (single-line, no trailing whitespace
// beyond \n) so callers can capture with $(ppz get handle).
func cmdGet(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: ppz get <key>")
		os.Exit(2)
	}
	key := args[0]
	switch key {
	case "handle":
		return getHandle()
	}
	fmt.Fprintf(os.Stderr, "ppz get: unknown key %q (try: handle)\n", key)
	os.Exit(2)
	return nil
}

// setHandle wires `ppz set handle HANDLE` to the daemon's existing
// IPCSwitch verb. Same daemon-side state mutation as the retired
// `ppz source switch HANDLE`, exposed under the canonical set/get
// pattern.
func setHandle(handle string) error {
	var reply cliproto.SwitchReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCSwitch,
		cliproto.SwitchRequest{Handle: handle, Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &reply); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "handle=%s\n", reply.Handle)
	warnIfHandleEnvOverride("updated")
	return nil
}

// unsetHandle clears the daemon's current handle for this session.
// Wraps IPCDisconnect (same state-clearing behaviour `ppz source clear`
// used pre-Phase 1).
func unsetHandle() error {
	var reply cliproto.DisconnectReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCDisconnect,
		cliproto.DisconnectRequest{Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &reply); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "unset")
	warnIfHandleEnvOverride("cleared")
	return nil
}

// getHandle prints the daemon's current handle for this session, or
// empty string + exit 1 if no current is set. Scripts that capture
// with $(ppz get handle) can distinguish "no current" from "current
// is foo" via exit code.
func getHandle() error {
	var reply cliproto.StatusReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCStatus,
		cliproto.StatusRequest{Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &reply); err != nil {
		return err
	}
	if reply.Current == "" {
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, reply.Current)
	return nil
}

// setNamespace wires `ppz set namespace PATH` to the daemon's
// IPCSetNamespace verb. Persists the per-session manifold so subsequent
// `ppz pipe create LEAF` calls inherit it (Phase 1.5).
func setNamespace(path string) error {
	var reply cliproto.SetNamespaceReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCSetNamespace,
		cliproto.SetNamespaceRequest{Namespace: path, Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &reply); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "namespace=%s\n", reply.Namespace)
	return nil
}

// unsetNamespace clears the daemon's namespace for this session.
func unsetNamespace() error {
	var reply cliproto.UnsetNamespaceReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCUnsetNamespace,
		cliproto.UnsetNamespaceRequest{Session: sessionID(), AncestorPIDs: ancestorPIDs()}, &reply); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "unset")
	return nil
}

// warnIfHandleEnvOverride is a slimmer cousin of warnIfEnvOverrides
// (which lived in the now-removed source.go). Prints a stderr warning
// when PPZ_CURRENT_HANDLE overrides the daemon state we just mutated.
func warnIfHandleEnvOverride(action string) {
	if env := os.Getenv("PPZ_CURRENT_HANDLE"); env != "" {
		fmt.Fprintf(os.Stderr,
			"warning: PPZ_CURRENT_HANDLE=%s is set; daemon current was %s but env still wins\n",
			env, action)
	}
}
