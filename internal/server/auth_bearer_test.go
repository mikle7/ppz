package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// requireBearer behaviour is exercised end-to-end via the e2e suite
// (which talks to a real Postgres). These tests cover the
// non-DB-dependent paths: missing/malformed Authorization headers
// must not even reach the DB.

func TestRequireBearer_MissingHeader_401(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	rec := httptest.NewRecorder()

	srv.requireBearer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run on missing Authorization")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestRequireBearer_NonBearerScheme_401(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	srv.requireBearer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run on non-Bearer scheme")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestRequireBearer_EmptyBearer_401(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	srv.requireBearer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run on empty bearer value")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}

func TestRequireBearer_UnknownPrefix_401(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	// Neither ppz_live_ nor ppz_oauth_ — server must reject before
	// hitting any DB lookup.
	req.Header.Set("Authorization", "Bearer something_random_abcdef")
	rec := httptest.NewRecorder()

	srv.requireBearer(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run on unknown bearer prefix")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", rec.Code)
	}
}
