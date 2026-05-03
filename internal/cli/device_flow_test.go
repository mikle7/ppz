package cli

import (
	"testing"
)

// verifyURL composes the verification URL the user opens (or has
// auto-opened) in their browser. Uses the URL the CLI was invoked
// with — NOT whatever `verification_uri` the server returns —
// because the server may live behind a different hostname inside
// its own network (compose, k8s, etc.).
//
// The user_code is included as a query param so the browser lands
// on a page where the code is pre-filled. The user just clicks
// "Approve" — no typing required (matches `claude` / `gh` UX).
func TestVerifyURL_UsesClientURLAndIncludesCode(t *testing.T) {
	cases := []struct {
		clientURL, userCode, want string
	}{
		{"https://pipescloud.io", "ABCD-1234",
			"https://pipescloud.io/oauth/device/verify?user_code=ABCD-1234"},
		{"http://localhost:8080", "WXYZ-9876",
			"http://localhost:8080/oauth/device/verify?user_code=WXYZ-9876"},
		// Trailing slash should not double up.
		{"http://localhost:8080/", "WXYZ-9876",
			"http://localhost:8080/oauth/device/verify?user_code=WXYZ-9876"},
	}
	for _, c := range cases {
		got := verifyURL(c.clientURL, c.userCode)
		if got != c.want {
			t.Errorf("verifyURL(%q, %q) = %q, want %q", c.clientURL, c.userCode, got, c.want)
		}
	}
}

// openBrowser dispatches to the platform-typical URL opener
// (`open` on macOS, `xdg-open` on Linux, `rundll32` on Windows).
// Tests inject a stub via the package-level `openCmd` hook so we
// don't actually spawn external processes.
func TestOpenBrowser_DispatchesToPlatformOpener(t *testing.T) {
	var captured []string
	prev := openCmd
	openCmd = func(name string, args ...string) error {
		captured = append([]string{name}, args...)
		return nil
	}
	t.Cleanup(func() { openCmd = prev })

	if err := openBrowser("https://example.com/oauth/device/verify?user_code=X"); err != nil {
		t.Fatalf("openBrowser: %v", err)
	}
	if len(captured) < 2 {
		t.Fatalf("openCmd not invoked; got %v", captured)
	}
	openers := map[string]bool{"open": true, "xdg-open": true, "rundll32": true}
	if !openers[captured[0]] {
		t.Errorf("unexpected opener command %q; want open/xdg-open/rundll32", captured[0])
	}
	if captured[len(captured)-1] != "https://example.com/oauth/device/verify?user_code=X" {
		t.Errorf("URL not last arg; got %v", captured)
	}
}

// `--no-open` (or stdin not being a TTY) suppresses the auto-open
// — for SSH sessions, CI runners, and explicit-control cases.
func TestMaybeOpenBrowser_RespectsAutoOpenFlag(t *testing.T) {
	called := false
	prev := openCmd
	openCmd = func(name string, args ...string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { openCmd = prev })

	maybeOpenBrowser(false, "https://example.com/foo")
	if called {
		t.Error("openCmd must not be called when autoOpen=false")
	}

	maybeOpenBrowser(true, "https://example.com/foo")
	if !called {
		t.Error("openCmd must be called when autoOpen=true")
	}
}
