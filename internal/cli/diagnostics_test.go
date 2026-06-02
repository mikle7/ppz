package cli

// Golden-style tests for the diagnostics CLI's default formatter. We
// don't pin the entire string (the output evolves), but we DO pin the
// invariants the pit-of-success contract depends on:
//
//   - Summary line names the connection state.
//   - Pattern hits are surfaced near the top with ⚠.
//   - The hint footer always mentions both --since and --bundle.
//
// If a future contributor reformats the output, the test fails on the
// invariant they broke — not on cosmetic whitespace changes.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestRenderDefault_AlwaysShowsHints(t *testing.T) {
	var buf bytes.Buffer
	renderDefault(&buf, cliproto.DiagReply{
		Summary:    cliproto.DiagSummary{State: "connected", URL: "nats://x:4222"},
		NATSEvents: []cliproto.DiagEvent{},
	}, "")
	out := buf.String()
	for _, want := range []string{"--since=", "--bundle"} {
		if !strings.Contains(out, want) {
			t.Errorf("default output must mention %q so users discover it; got:\n%s", want, out)
		}
	}
}

func TestRenderDefault_SurfacesPatterns(t *testing.T) {
	var buf bytes.Buffer
	at := time.Date(2026, 6, 2, 0, 33, 30, 0, time.UTC)
	renderDefault(&buf, cliproto.DiagReply{
		Summary: cliproto.DiagSummary{State: "connected"},
		Patterns: []cliproto.DiagPattern{
			{Name: "burst-swap-storm", At: at, Detail: "12 swaps in 1.2s"},
		},
		NATSEvents: []cliproto.DiagEvent{},
	}, "")
	out := buf.String()
	if !strings.Contains(out, "⚠") {
		t.Errorf("pattern hit must render with ⚠ marker; got:\n%s", out)
	}
	if !strings.Contains(out, "burst-swap-storm") {
		t.Errorf("pattern name must appear in output; got:\n%s", out)
	}
	if !strings.Contains(out, "12 swaps in 1.2s") {
		t.Errorf("pattern detail must appear in output; got:\n%s", out)
	}
}

func TestRenderDefault_NamesStateInSummary(t *testing.T) {
	var buf bytes.Buffer
	renderDefault(&buf, cliproto.DiagReply{
		Summary:    cliproto.DiagSummary{State: "disconnected"},
		NATSEvents: []cliproto.DiagEvent{},
	}, "")
	out := buf.String()
	if !strings.Contains(out, "disconnected") {
		t.Errorf("summary must name the state; got:\n%s", out)
	}
}

func TestRenderDefault_EmptyEventsRendersExplicitly(t *testing.T) {
	var buf bytes.Buffer
	renderDefault(&buf, cliproto.DiagReply{
		Summary:    cliproto.DiagSummary{State: "unknown"},
		NATSEvents: []cliproto.DiagEvent{},
	}, "")
	out := buf.String()
	if !strings.Contains(out, "(none)") {
		t.Errorf("empty event list must render explicitly so the reader knows it isn't truncated; got:\n%s", out)
	}
}
