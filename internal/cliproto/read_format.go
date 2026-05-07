package cliproto

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// senderColumnWidth is the fixed-width column used to keep the body
// column aligned across messages in streaming output. Handle regex caps
// at 32 chars; long senders overflow the column rather than getting
// truncated, on the theory that a misaligned line beats a lost identity.
const senderColumnWidth = 12

// IsTabularReadPipe reports whether `ppz read <handle>.<pipe>` should
// render in the v0.23 three-column tabular form by default. Only the
// human-visible inbox-shaped pipes do — pty pipes (stdout / stdin /
// stdctrl) and user-named custom pipes stay byte-faithful so replays
// work and arbitrary tooling on top doesn't get reflowed bytes.
func IsTabularReadPipe(pipe string) bool {
	return pipe == "inbox" || pipe == "broadcast"
}

// FormatReadMessage renders one ReadMessage in the tabular default. Layout:
//
//	HH:MM:SS  <sender-col>  <body>
//	                        <continuation>
//	                        <continuation>
//
// Body shape:
//   - Subject starting with "ack:" → "<subject> → <last-8-hex-of-id>"
//     (system protocol message — payload is ignored for display).
//   - Subject otherwise non-empty → "[<subject>] <payload-line-1>".
//   - Subject empty → "<payload-line-1>".
//
// Multi-line payloads emit subsequent lines indented under the body column
// (no time, no sender). loc controls the timezone of HH:MM:SS — production
// callers pass time.Local.
func FormatReadMessage(w io.Writer, m ReadMessage, loc *time.Location) {
	t := parseCreatedAt(m.CreatedAt)
	if loc == nil {
		loc = time.UTC
	}
	timeCol := t.In(loc).Format("15:04:05")

	sender := m.Sender
	if sender == "" {
		sender = "-"
	}

	// Compute the body — subject-aware. For ack:* subjects, payload is
	// not displayed; the row is purely "ack:read → <id8>".
	body := bodyForRow(m)

	// Multi-line: split the body into one or more lines and indent
	// continuations under the body column.
	prefix := fmt.Sprintf("%s  %-*s  ", timeCol, senderColumnWidth, sender)
	lines := strings.Split(body, "\n")
	indent := strings.Repeat(" ", len(prefix))
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintln(w, prefix+line)
		} else {
			fmt.Fprintln(w, indent+line)
		}
	}
}

// bodyForRow returns the rendered body — the third column of one row,
// pre-split. ack:* subjects squash to "<subject> → <id8>"; non-ack
// subjects prepend "[<subject>] " to the first payload line.
func bodyForRow(m ReadMessage) string {
	if strings.HasPrefix(m.Subject, "ack:") {
		return fmt.Sprintf("%s → %s", m.Subject, lastHexOfID(m.ID, 8))
	}
	if m.Subject != "" {
		return "[" + m.Subject + "] " + m.Payload
	}
	return m.Payload
}

// lastHexOfID returns the last n hex characters of id, treating the id
// as a UUID-shaped string (dashes stripped). For a malformed / shorter
// id it returns the whole stripped value.
func lastHexOfID(id string, n int) string {
	stripped := strings.ReplaceAll(id, "-", "")
	if len(stripped) <= n {
		return stripped
	}
	return stripped[len(stripped)-n:]
}

// parseCreatedAt parses the daemon's emitted timestamp. The daemon
// formats with "2006-01-02T15:04:05Z" which is a strict subset of
// RFC3339 (no fractional, always Z). Fall back to time.Time{} on
// parse error — the formatter then renders 00:00:00, surfacing the
// problem rather than swallowing it.
func parseCreatedAt(s string) time.Time {
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
