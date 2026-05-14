package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Phase 2 Cycle B: /orgs → /accounts rename — routes, templates, handler
// names. Sub-decision locked 2026-05-14: all-the-way (routes too, not
// just visible text).
//
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle B.

// renamedRoutes maps the old (post-Phase-1, pre-Phase-2) path to the
// expected new path. Method-aware because some routes have GET-only or
// POST-only handlers, and Go's ServeMux distinguishes by method.
type renamedRoute struct {
	method, oldPath, newPath string
}

func cycleBRenamedRoutes() []renamedRoute {
	return []renamedRoute{
		// API surface
		{"POST", "/api/v1/orgs/x/invites", "/api/v1/accounts/x/invites"},
		{"GET", "/api/v1/orgs/x/invites", "/api/v1/accounts/x/invites"},
		{"POST", "/api/v1/orgs/x/invites/some-id/revoke", "/api/v1/accounts/x/invites/some-id/revoke"},
		// GUI surface
		{"POST", "/orgs", "/accounts"},
		{"GET", "/orgs/some-id", "/accounts/some-id"},
		{"GET", "/orgs/some-id/keys", "/accounts/some-id/keys"},
		{"POST", "/orgs/some-id/keys", "/accounts/some-id/keys"},
		{"POST", "/orgs/some-id/keys/k1/revoke", "/accounts/some-id/keys/k1/revoke"},
		{"POST", "/orgs/some-id/members", "/accounts/some-id/members"},
		{"POST", "/orgs/some-id/members/u1/remove", "/accounts/some-id/members/u1/remove"},
		{"POST", "/orgs/some-id/invites", "/accounts/some-id/invites"},
		{"POST", "/orgs/some-id/invites/i1/revoke", "/accounts/some-id/invites/i1/revoke"},
		{"GET", "/orgs/some-id/sources/h/pipes/p", "/accounts/some-id/sources/h/pipes/p"},
		{"GET", "/orgs/some-id/sources/h/terminal", "/accounts/some-id/sources/h/terminal"},
		{"GET", "/orgs/some-id/sources/h/terminal/ws", "/accounts/some-id/sources/h/terminal/ws"},
	}
}

// TestRoutes_OrgsPathsReturn404 — old `/orgs/...` and `/api/v1/orgs/...`
// shapes must 404 after the rename. ServeMux returns 404 for paths with
// no registered pattern (regardless of method).
func TestRoutes_OrgsPathsReturn404(t *testing.T) {
	s := &Server{Version: "v0-test"}
	mux := s.Routes()

	for _, r := range cycleBRenamedRoutes() {
		t.Run(r.method+" "+r.oldPath, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.oldPath, nil)
			req.Header.Set("Accept", "application/json") // surface 401 as 401, not 302
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: status = %d, want 404 (route should be unmounted after rename)",
					r.method, r.oldPath, w.Code)
			}
		})
	}
}

// TestRoutes_AccountsPathsAreMounted — new `/accounts/...` and
// `/api/v1/accounts/...` shapes must be mounted. Mounted means the
// response is anything other than 404 — typically 401 (auth required)
// or 302 (redirect to login) for session-gated GUI routes, or
// 401/400/403 for the bearer-gated API routes.
func TestRoutes_AccountsPathsAreMounted(t *testing.T) {
	s := &Server{Version: "v0-test"}
	mux := s.Routes()

	for _, r := range cycleBRenamedRoutes() {
		t.Run(r.method+" "+r.newPath, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.newPath, nil)
			req.Header.Set("Accept", "application/json") // 401 instead of 302
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Errorf("%s %s: status = 404, want any-not-404 (route should be mounted after rename)",
					r.method, r.newPath)
			}
		})
	}
}

// TestTemplates_OrgHTMLRemoved — the org.html template file is renamed
// to account.html. Check the embed.FS the templates package exposes (we
// look up by parsing the embedded set).
func TestTemplates_OrgHTMLRemoved(t *testing.T) {
	files, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("templateFS.ReadDir: %v", err)
	}
	for _, f := range files {
		if f.Name() == "org.html" {
			t.Errorf("templates/org.html still present — should be renamed to account.html")
		}
	}
}

func TestTemplates_AccountHTMLPresent(t *testing.T) {
	if _, err := templateFS.ReadFile("templates/account.html"); err != nil {
		t.Errorf("templates/account.html missing: %v", err)
	}
}

// TestTemplates_NoOrganisationStrings — rendered templates and template
// source must not contain "Organisation"/"organisation" visible text.
// Cheap textual scan over the embedded set. Allowed: route paths that
// are *test fixtures* (none) — production templates should be clean.
func TestTemplates_NoOrganisationStrings(t *testing.T) {
	files, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("templateFS.ReadDir: %v", err)
	}
	forbidden := []string{"Organisation", "organisation", "/orgs", "OrganisationName"}
	for _, f := range files {
		data, err := templateFS.ReadFile("templates/" + f.Name())
		if err != nil {
			t.Fatalf("read %s: %v", f.Name(), err)
		}
		s := string(data)
		for _, needle := range forbidden {
			if strings.Contains(s, needle) {
				t.Errorf("templates/%s contains forbidden string %q", f.Name(), needle)
			}
		}
	}
}

// TestServer_RenamedHandlerMethods — compile-time pin for handler-name
// rename. The closures below reference the new names; if the methods
// don't exist on *Server, the test file fails to build. Phase 1.5's
// manifold_test.go uses the same pattern (see InsertSource compile-pin).
func TestServer_RenamedHandlerMethods(t *testing.T) {
	var s *Server
	if s == nil {
		return
	}
	// Compile-time references; these closures never run.
	_ = s.handleGUICreateAccount
	_ = s.handleGUIAccountRedirect
	_ = s.handleGUIAccountTab
	_ = s.handleAPIListInvitesForAccount
}
