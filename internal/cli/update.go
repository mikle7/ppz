package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/pipescloud/ppz/internal/version"
)

const (
	defaultUpdateManifestURL = "https://raw.githubusercontent.com/pipescloud/ppz/main/update/manifest.json"
	defaultInstallScriptURL  = "https://raw.githubusercontent.com/pipescloud/ppz/main/install.sh"
)

type updateManifest struct {
	LatestVersion string `json:"latest_version"`
	InstallURL    string `json:"install_url,omitempty"`
	ReleaseURL    string `json:"release_url,omitempty"`
}

func maybeNotifyUpdate() {
	if os.Getenv("PPZ_UPDATE_CHECK") == "0" {
		return
	}
	if !isExactReleaseVersion(version.Version) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	manifest, err := fetchUpdateManifest(ctx)
	if err != nil || !isNewerVersion(manifest.LatestVersion, version.Version) {
		return
	}
	msg := fmt.Sprintf("update available: ppz %s (current %s); run 'ppz upgrade'",
		normaliseVersionForDisplay(manifest.LatestVersion), normaliseVersionForDisplay(version.Version))
	if useStderrColor() {
		// 33 = yellow / amber. Informational tone — distinct from the
		// red used elsewhere for hard-error states ("not running",
		// "authentication error").
		msg = "\x1b[33m" + msg + "\x1b[0m"
	}
	fmt.Fprintln(os.Stderr, msg)
}

// useStderrColor decides whether the update notice (written to stderr)
// should carry ANSI escapes. NO_COLOR wins over everything per
// https://no-color.org/. FORCE_COLOR turns it on regardless of tty.
// Default: on when stderr is an interactive terminal.
func useStderrColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func fetchUpdateManifest(ctx context.Context) (updateManifest, error) {
	url := os.Getenv("PPZ_UPDATE_MANIFEST_URL")
	if url == "" {
		url = defaultUpdateManifestURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return updateManifest{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return updateManifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return updateManifest{}, fmt.Errorf("update manifest: HTTP %d", resp.StatusCode)
	}
	var manifest updateManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return updateManifest{}, err
	}
	return manifest, nil
}

func cmdUpgrade(args []string) error {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz upgrade")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	installURL := defaultInstallScriptURL
	if v := os.Getenv("PPZ_INSTALL_SCRIPT_URL"); v != "" {
		installURL = v
	} else if manifest, err := fetchUpdateManifest(ctx); err == nil && manifest.InstallURL != "" {
		installURL = manifest.InstallURL
	}
	return runInstallScript(context.Background(), installURL)
}

func runInstallScript(ctx context.Context, installURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, installURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download installer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download installer: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "ppz-install-*.sh")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o700); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "bash", filepath.Clean(tmpPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Run()
}

func isNewerVersion(latest, current string) bool {
	l, ok := parseReleaseVersion(latest)
	if !ok {
		return false
	}
	c, ok := parseReleaseVersion(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseReleaseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "+-"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func isExactReleaseVersion(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func normaliseVersionForDisplay(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	if _, ok := parseReleaseVersion(v); ok {
		return "v" + v
	}
	return v
}
