package cli

import "golang.org/x/term"

// setLocalRawMode puts the given fd into raw mode if it's a real terminal,
// returning a restore function that undoes the change. If the fd is not a
// tty (pipe / file / /dev/null) it's a safe no-op — callers can defer the
// returned function unconditionally.
//
// Used by `ppz terminal view` so that locally-typed keystrokes don't get
// echoed by the user's terminal line discipline (which would otherwise
// happen even though we drain stdin in a goroutine — local echo is the
// terminal emulator's job, not ours).
func setLocalRawMode(fd uintptr) func() {
	if !term.IsTerminal(int(fd)) {
		return func() {}
	}
	oldState, err := term.MakeRaw(int(fd))
	if err != nil {
		return func() {}
	}
	return func() {
		_ = term.Restore(int(fd), oldState)
	}
}
