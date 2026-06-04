package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestTopLevelHelp_ListsCoreVerbs pins that the grouped top-level help still
// surfaces the core verbs and points at the topic pages. The detailed prose
// (the inbox aliases, the tabular format) moved to per-command help — see
// TestCommandHelp_CarriesMovedDetail.
func TestTopLevelHelp_ListsCoreVerbs(t *testing.T) {
	t.Setenv("COLUMNS", "10000") // wide enough that no phrase is broken across lines
	text := captureUsage(t)
	for _, want := range []string{
		"ppz read",
		"ppz send",
		"ppz upgrade",
		"ppz help acks", // topic pointer in the footer
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("top-level help missing %q:\n%s", want, text)
		}
	}
}

// TestCommandHelp_CarriesMovedDetail pins that the detail relocated out of the
// top-level block still reaches the user — now via per-command `--help`.
func TestCommandHelp_CarriesMovedDetail(t *testing.T) {
	t.Setenv("COLUMNS", "10000")
	cases := []struct{ key, want string }{
		{"read", "ppz read inbox"},
		{"send", "<handle>.inbox"},
		{"acks", "ack:read"},
		{"sessions", "PPZ_SESSION"},
	}
	for _, c := range cases {
		var b strings.Builder
		printHelp(&b, c.key)
		if !strings.Contains(b.String(), c.want) {
			t.Errorf("`ppz help %s` missing %q:\n%s", c.key, c.want, b.String())
		}
	}
}

// captureUsage renders the top-level help to a string via the os.Pipe seam
// usage() writes through.
func captureUsage(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(out)
}
