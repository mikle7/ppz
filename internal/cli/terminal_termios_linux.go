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

// disablePTYEcho clears the line discipline's ECHO bits on the PTY so
// input bytes (user keystrokes + local-terminal replies to escape
// queries) don't reflect back to master read and end up republished
// to .stdout / .raw. Other Lflag bits — ICANON, ISIG — are left alone
// so cooked-mode line buffering and Ctrl-C-style signals still work.
//
// Setting termios on the master fd works on Linux because the line
// discipline is shared between master and slave.
func disablePTYEcho(fd uintptr) {
	t, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	if err != nil {
		return
	}
	t.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ECHOCTL | unix.ECHOKE
	_ = unix.IoctlSetTermios(int(fd), unix.TCSETS, t)
}
