package cliproto

import (
	"bytes"
	"strings"
	"testing"
)

// RED tests for the post-Phase-1 wording-drift findings from the
// manual smoke test against a real daemon. Phase 1 renamed
// `organisations` → `accounts` in the schema, Go types, and JSON
// tags — but several user-facing output strings still say "org".
// These tests pin the corrected wording.

// TestPrintLogin_UsesAccountWording asserts the `ppz daemon login`
// success line says `account=<uuid>` (not `org=<uuid>`).
//
// Found by manual smoke test against compose-up ppz-server: actual
// output today is `logged in url=… key=… org=ced45940-…`. The
// `org=` token leaks the pre-Phase-1 vocabulary into user-visible
// CLI output despite the underlying field having already been
// renamed AccountID. See cliproto/format.go ~L218.
func TestPrintLogin_UsesAccountWording(t *testing.T) {
	var buf bytes.Buffer
	r := LoginReply{
		URL:       "http://localhost:8080",
		KeyPrefix: "abcd1234",
		AccountID: "ced45940-82ac-498c-973f-812dfd15327a",
	}
	PrintLogin(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "account=") {
		t.Errorf("PrintLogin output missing `account=`; got %q", out)
	}
	if strings.Contains(out, "org=") {
		t.Errorf("PrintLogin output still contains `org=` (pre-Phase-1 wording); got %q", out)
	}
}

// TestPrintStatus_UsesAccountLineLabel asserts `ppz status` output
// uses `account: <name>` (not `org: <name>`) as the line label.
//
// Found by manual smoke test: actual is `org: alpha`. format.go
// L131/L133 still hardcodes the `"org: %s\n"` format string.
func TestPrintStatus_UsesAccountLineLabel(t *testing.T) {
	var buf bytes.Buffer
	s := StatusReply{
		DaemonPID:   12345,
		LoggedIn:    true,
		LoginCheck:  LoginCheckOK,
		URL:         "http://localhost:8080",
		KeyPrefix:   "abcd1234",
		AccountID:   "ced45940-82ac-498c-973f-812dfd15327a",
		AccountName: "alpha",
	}
	PrintStatus(&buf, s)
	out := buf.String()
	if !strings.Contains(out, "account:") {
		t.Errorf("PrintStatus output missing `account:` line label; got %q", out)
	}
	for _, banned := range []string{"\norg:", "^org:"} {
		// Match `org:` only when it's the start of a line — avoid false
		// positives on "org_id" or similar substrings (there aren't any
		// today, but stay defensive).
		if strings.Contains(out, "\norg: ") || strings.HasPrefix(out, "org: ") {
			t.Errorf("PrintStatus output still has `org:` line label (pre-Phase-1); checked %q against output %q", banned, out)
		}
	}
}

// TestNewSourceTaken_UsesAccountWording asserts the E_SOURCE_TAKEN
// error message says "in this account" (not "in this org").
//
// Found by manual smoke test: actual error today is
// `E_SOURCE_TAKEN: source 'cindy' already exists in this org`.
// cliproto/errors.go L80, L136, L138 hardcode "in this org".
func TestNewSourceTaken_UsesAccountWording(t *testing.T) {
	err := NewSourceTaken("cindy")
	if !strings.Contains(err.Message, "in this account") {
		t.Errorf("E_SOURCE_TAKEN message missing `in this account`; got %q", err.Message)
	}
	if strings.Contains(err.Message, "in this org") {
		t.Errorf("E_SOURCE_TAKEN message still contains `in this org` (pre-Phase-1); got %q", err.Message)
	}
}
