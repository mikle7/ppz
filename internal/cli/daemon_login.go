package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/daemon"
	"github.com/pipescloud/ppz/internal/version"
)

// cmdDaemonLogin handles `ppz [daemon] login URL [-apikey K]`.
//
// Two auth paths:
//   - With  -apikey K  → V1 path, daemon stores the key as the bearer
//   - Without -apikey  → V2 device flow (a la `gh auth login`):
//     1. POST URL/oauth/device/code → get user_code + device_code
//     2. Print user_code + verification URL, ask the user to visit
//     it in a browser and click "Approve"
//     3. Poll URL/oauth/device/token until approved → bearer token
//     4. Hand the bearer to the daemon's existing Login IPC (which
//     stores it and uses it as `Authorization: Bearer <value>`
//     for all subsequent API calls)
//
// Auto-starts the daemon if it isn't running.
func cmdDaemonLogin(args []string) error {
	if wantsHelp(args) {
		printHelp(os.Stdout, "login")
		return nil
	}
	if len(args) < 1 {
		usageExit("login")
	}
	url := normaliseLoginURL(args[0])
	rest := args[1:]
	fs := flag.NewFlagSet("daemon login", flag.ExitOnError)
	apikey := fs.String("apikey", "", "API key issued by the server (omit to use device flow)")
	noOpen := fs.Bool("no-open", false, "device flow: don't auto-open the browser")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	credential := *apikey
	if credential == "" {
		got, err := runDeviceFlow(url, !*noOpen)
		if err != nil {
			return err
		}
		credential = got
	}

	already, pid, err := ensureDaemonRunning()
	if err != nil {
		return err
	}
	if !already {
		fmt.Fprintf(os.Stdout, "daemon started pid=%d\n", pid)
	}
	var reply cliproto.LoginReply
	if err := daemon.Call(ipcSocket(), cliproto.IPCLogin, cliproto.LoginRequest{URL: url, APIKey: credential}, &reply); err != nil {
		return err
	}
	cliproto.PrintLogin(os.Stdout, reply)
	maybeNotifyUpdate()
	return nil
}

// runDeviceFlow drives the OAuth 2.0 Device Authorization Grant against
// `url` and returns the resulting bearer token. With autoOpen=true,
// it shells out to the platform browser-opener so the user just clicks
// "Approve" — no typing the user_code.
func runDeviceFlow(url string, autoOpen bool) (string, error) {
	// 1. Mint device + user codes. Send our identity so the verify
	//    page can name the client (e.g. "ppz CLI 0.15.0 (abc1234)
	//    would like to connect").
	reqBody, _ := json.Marshal(struct {
		ClientName string `json:"client_name"`
	}{ClientName: fmt.Sprintf("ppz CLI %s (%s)", version.Version, version.BuildSHA)})
	resp, err := http.Post(url+"/oauth/device/code", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("device code: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device code: HTTP %d: %s", resp.StatusCode, body)
	}
	var dc struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		Interval   int    `json:"interval"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &dc); err != nil {
		return "", fmt.Errorf("device code parse: %w", err)
	}

	// 2. Build the browser URL ourselves (don't trust the server's
	//    verification_uri — it may be a compose-internal hostname).
	//    Include user_code so the verify page is pre-filled.
	browseURL := verifyURL(url, dc.UserCode)

	// 3. Print instructions + (best-effort) auto-open.
	fmt.Fprintf(os.Stdout, "Opening browser to authorize this device:\n  %s\n", browseURL)
	fmt.Fprintf(os.Stdout, "(if the browser doesn't open, paste the URL above; code is %s)\n", dc.UserCode)
	fmt.Fprintf(os.Stdout, "waiting for approval...\n")
	maybeOpenBrowser(autoOpen, browseURL)

	// 3. Poll until approved, expired, or rejected.
	interval := time.Duration(dc.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)
		token, errCode, err := pollDeviceToken(url, dc.DeviceCode)
		if err != nil {
			return "", fmt.Errorf("poll: %w", err)
		}
		if token != "" {
			fmt.Fprintln(os.Stdout, "✓ approved")
			return token, nil
		}
		switch errCode {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 1 * time.Second
		case "expired_token":
			return "", fmt.Errorf("device code expired before approval — re-run `ppz login`")
		case "invalid_grant":
			return "", fmt.Errorf("device code invalid — re-run `ppz login`")
		default:
			return "", fmt.Errorf("unexpected error %q", errCode)
		}
	}
	return "", fmt.Errorf("timed out waiting for approval")
}

// pollDeviceToken returns (token, "", nil) on success, ("", errCode, nil)
// on RFC 8628 known errors, ("", "", err) on transport/parse errors.
func pollDeviceToken(url, deviceCode string) (string, string, error) {
	req := struct {
		DeviceCode string `json:"device_code"`
	}{DeviceCode: deviceCode}
	body, _ := json.Marshal(req)
	resp, err := http.Post(url+"/oauth/device/token", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var ok struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(respBody, &ok); err != nil {
			return "", "", err
		}
		return ok.AccessToken, "", nil
	}

	var errResp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(respBody, &errResp)
	return "", errResp.Error, nil
}

// cmdDaemonLogout clears stored credentials, the current source pointer, and
// the per-session cursors directory under $PPZ_HOME. Idempotent: if any file
// is already absent, that's fine.
//
// Implemented client-side (file removal + brief sleep so the daemon's 200 ms
// poller picks up the deletion). Avoids adding a new IPC method for what is
// fundamentally "blow away three files".
func cmdDaemonLogout(args []string) error {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz daemon logout")
		os.Exit(2)
	}
	h := home()
	for _, name := range []string{"credentials", "current"} {
		_ = os.Remove(filepath.Join(h, name))
	}
	_ = os.RemoveAll(filepath.Join(h, "cursors"))
	fmt.Fprintln(os.Stdout, "logged out")
	return nil
}

// normaliseLoginURL accepts either a bare hostname (e.g.
// `pipescloud.io`) or a fully-qualified URL and returns a
// canonical-form `<scheme>://<host>[:port][/path]`. Heuristics:
//
//   - explicit http:// or https:// → keep, just trim trailing slash
//   - localhost / 127.0.0.1 / 0.0.0.0 / non-dotted hostnames →
//     assume http (local dev rarely has TLS)
//   - everything else → assume https
//
// Trailing slashes on the netloc-only part are dropped because the
// device-flow URLs we build later concatenate `/oauth/...` paths.
func normaliseLoginURL(in string) string {
	in = strings.TrimSpace(in)
	if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
		return strings.TrimRight(in, "/")
	}
	scheme := "https://"
	host := in
	if i := strings.Index(in, "/"); i >= 0 {
		host = in[:i]
	}
	if isLocalDevHost(host) {
		scheme = "http://"
	}
	return strings.TrimRight(scheme+in, "/")
}

func isLocalDevHost(host string) bool {
	// Strip :port for the heuristic check.
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	}
	// Compose-internal hostnames: no dots → not a public domain.
	return !strings.Contains(host, ".")
}
