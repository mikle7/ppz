package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// fakeServerError writes a cliproto.HTTPError JSON body with the given status code,
// matching the shape that callServer decodes in handlers.go.
func fakeServerError(w http.ResponseWriter, status int, e *cliproto.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(cliproto.HTTPError{Error: *e})
}

// callSourceDestroy runs handleSourceDestroy over an in-memory net.Pipe and
// returns the decoded response.
func callSourceDestroy(t *testing.T, d *Daemon, req cliproto.SourceDestroyRequest) (cliproto.SourceDestroyReply, *cliproto.Error) {
	t.Helper()
	params, _ := json.Marshal(req)
	srvConn, cliConn := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer srvConn.Close()
		d.handleSourceDestroy(context.Background(), srvConn, params)
		close(done)
	}()

	var resp ipcResponse
	if err := json.NewDecoder(cliConn).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cliConn.Close()
	<-done

	if resp.Error != nil {
		return cliproto.SourceDestroyReply{}, resp.Error
	}
	var reply cliproto.SourceDestroyReply
	raw, _ := json.Marshal(resp.Result)
	_ = json.Unmarshal(raw, &reply)
	return reply, nil
}

func newDaemonWithFakeServer(t *testing.T, mux *http.ServeMux) *Daemon {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	home := t.TempDir()
	d := New(home, "")
	if err := d.State.SetLogin(Credentials{
		URL:    srv.URL,
		APIKey: "test-key",
	}, "test-org-id", "Test Org", "test-key"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}
	return d
}

func TestHandleSourceDestroy_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/sources/apple", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	d := newDaemonWithFakeServer(t, mux)

	reply, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "apple"})
	if ipcErr != nil {
		t.Fatalf("unexpected error: %v", ipcErr)
	}
	if reply.Handle != "apple" {
		t.Errorf("got handle %q, want %q", reply.Handle, "apple")
	}
}

func TestHandleSourceDestroy_ClearsSessionCurrentForDestroyedHandle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/sources/apple", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	d := newDaemonWithFakeServer(t, mux)

	// Two sessions both pointing at "apple".
	if err := d.State.SetCurrent("sess-a", "apple"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if err := d.State.SetCurrent("sess-b", "apple"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}

	_, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "apple"})
	if ipcErr != nil {
		t.Fatalf("unexpected error: %v", ipcErr)
	}

	if got := d.State.Current("sess-a"); got != "" {
		t.Errorf("sess-a current should be cleared, got %q", got)
	}
	if got := d.State.Current("sess-b"); got != "" {
		t.Errorf("sess-b current should be cleared, got %q", got)
	}
}

func TestHandleSourceDestroy_RemovesFromKnownPipes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/sources/apple", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	d := newDaemonWithFakeServer(t, mux)
	d.State.RememberPipe("apple")

	_, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "apple"})
	if ipcErr != nil {
		t.Fatalf("unexpected error: %v", ipcErr)
	}

	if d.State.KnowsPipe("apple") {
		t.Error("apple should be removed from knownPipes after destroy")
	}
}

func TestHandleSourceDestroy_SourceNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/sources/ghost", func(w http.ResponseWriter, r *http.Request) {
		fakeServerError(w, http.StatusNotFound, cliproto.NewSourceNotFound("ghost"))
	})
	d := newDaemonWithFakeServer(t, mux)

	_, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "ghost"})
	if ipcErr == nil {
		t.Fatal("expected error for unknown source, got nil")
	}
	if ipcErr.Code != cliproto.ESourceNotFound {
		t.Errorf("got error code %q, want %q", ipcErr.Code, cliproto.ESourceNotFound)
	}
}

func TestHandleSourceDestroy_NotLoggedIn(t *testing.T) {
	home := t.TempDir()
	d := New(home, "")
	// No SetLogin — daemon has no credentials.

	_, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "apple"})
	if ipcErr == nil {
		t.Fatal("expected E_NOT_LOGGED_IN, got nil")
	}
	if ipcErr.Code != cliproto.ENotLoggedIn {
		t.Errorf("got error code %q, want %q", ipcErr.Code, cliproto.ENotLoggedIn)
	}
}

func TestHandleSourceDestroy_InvalidHandle(t *testing.T) {
	home := t.TempDir()
	d := New(home, "")
	if err := d.State.SetLogin(Credentials{URL: "http://x", APIKey: "k"}, "org", "Org", "k"); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}

	_, ipcErr := callSourceDestroy(t, d, cliproto.SourceDestroyRequest{Handle: "INVALID HANDLE!"})
	if ipcErr == nil {
		t.Fatal("expected error for invalid handle, got nil")
	}
	if ipcErr.Code != cliproto.EInvalidHandle {
		t.Errorf("got error code %q, want %q", ipcErr.Code, cliproto.EInvalidHandle)
	}
}
