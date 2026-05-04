//go:build linux || darwin

package cli

import (
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestSetPTYInputEcho_TogglesEchoViaMasterAndRestores(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	before, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD before: %v", err)
	}
	if before.Lflag&unix.ECHO == 0 {
		t.Fatalf("precondition failed: expected ECHO on, got Lflag=%#x", before.Lflag)
	}

	restore := setPTYInputEcho(master.Fd(), false)
	if restore == nil {
		t.Fatal("setPTYInputEcho returned nil restore func")
	}

	during, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD during: %v", err)
	}
	if during.Lflag&unix.ECHO != 0 {
		t.Fatalf("expected ECHO cleared via master fd, got Lflag=%#x", during.Lflag)
	}

	restore()

	after, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD after: %v", err)
	}
	if after.Lflag&unix.ECHO == 0 {
		t.Fatalf("expected ECHO restored after restore(), got Lflag=%#x", after.Lflag)
	}
}
