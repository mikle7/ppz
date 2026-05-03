//go:build linux || darwin

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// termiosForFD returns the current termios on the given fd, using the
// platform's get ioctl request (TCGETS on Linux, TIOCGETA on Darwin).
// Defined in this build-tagged file because the request constant differs
// per OS.
func termiosForFD(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, ttyGetTermios)
}

func pipePair() (*os.File, *os.File, error) {
	return os.Pipe()
}
