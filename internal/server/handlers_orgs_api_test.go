package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Auth + validation paths for the new /api/v1/orgs endpoints.
// Behavioural coverage (creation persists, listing returns the user's
// orgs) lives in tests/org/* e2e scenarios.

func TestOrgsAPI_NoAuth_401(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		body         string
	}{
		{"GET", "/api/v1/orgs", ""},
		{"POST", "/api/v1/orgs", `{"name":"foo"}`},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			srv := &Server{}
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status: got %d want 401", rec.Code)
			}
		})
	}
}

func TestAPICreateOrg_APIKeyCaller_403(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", bytes.NewReader([]byte(`{"name":"foo"}`)))
	req.Header.Set("Content-Type", "application/json")
	// no AuthedCaller in context → UserID == uuid.Nil → 403
	rec := httptest.NewRecorder()
	srv.handleAPICreateOrg(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("API-key caller (no UserID) should 403; got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPICreateOrg_MissingName_400(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", bytes.NewReader([]byte(`{"name":""}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withAuthedCaller(req.Context(), uuidNonZero()))
	rec := httptest.NewRecorder()
	srv.handleAPICreateOrg(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty name should 400; got %d", rec.Code)
	}
}

func TestAPICreateOrg_InvalidSlug_400(t *testing.T) {
	for _, name := range []string{
		"Foo",          // uppercase
		"foo bar",      // space
		"foo_bar",      // underscore
		"-foo",         // leading dash
		"foo-",         // trailing dash
		"foo.bar",      // dot
		"a-very-long-name-that-exceeds-forty-chars-limit",
	} {
		t.Run(name, func(t *testing.T) {
			srv := &Server{}
			body := []byte(`{"name":"` + name + `"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(withAuthedCaller(req.Context(), uuidNonZero()))
			rec := httptest.NewRecorder()
			srv.handleAPICreateOrg(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("invalid slug %q should 400; got %d body=%s", name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestValidOrgSlug_AcceptsValid(t *testing.T) {
	for _, name := range []string{"a", "foo", "foo-bar", "foo123", "abc-def-ghi", "x9"} {
		if !validOrgSlug(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
}

func TestValidOrgSlug_RejectsInvalid(t *testing.T) {
	for _, name := range []string{"", "Foo", "foo_bar", "-foo", "foo-", "foo.bar", "foo bar"} {
		if validOrgSlug(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}
