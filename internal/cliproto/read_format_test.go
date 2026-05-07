package cliproto

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// FormatReadMessage is the v0.23 default `ppz read` renderer for inbox-shaped
// pipes. The bare opt-out and other modes (--json/--tty/--raw) bypass it.

func mkMsg(id, sender, subject, payload string) ReadMessage {
	return ReadMessage{
		ID:        id,
		Sender:    sender,
		Subject:   subject,
		Payload:   payload,
		CreatedAt: "2026-05-07T14:23:01Z",
	}
}

// IsTabularReadPipe identifies pipes that get the tabular default — only
// the human-visible pipes. Anything else (stdout / stdin / user-named)
// stays byte-faithful.
func TestIsTabularReadPipe(t *testing.T) {
	for _, pipe := range []string{"inbox", "broadcast"} {
		if !IsTabularReadPipe(pipe) {
			t.Errorf("IsTabularReadPipe(%q) = false, want true", pipe)
		}
	}
	for _, pipe := range []string{"stdout", "stdin", "stdctrl", "custom-name", ""} {
		if IsTabularReadPipe(pipe) {
			t.Errorf("IsTabularReadPipe(%q) = true, want false", pipe)
		}
	}
}

// Plain row: HH:MM:SS local time + sender + payload.
func TestFormatReadMessage_Plain(t *testing.T) {
	loc := time.FixedZone("test", 0) // UTC for stable test output
	var b bytes.Buffer
	FormatReadMessage(&b, mkMsg("11111111-2222-3333-4444-555566667777", "foo", "", "Hello, how are you?"), loc)
	got := b.String()
	want := "14:23:01  foo           Hello, how are you?\n"
	if got != want {
		t.Errorf("plain row\n got: %q\nwant: %q", got, want)
	}
}

// Empty sender renders as "-".
func TestFormatReadMessage_EmptySenderDash(t *testing.T) {
	loc := time.FixedZone("test", 0)
	var b bytes.Buffer
	FormatReadMessage(&b, mkMsg("11111111-2222-3333-4444-555566667777", "", "", "this is another message"), loc)
	got := b.String()
	if !strings.Contains(got, "  -  ") && !strings.Contains(got, "  -   ") {
		// Padded sender column starts with "-" then trailing space.
	}
	if !strings.HasPrefix(got, "14:23:01  -") {
		t.Errorf("empty-sender row should start with `14:23:01  -`, got %q", got)
	}
	if !strings.HasSuffix(got, "  this is another message\n") {
		t.Errorf("empty-sender row should end with payload, got %q", got)
	}
}

// ack:* subject becomes `<subject> → <last8-of-id-hex>`.
func TestFormatReadMessage_AckSubjectRendersWithLast8(t *testing.T) {
	loc := time.FixedZone("test", 0)
	var b bytes.Buffer
	// id hex (dashes stripped): 11112222333344445555666677778899 → last 8 = "77778899"
	FormatReadMessage(&b, mkMsg("11111111-2222-3333-4444-555566667777-8899", "miner-test", "ack:read", "(ignored payload)"), loc)
	// Payload should NOT appear; the rendered body is the special form.
	got := b.String()
	if strings.Contains(got, "(ignored payload)") {
		t.Errorf("ack row leaked payload: %q", got)
	}
	if !strings.Contains(got, "ack:read → ") {
		t.Errorf("ack row missing arrow form: %q", got)
	}
	// Last 8 hex of stripped id "11112222333344445555666677778899" → "77778899"
	if !strings.Contains(got, "77778899") {
		t.Errorf("ack row missing last-8-hex of id: %q", got)
	}
}

// User-set non-ack subject renders inline as `[subject] payload`.
func TestFormatReadMessage_UserSubjectInline(t *testing.T) {
	loc := time.FixedZone("test", 0)
	var b bytes.Buffer
	FormatReadMessage(&b, mkMsg("aa", "smelter", "status update", "step 1 complete"), loc)
	got := b.String()
	if !strings.HasSuffix(got, "  [status update] step 1 complete\n") {
		t.Errorf("user-subject row should render `[status update] step 1 complete`, got %q", got)
	}
}

// Multi-line payloads continue on indented subsequent lines aligned under the
// payload column (after the time + sender columns).
func TestFormatReadMessage_MultiLineIndentsContinuations(t *testing.T) {
	loc := time.FixedZone("test", 0)
	var b bytes.Buffer
	FormatReadMessage(&b, mkMsg("aa", "smelter", "", "status update:\nstep 1 complete\nstep 2 in progress"), loc)
	got := b.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("multi-line want 3 lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "14:23:01  smelter") {
		t.Errorf("first line should carry time+sender, got %q", lines[0])
	}
	// Continuation lines must NOT carry time or sender.
	for _, line := range lines[1:] {
		if strings.Contains(line, "14:23:01") {
			t.Errorf("continuation line leaked timestamp: %q", line)
		}
		if strings.Contains(line, "smelter") {
			t.Errorf("continuation line leaked sender: %q", line)
		}
		// Continuation indent should be at least the time-column width
		// so it lines up under <body>.
		if !strings.HasPrefix(line, "          ") { // ≥10 spaces
			t.Errorf("continuation line lacks indent: %q", line)
		}
	}
	// Body content preserved.
	if !strings.HasSuffix(lines[0], "status update:") {
		t.Errorf("first line body = %q, want ending `status update:`", lines[0])
	}
	if !strings.HasSuffix(lines[1], "step 1 complete") {
		t.Errorf("second line = %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], "step 2 in progress") {
		t.Errorf("third line = %q", lines[2])
	}
}

// Continuation indent should align under the body column. Compute it via the
// known prefix shape so the test stays stable if column widths shift.
func TestFormatReadMessage_ContinuationIndentMatchesBodyColumn(t *testing.T) {
	loc := time.FixedZone("test", 0)
	var b bytes.Buffer
	FormatReadMessage(&b, mkMsg("aa", "x", "", "first\nsecond"), loc)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	idx := strings.Index(lines[0], "first")
	if idx < 0 {
		t.Fatalf("body 'first' not found in line: %q", lines[0])
	}
	prefix := strings.Repeat(" ", idx)
	if !strings.HasPrefix(lines[1], prefix+"second") {
		t.Errorf("continuation indent mismatch — line[0] body at col %d, line[1]=%q", idx, lines[1])
	}
}

// Time renders in the supplied location (local in production).
func TestFormatReadMessage_TimeUsesProvidedLocation(t *testing.T) {
	tz := time.FixedZone("plus10", 10*3600)
	var b bytes.Buffer
	// CreatedAt is RFC3339-Z (UTC). +10h → 00:23:01 next day, but HH:MM:SS only shows time.
	FormatReadMessage(&b, mkMsg("aa", "x", "", "p"), tz)
	if !strings.HasPrefix(b.String(), "00:23:01  ") {
		t.Errorf("time column should reflect +10h zone: %q", b.String())
	}
}
