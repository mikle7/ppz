package cli

import (
	"errors"
	"os"

	"golang.org/x/term"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
	"github.com/pipescloud/ppz/internal/version"
)

// useColor returns true when stdout is an interactive terminal AND the
// caller hasn't opted out via NO_COLOR (https://no-color.org/). Pipes
// and e2e fixtures hit the false branch and get plain ASCII output.
func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func cmdStatus(args []string) error {
	var st cliproto.StatusReply
	err := daemon.Call(ipcSocket(), cliproto.IPCStatus,
		cliproto.StatusRequest{Session: sessionID()}, &st)
	if err != nil {
		// Daemon unreachable → "daemon: not running" on stdout, exit 11.
		var pErr *cliproto.Error
		if errors.As(err, &pErr) && pErr.Code == cliproto.EDaemonNotRunning {
			cliproto.PrintStatusWithEnv(os.Stdout, cliproto.StatusReply{}, "", "", useColor())
			os.Exit(cliproto.ExitCode(cliproto.EDaemonNotRunning))
		}
		return err
	}
	envCurrent := os.Getenv("PPZ_CURRENT_HANDLE")
	cliproto.PrintStatusWithEnvAndCLIVersion(os.Stdout, st, envCurrent, st.CurrentPath, useColor(), version.Version)
	maybeNotifyUpdate()
	return nil
}
