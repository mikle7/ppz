package cli

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// `ppz ls clancy%` (pattern argument, no --watch) should filter the
// snapshot table to matching pipes rather than rejecting the args.
// Without this, the natural `ls` invocation errors out and the user
// has to either add --watch (changes semantics from "snapshot now" to
// "block until unread arrives") or pipe through grep (loses column
// alignment and the daemon's payload truncation).
//
// Contract: cmdLs forwards positional args as ListRequest.Patterns
// over IPCList. The daemon side already has buildFilteredList wired
// for ListWatchRequest; this opens the door from plain ls.
func TestCmdLs_PatternsForwardToList(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-ls-pat-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveLsListDaemon(t, sock)

	if err := cmdLs([]string{"clancy%"}); err != nil {
		t.Fatalf("cmdLs with pattern arg: %v (want filtered list, not error)", err)
	}

	if requests.count() != 1 {
		t.Fatalf("IPCList request count = %d, want 1", requests.count())
	}
	got := requests.at(0)
	if len(got.Patterns) != 1 || got.Patterns[0] != "clancy%" {
		t.Fatalf("ListRequest.Patterns = %v, want [clancy%%]", got.Patterns)
	}
}

// Bare `ppz ls` (no args) must keep working unchanged — every other
// IPCList caller (source.go, pipe.go, completion.go)
// passes no patterns and expects the full snapshot.
func TestCmdLs_NoArgsSendsEmptyPatterns(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-ls-nopat-")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	t.Setenv("PPZ_IPC_SOCKET", sock)

	requests := serveLsListDaemon(t, sock)

	if err := cmdLs(nil); err != nil {
		t.Fatalf("cmdLs no args: %v", err)
	}
	if requests.count() != 1 {
		t.Fatalf("IPCList request count = %d, want 1", requests.count())
	}
	if len(requests.at(0).Patterns) != 0 {
		t.Fatalf("ListRequest.Patterns = %v, want empty", requests.at(0).Patterns)
	}
}

func serveLsListDaemon(t *testing.T, sock string) *recorder[cliproto.ListRequest] {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	listRequests := &recorder[cliproto.ListRequest]{}
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
			var req struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = conn.Close()
				continue
			}
			if req.Method == cliproto.IPCList {
				var lr cliproto.ListRequest
				_ = json.Unmarshal(req.Params, &lr)
				listRequests.add(lr)
				_ = json.NewEncoder(conn).Encode(map[string]any{
					"result": cliproto.ListReply{},
				})
			}
			_ = conn.Close()
		}
	}()

	return listRequests
}
