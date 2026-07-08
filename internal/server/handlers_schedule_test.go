package server

// RED — docs/specs/schedule.md. Schedules are server-side durable
// state, managed over the same bearer-gated REST surface as sources
// and pipes:
//
//	POST   /api/v1/schedules        create (daemon forwards ScheduleCreate)
//	GET    /api/v1/schedules        list for the authenticated org
//	DELETE /api/v1/schedules/{id}   remove by short id
//
// These pins prove the routes are mounted and gated — the pattern of
// TestGUIUncollaredPipePage_RouteMounted. Behaviour (rows persisting,
// the firing loop publishing) is covered by tests/schedule/ e2e.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScheduleRoutes_MountedAndGated(t *testing.T) {
	srv := &Server{}
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/schedules"},
		{http.MethodGet, "/api/v1/schedules"},
		{http.MethodDelete, "/api/v1/schedules/a1b2c3d4"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s: 404 — route not mounted (body=%q)", c.method, c.path, rec.Body.String())
			continue
		}
		if rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s: 405 — path mounted but method missing", c.method, c.path)
			continue
		}
		// No Authorization header ⇒ the bearer gate must fire before
		// any handler logic. 200 here would mean an unauthenticated
		// data-plane route.
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d, want 401 (bearer gate)", c.method, c.path, rec.Code)
		}
	}
}
