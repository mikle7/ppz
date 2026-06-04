package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// callComplete runs handleComplete over an in-memory net.Pipe and
// returns the decoded CompleteReply. Mirrors the callSourceDestroy
// helper in source_destroy_test.go.
func callComplete(t *testing.T, d *Daemon, req cliproto.CompleteRequest) (cliproto.CompleteReply, *cliproto.Error) {
	t.Helper()
	params, _ := json.Marshal(req)
	srvConn, cliConn := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer srvConn.Close()
		d.handleComplete(context.Background(), srvConn, params)
		close(done)
	}()

	var resp ipcResponse
	if err := json.NewDecoder(cliConn).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cliConn.Close()
	<-done

	if resp.Error != nil {
		return cliproto.CompleteReply{}, resp.Error
	}
	var reply cliproto.CompleteReply
	raw, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(raw, &reply)
	return reply, nil
}

// TestHandleComplete_ServesFromCache: when refreshSourceCache has
// already run, handleComplete returns the cached snapshot without
// hitting the server. This is the steady-state hot path — every tab
// press should land here.
func TestHandleComplete_ServesFromCache(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sources", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(cliproto.ListSourcesReply{})
	})
	d := newDaemonWithFakeServer(t, mux)

	// Pre-warm the cache the way a real `ppz ls` would.
	d.refreshSourceCache([]cliproto.Source{
		{Handle: "alice", Kind: string(cliproto.KindPTY), Pipes: []string{"alerts"}},
		{Handle: "bob", Kind: string(cliproto.KindMessage)},
	})

	reply, ipcErr := callComplete(t, d, cliproto.CompleteRequest{})
	if ipcErr != nil {
		t.Fatalf("handleComplete: %v", ipcErr)
	}
	if reply.Stale {
		t.Fatalf("reply.Stale = true; cache was pre-warmed so should be fresh")
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("server hit %d times; handleComplete must not call /api/v1/sources when cache is warm", hits)
	}

	got := map[string][]string{}
	for _, s := range reply.Sources {
		sort.Strings(s.Pipes)
		got[s.Handle] = s.Pipes
	}
	// alice (pty) merges pipesForKind(pty) ∪ user pipes; bob (message)
	// gets just the message default.
	wantAlice := []string{"alerts", "heartbeat", "inbox", "stdctrl", "stdin", "stdout"}
	wantBob := []string{"inbox"}
	if !equalStrings(got["alice"], wantAlice) {
		t.Errorf("alice pipes = %v, want %v", got["alice"], wantAlice)
	}
	if !equalStrings(got["bob"], wantBob) {
		t.Errorf("bob pipes = %v, want %v", got["bob"], wantBob)
	}
}

// TestHandleComplete_ColdCacheWarmsOnce: on a cold daemon the first
// completion call does a SINGLE cheap GET /api/v1/sources to warm the
// cache. JetStream / /api/v1/pipes must NOT be touched — that's the
// whole reason this verb exists.
func TestHandleComplete_ColdCacheWarmsOnce(t *testing.T) {
	var sourcesHits, pipesHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sources", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&sourcesHits, 1)
		_ = json.NewEncoder(w).Encode(cliproto.ListSourcesReply{
			Sources: []cliproto.Source{
				{Handle: "carol", Kind: string(cliproto.KindMessage)},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/pipes", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&pipesHits, 1)
		_ = json.NewEncoder(w).Encode(cliproto.ListUncollaredPipesReply{})
	})
	d := newDaemonWithFakeServer(t, mux)

	reply, ipcErr := callComplete(t, d, cliproto.CompleteRequest{})
	if ipcErr != nil {
		t.Fatalf("first handleComplete: %v", ipcErr)
	}
	if len(reply.Sources) != 1 || reply.Sources[0].Handle != "carol" {
		t.Errorf("cold-cache reply = %+v, want carol", reply.Sources)
	}
	if got := atomic.LoadInt32(&sourcesHits); got != 1 {
		t.Errorf("first call: /api/v1/sources hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&pipesHits); got != 0 {
		t.Errorf("first call: /api/v1/pipes hits = %d, want 0 — completion must not enrich uncollared", got)
	}

	// Second call — cache is now warm; no additional server hit.
	if _, ipcErr := callComplete(t, d, cliproto.CompleteRequest{}); ipcErr != nil {
		t.Fatalf("second handleComplete: %v", ipcErr)
	}
	if got := atomic.LoadInt32(&sourcesHits); got != 1 {
		t.Errorf("second call: /api/v1/sources hits = %d, want still 1 (warm cache)", got)
	}
}

// TestHandleComplete_LoggedOutReturnsStale: a daemon without credentials
// returns Stale with no sources and no error. Tab completion must never
// surface an authentication failure mid-keystroke.
func TestHandleComplete_LoggedOutReturnsStale(t *testing.T) {
	home := t.TempDir()
	d := New(home, "") // no SetLogin → no credentials

	reply, ipcErr := callComplete(t, d, cliproto.CompleteRequest{})
	if ipcErr != nil {
		t.Fatalf("handleComplete on logged-out daemon: %v (want clean empty reply)", ipcErr)
	}
	if !reply.Stale {
		t.Errorf("reply.Stale = false; want true on logged-out daemon")
	}
	if len(reply.Sources) != 0 {
		t.Errorf("reply.Sources = %v, want empty on logged-out daemon", reply.Sources)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
