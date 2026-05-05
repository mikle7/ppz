package cli

import (
	"os"
	"strings"
	"testing"
)

// captureComplete runs cmdComplete with the given args and returns
// stdout as the list of completion candidates the shell would receive.
// We swap os.Stdout for a pipe — cmdComplete writes directly to stdout
// (matching the shell-hook contract) so capturing it is the only way.
func captureComplete(t *testing.T, args []string) []string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	if err := cmdComplete(args); err != nil {
		w.Close()
		t.Fatalf("cmdComplete: %v", err)
	}
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	out := b.String()
	if out == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// TestComplete_TopLevel: `ppz <tab>` lists every top-level verb.
func TestComplete_TopLevel(t *testing.T) {
	got := captureComplete(t, []string{""})
	if !contains(got, "status") || !contains(got, "source") || !contains(got, "read") {
		t.Errorf("expected top-level verbs, got %v", got)
	}
}

// TestComplete_PrefixFilter: `ppz s<tab>` filters to s-prefixed verbs.
func TestComplete_PrefixFilter(t *testing.T) {
	got := captureComplete(t, []string{"s"})
	for _, g := range got {
		if !strings.HasPrefix(g, "s") {
			t.Errorf("got non-s prefix: %q", g)
		}
	}
	// At minimum: send, source, status.
	for _, want := range []string{"send", "source", "status"} {
		if !contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

// TestComplete_Subverb: `ppz source <tab>` returns all four subverbs.
func TestComplete_Subverb(t *testing.T) {
	got := captureComplete(t, []string{"source", ""})
	want := []string{"create", "switch", "clear", "destroy"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("missing %q in %v", w, got)
		}
	}
}

// TestComplete_SubverbPrefix: `ppz source cr<tab>` narrows to "create".
func TestComplete_SubverbPrefix(t *testing.T) {
	got := captureComplete(t, []string{"source", "cr"})
	if len(got) != 1 || got[0] != "create" {
		t.Errorf("got %v, want [create]", got)
	}
}

// TestComplete_DashDashStripped: the shell script passes `--` as the
// first arg; cmdComplete must skip it. Without this, a verb at index 0
// would be misinterpreted as a partial.
func TestComplete_DashDashStripped(t *testing.T) {
	got := captureComplete(t, []string{"--", "source", ""})
	if !contains(got, "create") {
		t.Errorf("expected source subverbs after --, got %v", got)
	}
}

// TestComplete_UnknownVerb: `ppz nope X<tab>` returns nothing — we
// don't speculate about a verb we don't know.
func TestComplete_UnknownVerb(t *testing.T) {
	got := captureComplete(t, []string{"nope", "X"})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
