//go:build darwin

package cli

import "golang.org/x/sys/unix"

// Per-OS termios ioctl request constants — see the matching linux file.
const (
	ttyGetTermios = unix.TIOCGETA
	ttySetTermios = unix.TIOCSETA
)

// disablePTYEcho clears the line discipline's ECHO bits on the PTY so
// input bytes (user keystrokes + local-terminal replies to escape
// queries) don't reflect back to master read and end up republished
// to .stdout / .raw. Other Lflag bits — ICANON, ISIG — are left alone
// so cooked-mode line buffering and Ctrl-C-style signals still work.
//
// On Darwin the line discipline is shared between master and slave,
// so writing termios on the master fd applies to both.
func disablePTYEcho(fd uintptr) {
	t, err := unix.IoctlGetTermios(int(fd), unix.TIOCGETA)
	if err != nil {
		return
	}
	t.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	_ = unix.IoctlSetTermios(int(fd), unix.TIOCSETA, t)
}
