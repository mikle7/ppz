package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Phase 2 Cycle G: API keys page in admin GUI.
//
// The existing keys functionality is on the account page (Keys tab)
// after Cycle B's rename. This cycle confirms the post-rename surface
// and locks the one-time-reveal pattern.
//
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle G.

// TestAPIKeys_GUIRoutesMounted — every route the Keys admin surface
// uses must be registered. mux.Handler() returns a non-empty pattern
// for any registered route.
func TestAPIKeys_GUIRoutesMounted(t *testing.T) {
	s := &Server{Version: "v0-test"}
	mux := s.Routes()
	for _, tc := range []struct{ method, path string }{
		{"GET", "/accounts/some-id/keys"},
		{"POST", "/accounts/some-id/keys"},
		{"POST", "/accounts/some-id/keys/key-id/revoke"},
		{"POST", "/api/v1/keys/key-id/revoke"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			_, pattern := mux.Handler(req)
			if pattern == "" {
				t.Errorf("%s %s: no matched pattern; route must be mounted", tc.method, tc.path)
			}
		})
	}
}

// TestAPIKeys_KeyCreatedTemplateUsesAccountName — the one-time
// reveal template references .AccountName (matching the handler's
// data map) rather than the pre-Phase-2 .OrgName.
func TestAPIKeys_KeyCreatedTemplateUsesAccountName(t *testing.T) {
	data, err := templateFS.ReadFile("templates/key_created.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "{{.OrgName}}") {
		t.Error("key_created.html still references .OrgName — should be .AccountName per Cycle B rename")
	}
	if !strings.Contains(s, "{{.AccountName}}") {
		t.Error("key_created.html should reference .AccountName")
	}
}

// TestAPIKeys_KeyCreatedTemplateExposesPlaintext — the one-time
// reveal pattern: the freshly-minted plaintext is shown once on
// this page and never again (it's not persisted; only the hash is).
// Test pins the data-new-key attribute the GUI test fixtures key
// off and the {{.Plaintext}} reference.
func TestAPIKeys_KeyCreatedTemplateExposesPlaintext(t *testing.T) {
	data, err := templateFS.ReadFile("templates/key_created.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	s := string(data)
	for _, needle := range []string{
		"data-new-key",
		"{{.Plaintext}}",
		"copy this now",
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("key_created.html missing %q", needle)
		}
	}
}
