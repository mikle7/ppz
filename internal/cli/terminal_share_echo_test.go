//go:build linux || darwin

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestCmdTerminalShareEchoesInteractiveInputBeforeEnter(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	dir, err := os.MkdirTemp("/tmp", "ppz-terminal-share-echo-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)
	serveTerminalShareEchoDaemon(t, sock)

	localMaster, localSlave, err := pty.Open()
	if err != nil {
		t.Fatalf("local pty.Open: %v", err)
	}
	defer localMaster.Close()
	defer localSlave.Close()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer stdoutReader.Close()
	defer stdoutWriter.Close()

	oldStdin, oldStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = localSlave, stdoutWriter
	t.Cleanup(func() {
		os.Stdin, os.Stdout = oldStdin, oldStdout
	})

	stdout := newAsyncBuffer(t, stdoutReader)

	done := make(chan error, 1)
	go func() {
		done <- cmdTerminalShare([]string{"term-echo", "--", bash, "--noprofile", "--norc", "-i"})
	}()

	if !stdout.waitContains("bash", 2*time.Second) {
		t.Fatalf("interactive shell did not start; stdout=%q", stdout.String())
	}

	const typed = "echo PPZ_TERMINAL_SHARE_ECHO"
	if _, err := io.WriteString(localMaster, typed); err != nil {
		t.Fatalf("write typed input: %v", err)
	}

	if !stdout.waitContains(typed, time.Second) {
		_, _ = io.WriteString(localMaster, "\nexit\n")
		t.Fatalf("typed input was not echoed before Enter; stdout=%q", stdout.String())
	}

	if _, err := io.WriteString(localMaster, "\nexit\n"); err != nil {
		t.Fatalf("write exit: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cmdTerminalShare: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cmdTerminalShare did not exit after shell exit")
	}
}

type asyncBuffer struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  bytes.Buffer
}

func newAsyncBuffer(t *testing.T, r *os.File) *asyncBuffer {
	t.Helper()
	b := &asyncBuffer{}
	b.cond = sync.NewCond(&b.mu)
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	go func() {
		defer close(done)
		_, _ = io.Copy(b, r)
	}()
	return b
}

func (b *asyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	b.cond.Broadcast()
	return n, err
}

func (b *asyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *asyncBuffer) waitContains(substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	b.mu.Lock()
	defer b.mu.Unlock()
	for !strings.Contains(b.buf.String(), substr) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timer := time.AfterFunc(remaining, func() {
			b.mu.Lock()
			b.cond.Broadcast()
			b.mu.Unlock()
		})
		b.cond.Wait()
		timer.Stop()
	}
	return true
}

func serveTerminalShareEchoDaemon(t *testing.T, sock string) {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleTerminalShareEchoDaemonConn(conn)
		}
	}()
}

func handleTerminalShareEchoDaemonConn(conn net.Conn) {
	defer conn.Close()
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	enc := json.NewEncoder(conn)
	switch req.Method {
	case cliproto.IPCCreate:
		_ = enc.Encode(map[string]any{
			"result": cliproto.CreateReply{
				Handle:  "term-echo",
				Subject: "test.term-echo",
				Pipes:   []string{"broadcast", "stdin", "stdout", "stdctrl", "inbox"},
			},
		})
	case cliproto.IPCSend:
		var br cliproto.SendRequest
		_ = json.Unmarshal(req.Params, &br)
		_ = enc.Encode(map[string]any{
			"result": cliproto.SendReply{
				ID:      "test-id",
				Subject: "test." + br.Handle + "." + br.Channel,
				Bytes:   len(br.Payload),
			},
		})
	case cliproto.IPCRead:
		_, _ = io.Copy(io.Discard, conn)
	}
}
