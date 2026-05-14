package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pipescloud/ppz/internal/auth"
)

// Phase 2 Cycle D: three-mode /login.
//
// /login dispatches by Server.AuthMode (populated in Cycle C):
//
//   AuthModeNone     — auto-completes the session, renders an upgrade-
//                      path informational panel.
//   AuthModePassword — renders username/password form (GET); validates
//                      against users.password_hash on POST (POST flow
//                      lands in Cycle F when the Users page exists).
//   AuthModeOAuth    — redirects to the configured Provider's Authorize
//                      URL. OSS ships a stub provider; pipescloud
//                      implements out-of-tree.
//
// All three terminate in the same downstream contract: a user_id
// session cookie. See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md.

// TestLogin_ModeNone_RendersUpgradePanel — GET /login under mode=none
// returns 200 with the documented upgrade-path text + a "Continue to
// dashboard" link. Session-cookie auto-set is asserted separately
// in the integration suite (requires DB).
func TestLogin_ModeNone_RendersUpgradePanel(t *testing.T) {
	s := &Server{AuthMode: AuthModeNone, Version: "test"}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, needle := range []string{
		"PPZ_SERVER_AUTH_MODE",
		"password",
		"Continue to dashboard",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("body missing %q\n--- body ---\n%s", needle, body)
		}
	}
}

// TestLogin_ModePassword_RendersForm — GET /login under mode=password
// returns 200 with a username + password form posting back to /login.
func TestLogin_ModePassword_RendersForm(t *testing.T) {
	s := &Server{AuthMode: AuthModePassword, Version: "test"}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, needle := range []string{
		`name="username"`,
		`name="password"`,
		`type="password"`,
		`method="post"`,
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("form body missing %q\n--- body ---\n%s", needle, body)
		}
	}
}

// TestLogin_ModeOAuth_RedirectsToProvider — GET /login under mode=oauth
// 302s to the Provider's Authorize URL. The stub provider returns its
// configured-error URL; pipescloud's provider returns a real upstream
// authorize endpoint.
func TestLogin_ModeOAuth_RedirectsToProvider(t *testing.T) {
	stub := &auth.StubProvider{}
	s := &Server{AuthMode: AuthModeOAuth, Provider: stub, Version: "test"}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 302 or 500 (stub provider may surface a configured-error response); body=%s",
			w.Code, w.Body.String())
	}
}

// TestProvider_InterfaceContract — compile-time pin on the Provider
// interface methods. Cycle D Phase 4 (pipescloud) implements this.
func TestProvider_InterfaceContract(t *testing.T) {
	var _ auth.Provider = (*auth.StubProvider)(nil)
}

// TestStubProvider_AuthorizeReturnsConfiguredError — the OSS-shipped
// stub responds with a 500 + "provider not configured" message. This
// is the no-pipescloud-installed deployment shape under mode=oauth.
func TestStubProvider_AuthorizeReturnsConfiguredError(t *testing.T) {
	stub := &auth.StubProvider{}
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	stub.Authorize(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "provider") {
		t.Errorf("body should mention provider; got %q", w.Body.String())
	}
}
