//go:build linux || darwin

package cli

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestForwardStdinWritesPayloadVerbatim(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-forward-stdin-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"no newline", "hello"},
		{"escape sequence", "\x1b[13u"},
		{"already has newline", "hello\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(sock)
			ln, err := net.Listen("unix", sock)
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			t.Cleanup(func() { _ = ln.Close() })

			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				var req struct {
					Method string `json:"method"`
				}
				_ = json.NewDecoder(conn).Decode(&req)
				_ = json.NewEncoder(conn).Encode(cliproto.ReadEvent{
					Message: &cliproto.ReadMessage{Payload: tc.payload},
				})
			}()

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			go forwardStdin(ctx, "myhost", w)

			buf := make([]byte, 64)
			_ = r.SetReadDeadline(time.Now().Add(time.Second))
			n, _ := r.Read(buf)
			got := string(buf[:n])

			if got != tc.payload {
				t.Errorf("forwardStdin wrote %q, want %q (verbatim — no appended newline)", got, tc.payload)
			}
		})
	}
}

// TestForwardStdinRequestsSinceMS guards the retained-stdin-replay fix: a
// fresh forwardStdin call must ask the daemon for a SinceMS cutoff so a
// brand-new process (empty seenIDRing) doesn't get the full retained
// backlog replayed into its wrapped child on connect.
func TestForwardStdinRequestsSinceMS(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-forward-stdin-sincems-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	reqCh := make(chan cliproto.ReadRequest, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req struct {
			Params cliproto.ReadRequest `json:"params"`
		}
		_ = json.NewDecoder(conn).Decode(&req)
		reqCh <- req.Params
		// No response — the test only cares what was asked for.
	}()

	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer w.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go forwardStdin(ctx, "myhost", w)

	select {
	case req := <-reqCh:
		if req.SinceMS <= 0 {
			t.Errorf("forwardStdin sent SinceMS=%d, want >0 (a fresh process must filter retained backlog, not replay it)", req.SinceMS)
		}
		if !req.NoAdvance || !req.Follow {
			t.Errorf("forwardStdin request changed shape: NoAdvance=%v Follow=%v, want both true", req.NoAdvance, req.Follow)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon never received a read request")
	}
}
