package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
