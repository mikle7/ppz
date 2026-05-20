package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Phase 1.5 follow-up: viewing an uncollared (sourceless) pipe in the
// GUI.
//
// The org page lists uncollared pipes with PipeLink
// `/orgs/<slug>/pipes/<dotted-path>` (handlers_gui.go), but no route was
// ever mounted for that shape — only the collared
// `/orgs/{id}/sources/{handle}/pipes/{pipe}` exists. Clicking the link
// for e.g. `testroom` at the root manifold hits Go's default ServeMux
// 404 ("page not found") instead of rendering the pipe page.
//
// This test pins the route's existence: hitting it without a session
// cookie should NOT 404. The session gate then either redirects to
// /login (browser) or returns 401 (json client). Either is fine — the
// signal we care about is "the route resolves and the session
// middleware ran," which proves the pattern is mounted.
func TestGUIUncollaredPipePage_RouteMounted(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/orgs/jamesmiles/pipes/testroom", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("GET /orgs/{id}/pipes/{pipe} returned 404 — uncollared pipe route not mounted (body=%q)", rec.Body.String())
	}
	// Defensive — the route must be session-gated like its collared
	// sibling. A 200 here would mean we accidentally exposed the page
	// without auth.
	if rec.Code != http.StatusFound && rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 302 (session redirect) or 401 — route exists but session gate didn't fire", rec.Code)
	}
}
