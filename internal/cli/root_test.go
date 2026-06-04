package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestUsageMentionsInboxAliases(t *testing.T) {
	t.Setenv("COLUMNS", "10000") // wide enough that no phrase is broken across lines
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

	text := string(out)
	for _, want := range []string{
		"ppz read inbox",
		"bare handle → <handle>.inbox",
		"ppz upgrade",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("usage output missing %q:\n%s", want, text)
		}
	}
}
