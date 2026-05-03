package cli

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestPublishAndDisplayStdoutDisplayDoesNotWaitForPublish(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-stdout-publish-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	releasePublish, published := serveBlockingStdoutPublishDaemon(t, sock)
	display := newRecordingWriter()
	done := make(chan struct{})

	go func() {
		defer close(done)
		publishAndDisplayStdout("term", &chunkReader{chunks: [][]byte{[]byte("first"), []byte("second")}}, display)
	}()

	if !display.waitContains("first", time.Second) {
		t.Fatalf("display never received first chunk; got %q", display.String())
	}
	if !display.waitContains("firstsecond", 200*time.Millisecond) {
		t.Fatalf("display waited for blocked publish before rendering second chunk; got %q", display.String())
	}

	close(releasePublish)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishAndDisplayStdout did not finish after publish was released")
	}

	if got := strings.Join(published.snapshot(), "|"); got != "first|second" {
		t.Fatalf("published payloads = %q, want first|second", got)
	}
}

type chunkReader struct {
	chunks [][]byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]
	return copy(p, chunk), nil
}

type recordingWriter struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  strings.Builder
}

func newRecordingWriter() *recordingWriter {
	w := &recordingWriter{}
	w.cond = sync.NewCond(&w.mu)
	return w
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	w.cond.Broadcast()
	return n, err
}

func (w *recordingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *recordingWriter) waitContains(substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	w.mu.Lock()
	defer w.mu.Unlock()
	for !strings.Contains(w.buf.String(), substr) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timer := time.AfterFunc(remaining, func() {
			w.mu.Lock()
			w.cond.Broadcast()
			w.mu.Unlock()
		})
		w.cond.Wait()
		timer.Stop()
	}
	return true
}

type publishedPayloads struct {
	mu       sync.Mutex
	payloads []string
}

func (p *publishedPayloads) append(payload string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.payloads = append(p.payloads, payload)
}

func (p *publishedPayloads) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.payloads...)
}

func serveBlockingStdoutPublishDaemon(t *testing.T, sock string) (chan struct{}, *publishedPayloads) {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	release := make(chan struct{})
	published := &publishedPayloads{}
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
			go func() {
				defer conn.Close()
				var req struct {
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(conn).Decode(&req); err != nil {
					return
				}
				if req.Method != cliproto.IPCBroadcast {
					return
				}
				var br cliproto.BroadcastRequest
				if err := json.Unmarshal(req.Params, &br); err != nil {
					return
				}
				published.append(br.Payload)
				if br.Payload == "first" {
					<-release
				}
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.BroadcastReply{ID: "id", Subject: "org.term.stdout", Bytes: len(br.Payload)},
				})
			}()
		}
	}()

	return release, published
}
