package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
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
// middleware. RED today (route doesn't exist) → 404.
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
