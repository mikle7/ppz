//go:build linux

package cli

import "golang.org/x/sys/unix"

// Per-OS termios ioctl request constants. Linux uses TCGETS/TCSETS;
// Darwin uses TIOCGETA/TIOCSETA. Both apply to a tty fd's line
// discipline; we expose them under a stable name so cross-platform
// code (including tests) can share helpers.
const (
	ttyGetTermios = unix.TCGETS
	ttySetTermios = unix.TCSETS
)

// setPTYInputEcho toggles the PTY line discipline's ECHO bits and returns a
// restore function for the previous termios state.
//
// Setting termios on the master fd works on Linux because the line
// discipline is shared between master and slave.
func setPTYInputEcho(fd uintptr, enabled bool) func() {
	t, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	if err != nil {
		return func() {}
	}
	before := *t
	if enabled {
		t.Lflag |= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	} else {
		t.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	}
	if err := unix.IoctlSetTermios(int(fd), unix.TCSETS, t); err != nil {
		return func() {}
	}
	return func() {
		restore := before
		_ = unix.IoctlSetTermios(int(fd), unix.TCSETS, &restore)
	}
}
