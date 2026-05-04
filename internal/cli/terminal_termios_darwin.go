//go:build darwin

package cli

import "golang.org/x/sys/unix"

// Per-OS termios ioctl request constants — see the matching linux file.
const (
	ttyGetTermios = unix.TIOCGETA
	ttySetTermios = unix.TIOCSETA
)

// setPTYInputEcho toggles the PTY line discipline's ECHO bits and returns a
// restore function for the previous termios state.
//
// On Darwin the line discipline is shared between master and slave,
// so writing termios on the master fd applies to both.
func setPTYInputEcho(fd uintptr, enabled bool) func() {
	t, err := unix.IoctlGetTermios(int(fd), unix.TIOCGETA)
	if err != nil {
		return func() {}
	}
	before := *t
	if enabled {
		t.Lflag |= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	} else {
		t.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	}
	if err := unix.IoctlSetTermios(int(fd), unix.TIOCSETA, t); err != nil {
		return func() {}
	}
	return func() {
		restore := before
		_ = unix.IoctlSetTermios(int(fd), unix.TIOCSETA, &restore)
	}
}
