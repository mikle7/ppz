//go:build linux || darwin

package cli

// Reproduces the user-observed bug: when running `ppz terminal view` from
// an interactive terminal, locally-typed keystrokes get echoed by the
// local tty's line discipline. The fix is to put the local tty into
// raw mode (no echo, no canonical) for the lifetime of the viewer; this
// test asserts the raw-mode helper does exactly that on a real pty.
//
// Why a Go unit test rather than a bash scenario: our docker test runner
// runs with stdin = /dev/null so `term.IsTerminal(stdin)` returns false
// and any tty-only branch is dead code under the harness. Opening a
// creack/pty pair here gives us a real tty fd to assert against.

import (
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestSetLocalRawMode_ClearsEchoOnTerminal(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	defer master.Close()
	defer slave.Close()

	// Sanity check: a fresh pty's slave starts with ECHO on (cooked mode).
	before, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD before: %v", err)
	}
	if before.Lflag&unix.ECHO == 0 {
		t.Fatalf("precondition failed: expected ECHO on, got Lflag=%#x", before.Lflag)
	}

	restore := setLocalRawMode(slave.Fd())
	if restore == nil {
		t.Fatal("setLocalRawMode returned nil restore func")
	}

	during, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD during: %v", err)
	}
	if during.Lflag&unix.ECHO != 0 {
		t.Errorf("expected ECHO cleared after setLocalRawMode, got Lflag=%#x", during.Lflag)
	}

	restore()

	after, err := termiosForFD(int(slave.Fd()))
	if err != nil {
		t.Fatalf("termiosForFD after: %v", err)
	}
	if after.Lflag&unix.ECHO == 0 {
		t.Errorf("expected ECHO restored after restore(), got Lflag=%#x", after.Lflag)
	}
}

func TestSetLocalRawMode_NoOpOnNonTerminal(t *testing.T) {
	// A pipe is not a tty — setLocalRawMode should be a safe no-op (return
	// a non-nil restore function that does nothing) so callers don't have
	// to special-case non-tty stdin.
	r, w, err := pipePair()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	restore := setLocalRawMode(r.Fd())
	if restore == nil {
		t.Fatal("setLocalRawMode returned nil restore for non-tty (callers expect always-callable)")
	}
	// Calling the no-op restore should not panic.
	restore()
}
