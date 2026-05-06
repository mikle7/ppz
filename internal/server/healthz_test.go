package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthzReturnsVersion asserts that GET /healthz responds with JSON
// containing a "version" field. The plain-text "ok" response gives ops no
// way to confirm which binary is actually running after a deploy.
func TestHealthzReturnsVersion(t *testing.T) {
	s := &Server{Version: "v1.2.3-test"}
	mux := s.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v — got: %s", err, w.Body.String())
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
	if body.Version != "v1.2.3-test" {
		t.Errorf("version = %q, want %q", body.Version, "v1.2.3-test")
	}
}
