package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Phase 2 Cycle E: strip GitHub OAuth implementation from OSS.
//
// What goes: GitHub-specific authorize/callback handlers, the
// /auth/github/* routes, the GitHubClient*/AuthorizeURL/TokenURL/UserURL
// fields on Config + Server, the PPZ_GITHUB_* env reads in main.
//
// What stays: device-code endpoints (provider-agnostic protocol for
// `ppz daemon login` without -apikey), the Provider interface (Cycle D)
// — pipescloud's out-of-tree provider implementation replaces the
// stripped GitHub-specific glue.
//
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle E.

// TestStrip_GitHubRoutesUnmounted — /auth/github/* paths return no
// matched pattern. mux.Handler() returns an empty pattern for any
// path with no registered handler.
func TestStrip_GitHubRoutesUnmounted(t *testing.T) {
	s := &Server{Version: "v0-test"}
	mux := s.Routes()
	for _, path := range []string{
		"/auth/github/start",
		"/auth/github/callback",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			_, pattern := mux.Handler(req)
			if pattern != "" {
				t.Errorf("%s: matched pattern %q, want empty (should be unmounted)", path, pattern)
			}
		})
	}
}

// TestStrip_DeviceCodeRoutesStillMounted — positive assertion. The
// device-code endpoints are provider-agnostic and back `ppz daemon
// login` without -apikey; they must survive the GitHub strip.
func TestStrip_DeviceCodeRoutesStillMounted(t *testing.T) {
	s := &Server{Version: "v0-test"}
	mux := s.Routes()
	for _, tc := range []struct{ method, path string }{
		{"POST", "/oauth/device/code"},
		{"GET", "/oauth/device/verify"},
		{"POST", "/oauth/device/verify"},
		{"POST", "/oauth/device/token"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			_, pattern := mux.Handler(req)
			if pattern == "" {
				t.Errorf("%s %s: no matched pattern; device-code surface must stay mounted",
					tc.method, tc.path)
			}
		})
	}
}

// TestStrip_NoGitHubFieldsOnServer — reflection check. After the strip,
// neither Server nor Config has GitHubClientID, GitHubClientSecret,
// GitHubAuthorizeURL, GitHubTokenURL, or GitHubUserURL.
func TestStrip_NoGitHubFieldsOnServer(t *testing.T) {
	for _, fieldName := range []string{
		"GitHubClientID",
		"GitHubClientSecret",
		"GitHubAuthorizeURL",
		"GitHubTokenURL",
		"GitHubUserURL",
	} {
		if _, ok := reflect.TypeOf(Server{}).FieldByName(fieldName); ok {
			t.Errorf("Server.%s still present — should be stripped", fieldName)
		}
		if _, ok := reflect.TypeOf(Config{}).FieldByName(fieldName); ok {
			t.Errorf("Config.%s still present — should be stripped", fieldName)
		}
	}
}

// TestStrip_NoGitHubOAuthInServerSources — grep-style scan over the
// non-test Go sources under internal/server. No production code should
// import GitHub-OAuth libraries or reference PPZ_GITHUB_* env vars.
//
// Allowed: "github.com/..." import paths (Go module identifiers), test
// helper "mock-github" references (out-of-scope; tests use a docker
// fixture and are migrated separately).
func TestStrip_NoGitHubOAuthInServerSources(t *testing.T) {
	root := "."
	forbidden := []string{
		"PPZ_GITHUB_",
		"handleAuthGitHubStart",
		"handleAuthGitHubCallback",
		"GitHubClientID",
		"GitHubAuthorizeURL",
		"GitHubTokenURL",
		"GitHubUserURL",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(data)
		for _, needle := range forbidden {
			if strings.Contains(s, needle) {
				t.Errorf("%s contains forbidden token %q", path, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
