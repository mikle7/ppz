package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestHelpTopics_CoverTopLevelVerbs: every dispatchable top-level verb has a
// detailed help body, so `ppz <verb> --help` / `ppz help <verb>` always
// resolves to real content rather than the top-level fallback.
func TestHelpTopics_CoverTopLevelVerbs(t *testing.T) {
	for _, verb := range topLevelVerbs {
		body, ok := helpTopics[verb]
		if !ok || strings.TrimSpace(body) == "" {
			t.Errorf("verb %q has no helpTopics entry (add one, or exclude the verb)", verb)
		}
	}
}

// TestHelpGroups_CoverTopLevelVerbs is the source-of-truth bridge to
// completion.go: every verb listed in the dispatch metadata (topLevelVerbs)
// must appear as a command row in the grouped top-level help, and every row's
// leading verb must be a real verb. This keeps `ppz help` and tab-completion
// from drifting apart.
func TestHelpGroups_CoverTopLevelVerbs(t *testing.T) {
	// `completion` is intentionally absent from topLevelVerbs (operator-
	// internal per completion.go) but DOES get a help row — allow it.
	known := map[string]bool{"completion": true}
	for _, v := range topLevelVerbs {
		known[v] = true
	}

	inGroups := map[string]bool{}
	for _, g := range topLevelGroups {
		for _, r := range g.rows {
			verb := leadingVerb(r.sig)
			if verb == "" {
				t.Errorf("group %q has a row with no leading verb: %q", g.title, r.sig)
				continue
			}
			if !known[verb] {
				t.Errorf("group %q lists %q whose verb %q is not a dispatchable verb", g.title, r.sig, verb)
			}
			inGroups[verb] = true
		}
	}

	for _, v := range topLevelVerbs {
		if !inGroups[v] {
			t.Errorf("verb %q is dispatchable but missing from the grouped top-level help", v)
		}
	}
}

// TestHelpTopics_Topics: the cross-cutting concept pages resolve and are
// non-empty (they're referenced by the top-level footer + per-command help).
func TestHelpTopics_Topics(t *testing.T) {
	for _, topic := range []string{"acks", "sessions", "globs"} {
		if strings.TrimSpace(helpTopics[topic]) == "" {
			t.Errorf("topic %q missing or empty in helpTopics", topic)
		}
	}
}

// TestWantsHelp pins the help-detection contract, including the "--" stop that
// lets passthrough verbs forward help flags to the wrapped command.
func TestWantsHelp(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"foo", "--help"}, true},
		{nil, false},
		{[]string{"foo", "bar"}, false},
		// The bare word "help" is NOT a help request at the leaf level — it's
		// a legitimate payload/handle/instruction and must pass through.
		{[]string{"help"}, false},          // e.g. send to a handle named "help"
		{[]string{"alice", "help"}, false}, // ppz send alice help → send "help"
		{[]string{"H", "help"}, false},     // ppz command H help → type "help"
		{[]string{"H", "--", "--help"}, false},        // ppz command H -- --help
		{[]string{"H", "--", "cmd", "--help"}, false}, // ppz terminal share H -- cmd --help
	}
	for _, c := range cases {
		if got := wantsHelp(c.args); got != c.want {
			t.Errorf("wantsHelp(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

// TestGroupHelp_RoutesSubverbDetail: `ppz <group> <sub> --help` resolves to the
// "<group> <sub>" body, and a bare `ppz <group> --help` resolves to the group
// overview. Verified via the rendered output (printHelp goes through the shared
// renderer, so a routed body is observable in the text).
func TestGroupHelp_RoutesSubverbDetail(t *testing.T) {
	// "source destroy" detail mentions the glob examples; the group overview
	// does not. Capture stdout to tell them apart.
	out := captureStdout(t, func() {
		if !groupHelp("source", []string{"destroy", "--help"}) {
			t.Fatal("groupHelp did not handle `source destroy --help`")
		}
	})
	// "path.Match" appears only in the subverb detail, not the group overview.
	if !strings.Contains(out, "path.Match") {
		t.Errorf("`source destroy --help` did not render the subverb detail:\n%s", out)
	}
}

// TestPrintHelp_UnknownKeyFallsBack: a missing key degrades to the grouped
// top-level help rather than printing nothing.
func TestPrintHelp_UnknownKeyFallsBack(t *testing.T) {
	var b strings.Builder
	printHelp(&b, "no-such-key")
	if !strings.Contains(b.String(), "MESSAGING") {
		t.Errorf("unknown key did not fall back to top-level help:\n%s", b.String())
	}
}

// leadingVerb returns the verb token of a help-row signature ("ppz source
// create H" → "source"). Empty if the signature doesn't start with "ppz ".
func leadingVerb(sig string) string {
	fields := strings.Fields(sig)
	if len(fields) < 2 || fields[0] != "ppz" {
		return ""
	}
	return fields[1]
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// it wrote — mirrors captureComplete in completion_test.go.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out)
}
