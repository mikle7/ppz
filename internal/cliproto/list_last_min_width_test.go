package cliproto

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// RED test for v0.31.6: the LAST column should have a stable minimum
// width so that natural relative-time rollovers (e.g. "9 minutes ago"
// → "10 minutes ago" or "1 hour ago" → "2 hours ago") don't shift every
// downstream column right by one. Without a pinned minimum the column
// auto-sizes to its widest data value, which changes silently as time
// passes and produces visible "drift" between successive `ppz ls`
// invocations on the same pipes.
//
// Pin: 14 chars. Covers the common ranges:
//
//	"X minutes ago"  (13–14 chars)
//	"XX minutes ago" (14 chars)
//	"X hours ago"    (10–11 chars)
//	"XX hours ago"   (12 chars)
//	"X days ago"     (10 chars)
//	"just now"       (8 chars)
//
// Rare wider values ("XX seconds ago" = 14, "59 minutes ago" = 15) can
// still push past it — the goal is steady-state stability, not a hard
// cap.

// sampleSourcesShortRelative gives all-short LAST values so the
// auto-sized column would otherwise be narrower than the min. The test
// fails today because writeListTable starts widths[3] at len("LAST")=4
// and grows it only to the data max ("just now" = 8). With the pin in
// place it should clamp to 14.
func sampleSourcesShortRelative(at time.Time) []Source {
	return []Source{
		{
			Handle:    "chat",
			CreatedBy: "foo",
			PipeInfos: []PipeInfo{
				{Pipe: "inbox", Total: 1, Unread: 1, LastAt: &at, Preview: "hi"},
			},
		},
	}
}

// TestPrintList_LASTColumnHasMinimumWidth (RED): every rendered row's
// LAST column field must be padded to at least 14 chars even when all
// LAST values are short ("just now"). Otherwise a later run where one
// pipe rolls over to "20 minutes ago" widens the column and shifts
// PAYLOAD/CREATOR rightward — the "drift" symptom.
func TestPrintList_LASTColumnHasMinimumWidth(t *testing.T) {
	// "just now" — i.e. now - 0s. 8 chars wide.
	withFrozenNow(t, fixedNow())
	at := fixedNow()

	var buf bytes.Buffer
	PrintList(&buf, sampleSourcesShortRelative(at), false)

	// Inspect the header to find the absolute column boundaries.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + at least one row, got %d lines:\n%s", len(lines), buf.String())
	}
	header := lines[0]
	lastIdx := strings.Index(header, "LAST")
	payloadIdx := strings.Index(header, "PAYLOAD")
	if lastIdx < 0 || payloadIdx < 0 {
		t.Fatalf("LAST or PAYLOAD missing from header: %q", header)
	}
	// LAST column width = distance from "LAST" header start to where
	// PAYLOAD header starts, minus the 2-space inter-column separator.
	gotWidth := payloadIdx - lastIdx - 2
	if gotWidth < 14 {
		t.Errorf("LAST column width = %d; want ≥ 14 to absorb relative-time rollovers without drift\nheader: %q", gotWidth, header)
	}
}
