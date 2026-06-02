package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/version"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{name: "patch newer", latest: "v0.17.10", current: "v0.17.9", want: true},
		{name: "minor newer without v", latest: "0.18.0", current: "v0.17.9", want: true},
		{name: "same", latest: "v0.17.9", current: "0.17.9", want: false},
		{name: "older", latest: "v0.17.8", current: "v0.17.9", want: false},
		{name: "dev current skipped", latest: "v0.17.10", current: "dev", want: false},
		{name: "dirty release still compares", latest: "v0.17.10", current: "v0.17.9-dirty", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNewerVersion(tt.latest, tt.current); got != tt.want {
				t.Fatalf("isNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestNormaliseVersionForDisplay(t *testing.T) {
	tests := map[string]string{
		"0.17.9":  "v0.17.9",
		"v0.17.9": "v0.17.9",
		"dev":     "dev",
		"":        "unknown",
	}
	for in, want := range tests {
		if got := normaliseVersionForDisplay(in); got != want {
			t.Fatalf("normaliseVersionForDisplay(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsExactReleaseVersion(t *testing.T) {
	tests := map[string]bool{
		"v0.17.9":           true,
		"0.17.9":            true,
		"v0.17.9-1-gabc123": false,
		"dev":               false,
	}
	for in, want := range tests {
		if got := isExactReleaseVersion(in); got != want {
			t.Fatalf("isExactReleaseVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRunInstallScriptDownloadsAndExecutesInstaller(t *testing.T) {
	out := filepath.Join(t.TempDir(), "upgrade-ran")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript")
		_, _ = w.Write([]byte("#!/usr/bin/env bash\nprintf ok > \"$PPZ_TEST_UPGRADE_OUT\"\n"))
	}))
	t.Cleanup(server.Close)
	t.Setenv("PPZ_TEST_UPGRADE_OUT", out)

	if err := runInstallScript(context.Background(), server.URL); err != nil {
		t.Fatalf("runInstallScript: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read installer output: %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("installer output = %q, want ok", got)
	}
}

func TestFetchUpdateManifestUsesConfiguredRawManifestURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			t.Fatalf("manifest request path = %q, want /manifest.json", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"latest_version":"v9.9.9","install_url":"https://example.test/install.sh"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("PPZ_UPDATE_MANIFEST_URL", server.URL+"/manifest.json")

	manifest, err := fetchUpdateManifest(context.Background())
	if err != nil {
		t.Fatalf("fetchUpdateManifest: %v", err)
	}
	if manifest.LatestVersion != "v9.9.9" {
		t.Fatalf("LatestVersion = %q, want v9.9.9", manifest.LatestVersion)
	}
	if manifest.InstallURL != "https://example.test/install.sh" {
		t.Fatalf("InstallURL = %q, want configured installer", manifest.InstallURL)
	}
}

func TestMaybeNotifyUpdatePrintsNoticeForReleaseBuild(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.3"
	t.Cleanup(func() { version.Version = oldVersion })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"latest_version":"v1.2.4"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("PPZ_UPDATE_MANIFEST_URL", server.URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	maybeNotifyUpdate()
	_ = w.Close()
	os.Stderr = oldStderr
	t.Cleanup(func() { os.Stderr = oldStderr })

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "update available: ppz v1.2.4 (current v1.2.3); run 'ppz upgrade'") {
		t.Fatalf("update notice = %q, want newer-version upgrade hint", text)
	}
}

// withMockedReleaseEnv pretends the running binary is a release of
// `release` and that the manifest server reports `latest` available.
// Captures os.Stderr writes during `body` and returns them.
func withMockedReleaseEnv(t *testing.T, release, latest string, env map[string]string, body func()) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"latest_version":"` + latest + `"}`))
	}))
	t.Cleanup(srv.Close)

	oldVersion := version.Version
	version.Version = release
	t.Cleanup(func() { version.Version = oldVersion })

	t.Setenv("PPZ_UPDATE_MANIFEST_URL", srv.URL)
	t.Setenv("PPZ_UPDATE_CHECK", "")
	for k, v := range env {
		t.Setenv(k, v)
	}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	body()

	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out)
}

// TestMaybeNotifyUpdate_NoticeRendersAmber RED: with an update available
// and color forced on (FORCE_COLOR=1), the notice should carry ANSI
// yellow/amber escape codes (\x1b[33m … \x1b[0m). The current
// implementation writes plain text unconditionally, so this fails today.
//
// Amber rather than red so the line reads as informational ("heads up,
// upgrade available") and not as a hard error like "not running" or
// "authentication error" (which use red elsewhere in the status block).
func TestMaybeNotifyUpdate_NoticeRendersAmber(t *testing.T) {
	out := withMockedReleaseEnv(t, "v0.31.2", "v0.31.99", map[string]string{
		"FORCE_COLOR": "1",
		"NO_COLOR":    "",
	}, func() {
		maybeNotifyUpdate()
	})

	if !strings.Contains(out, "update available") {
		t.Fatalf("expected notice in stderr, got %q", out)
	}
	if !strings.Contains(out, "\x1b[33m") {
		t.Errorf("expected amber/yellow ANSI opener \\x1b[33m in notice, got %q", out)
	}
	if !strings.Contains(out, "\x1b[0m") {
		t.Errorf("expected ANSI reset \\x1b[0m at end of notice, got %q", out)
	}
}

// TestMaybeNotifyUpdate_NoColorEnvSuppressesAmber asserts that the
// https://no-color.org/ contract is honoured: even when an update is
// available and FORCE_COLOR would normally turn colour on, NO_COLOR
// wins and the notice is plain text. The text still appears — only the
// escape codes are suppressed.
func TestMaybeNotifyUpdate_NoColorEnvSuppressesAmber(t *testing.T) {
	out := withMockedReleaseEnv(t, "v0.31.2", "v0.31.99", map[string]string{
		"NO_COLOR":    "1",
		"FORCE_COLOR": "1", // NO_COLOR must take precedence
	}, func() {
		maybeNotifyUpdate()
	})

	if !strings.Contains(out, "update available") {
		t.Fatalf("expected notice in stderr, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR=1 must suppress ANSI escapes, got %q", out)
	}
}

// TestLsCallsMaybeNotifyUpdate RED: the body of cmdLs in ls.go must
// invoke maybeNotifyUpdate() so users running `ppz ls` (the most-
// frequent command per field usage) see upgrade prompts the same way
// `ppz status` and `ppz version` do today. Source-level structural
// assertion because the function writes directly to os.Stderr — a
// runtime test would need full IPC + manifest plumbing here just to
// observe a no-op when the call is missing.
func TestLsCallsMaybeNotifyUpdate(t *testing.T) {
	data, err := os.ReadFile("ls.go")
	if err != nil {
		t.Fatalf("read ls.go: %v", err)
	}
	if !bytes.Contains(data, []byte("maybeNotifyUpdate()")) {
		t.Errorf("ls.go must call maybeNotifyUpdate() like status.go does, but the call is missing")
	}
}

// TestUpdateFetchDeadline_DefaultCoversTypicalLatency is the regression
// guard for the 2026-06-02 user report ("ppz update checking is broken",
// #94). The manifest URL is served by raw.githubusercontent.com, which
// routinely takes ~770–890ms to respond on normal networks (measured
// directly). If the *default* fetch deadline drops below that band,
// every check hits context.DeadlineExceeded, the error is silently
// swallowed (fetchLatestIfNewer returns "", false), and `ppz status` /
// `ppz version` show no notification even when a newer release exists —
// exactly the 750ms bug #94 fixed.
//
// This pins the default constant directly rather than sleeping a real
// ~900ms in the suite: it is deterministic, instant, and fails the
// moment someone tightens the default back under the latency band. The
// full fetch->parse->notify path and the PPZ_UPDATE_TIMEOUT override are
// exercised end-to-end by TestFetchLatestIfNewer_HonorsUpdateTimeoutEnvOverride.
func TestUpdateFetchDeadline_DefaultCoversTypicalLatency(t *testing.T) {
	const minBudget = 1500 * time.Millisecond
	if updateFetchTimeout < minBudget {
		t.Fatalf("default update fetch deadline = %s, want >= %s to clear typical raw.githubusercontent.com latency (~770–890ms) with margin; "+
			"a tighter default silently suppresses the update notice (#94)", updateFetchTimeout, minBudget)
	}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// everything written to it. The update notice is a single short line, so
// it fits the pipe buffer without a draining goroutine.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	_ = r.Close()
	return string(out)
}

// TestFetchLatestIfNewer_HonorsUpdateTimeoutEnvOverride pins the
// follow-up to #94: the manifest-fetch deadline must be operator-tunable
// via PPZ_UPDATE_TIMEOUT, not a single hardcoded guess. #94 widened the
// deadline from 750ms to a fixed 2s based on one network's measurement,
// but 2s is still a guess — a user on a high-latency link (mobile,
// satellite, distant CDN edge) can exceed it and silently get no notice,
// the very failure #94 fixed. An override (mirroring PPZ_IPC_TIMEOUT)
// lets them widen it; it also lets this test drive the real
// fetch->parse->notify path deterministically at small delays instead of
// sleeping ~900ms of real time.
//
// A manifest server responds after a fixed delay; the override must be
// able to BOTH tighten the deadline below that delay (fetch times out ->
// notice suppressed) and widen it above (fetch completes -> notice
// prints).
//
// RED: with a hardcoded deadline that ignores the env, the tight 20ms
// override is disregarded, the 120ms response slips under the 2s default,
// and the notice prints when it should have been suppressed.
// GREEN: the override is honored, so 20ms times out and 2s does not.
func TestFetchLatestIfNewer_HonorsUpdateTimeoutEnvOverride(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.3"
	t.Cleanup(func() { version.Version = oldVersion })

	const serverDelay = 120 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(serverDelay)
		_, _ = w.Write([]byte(`{"latest_version":"v1.2.4"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PPZ_UPDATE_MANIFEST_URL", srv.URL)

	// Tighter than the server delay: the fetch must time out and the
	// notice must be suppressed.
	t.Setenv("PPZ_UPDATE_TIMEOUT", "20ms")
	if out := captureStderr(t, maybeNotifyUpdate); strings.Contains(out, "update available") {
		t.Fatalf("PPZ_UPDATE_TIMEOUT=20ms should have timed out the %s fetch and suppressed the notice, but got: %q", serverDelay, out)
	}

	// Wider than the server delay: the fetch completes and the notice
	// prints — proving the override widens as well as tightens (and that
	// the suppression above was the timeout, not a broken fetch).
	t.Setenv("PPZ_UPDATE_TIMEOUT", "2s")
	if out := captureStderr(t, maybeNotifyUpdate); !strings.Contains(out, "update available: ppz v1.2.4 (current v1.2.3)") {
		t.Fatalf("PPZ_UPDATE_TIMEOUT=2s should have let the %s fetch complete and printed the notice, but got: %q", serverDelay, out)
	}
}
