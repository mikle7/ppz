package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/db"
)

// Phase 1.5 Cycle B: POST /api/v1/pipes is the full-path-aware pipe
// creation endpoint. Body takes explicit manifold + nullable
// source_handle so the server doesn't need to introspect a dotted
// path string — the client sends an unambiguous request derived
// from daemon state.
//
// This is parallel to (not a replacement for) the pre-Phase-1.5
// POST /api/v1/sources/{handle}/pipes which stays as the
// collared-shortcut.

// TestPipesAPI_NoAuth_401 — when the route is mounted, a missing
// Authorization header surfaces as 401 from the requireAPIKey
// middleware. Pins the route-mount + auth-gate after Phase 1.5
// landed the new POST /api/v1/pipes endpoint.
func TestPipesAPI_NoAuth_401(t *testing.T) {
	srv := &Server{}
	body := bytes.NewReader([]byte(`{"manifold":"","name":"room"}`))
	req := httptest.NewRequest("POST", "/api/v1/pipes", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401 (no Authorization header)", rec.Code)
	}
}

// TestPipesAPI_RejectsCollaredOnUncollaredEndpoint — POST /api/v1/pipes
// is the sourceless endpoint; collared requests (Handle or non-nil
// SourceHandle set) must be redirected to the collared-shortcut path
// rather than silently routed through. Validates the guard from
// handlers_api.go before any DB call so this can be tested without
// a Postgres pool.
func TestPipesAPI_RejectsCollaredOnUncollaredEndpoint(t *testing.T) {
	srv := &Server{}
	key := db.APIKey{
		AccountID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CreatedByUserID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	}
	body := []byte(`{"handle":"cindy","name":"inbox"}`)
	req := httptest.NewRequest("POST", "/api/v1/pipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleCreatePipeFullPath(rec, req, key)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 (collared request on uncollared endpoint)", rec.Code)
	}
	if got := rec.Body.String(); !bytes.Contains([]byte(got), []byte("E_INVALID_PIPE")) {
		t.Errorf("body: want E_INVALID_PIPE in response, got %q", got)
	}
}

// TestPipesAPI_RejectsInvalidManifoldSegment — manifold validation
// rejects each dot-separated segment via the handle regex. Asserts the
// new E_INVALID_MANIFOLD code (registered in cliproto/errors.go) flows
// through with HTTP 400, not the previous fall-through to HTTP 500.
func TestPipesAPI_RejectsInvalidManifoldSegment(t *testing.T) {
	srv := &Server{}
	key := db.APIKey{
		AccountID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CreatedByUserID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	}
	// "Team_A" violates the handle regex (uppercase + underscore).
	body := []byte(`{"manifold":"Team_A","name":"room"}`)
	req := httptest.NewRequest("POST", "/api/v1/pipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleCreatePipeFullPath(rec, req, key)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 (invalid manifold)", rec.Code)
	}
	if got := rec.Body.String(); !bytes.Contains([]byte(got), []byte("E_INVALID_MANIFOLD")) {
		t.Errorf("body: want E_INVALID_MANIFOLD in response, got %q", got)
	}
}
