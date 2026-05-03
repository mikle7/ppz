package cli

// Helpers for the OAuth 2.0 device-authorization-grant client flow,
// shared between cmdDaemonLogin and tests.

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// verifyURL composes the verification URL the user opens in their
// browser. We deliberately ignore the server's `verification_uri`
// response — the server may live behind a different hostname inside
// its own network (compose, k8s) than the URL the CLI hit. The CLI
// trusts what the user typed; that's the URL their browser must hit.
//
// The user_code is included so the browser lands on the verify page
// with the code pre-filled — user just clicks "Approve as me", no
// typing required.
func verifyURL(clientURL, userCode string) string {
	clientURL = strings.TrimRight(clientURL, "/")
	return clientURL + "/oauth/device/verify?user_code=" + userCode
}

// openCmd is the package-level hook for spawning the platform's
// browser-opener command. Tests override this to capture the call
// without actually spawning a child process. Real impl shells out.
var openCmd = func(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// openBrowser is best-effort — if it fails (no display, missing
// command, sandboxing), the caller falls back to "please open this
// URL manually" instructions. Returns nil on a successful spawn.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return openCmd("open", url)
	case "windows":
		return openCmd("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, …
		return openCmd("xdg-open", url)
	}
}

// maybeOpenBrowser is the gated wrapper. autoOpen=false skips the
// open entirely (--no-open flag, or detected non-interactive use).
func maybeOpenBrowser(autoOpen bool, url string) {
	if !autoOpen {
		return
	}
	if err := openBrowser(url); err != nil {
		// Non-fatal; the printed URL is the fallback path.
		fmt.Println("(could not open browser automatically — open the URL above manually)")
	}
}
