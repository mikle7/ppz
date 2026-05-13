package cli

import (
	"io"
	"os"
	"sort"
	"strings"
	"testing"
)

// Tests for the Phase 1 source-verb reshape. See:
//   - docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo)
//   - Locked decisions #18, #19, #20, #21 in
//     OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md
//
// Phase 1 reshaped `ppz source` from a four-subverb family. Two
// subverbs survive (replacements for the others weren't expressive
// enough — glob destroy in particular):
//
//   ppz source create HANDLE   — claim a bare actor identity (a
//                                 message-kind source, with inbox
//                                 auto-pipe). Distinct from
//                                 `ppz terminal create` (pty pipe
//                                 set) and `ppz agent create` (agent
//                                 pipe set + harness).
//   ppz source destroy PATTERN — glob-destroy sources / pipes. The
//                                 expressive bits (glob across
//                                 handles, pipe-pattern that crosses
//                                 source boundaries) don't have a
//                                 clean replacement; the verb stays.
//
// The other two subverbs were replaced cleanly:
//
//   ppz source switch HANDLE   → ppz set handle HANDLE
//   ppz source clear           → ppz unset handle

// TestSubverbs_SourceHasCreateAndDestroy asserts the completion
// engine's subverbs map for `source` contains exactly "create" and
// "destroy" — switch / clear are intentionally gone (replaced by set
// handle / unset handle).
func TestSubverbs_SourceHasCreateAndDestroy(t *testing.T) {
	subs, ok := subverbs["source"]
	if !ok {
		t.Fatalf("subverbs has no key %q — source create/destroy must remain reachable", "source")
	}
	got := append([]string{}, subs...)
	sort.Strings(got)
	want := []string{"create", "destroy"}
	if len(got) != len(want) {
		t.Fatalf("subverbs[%q] = %v, want %v", "source", subs, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("subverbs[%q] = %v, want %v", "source", subs, want)
		}
	}
}

// TestUsage_MentionsSourceCreateAndDestroy asserts the top-level
// usage text advertises `ppz source create` and `ppz source destroy`,
// and does NOT advertise the retired switch / clear subverbs (their
// replacements live under `ppz set` / `ppz unset`).
func TestUsage_MentionsSourceCreateAndDestroy(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	for _, want := range []string{
		"ppz source create",
		"ppz source destroy",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage() missing %q", want)
		}
	}
	for _, banned := range []string{
		"ppz source switch",
		"ppz source clear",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — that subverb was removed in Phase 1", banned)
		}
	}
}

// TestUsage_MentionsSetGetUnset asserts the top-level usage text
// advertises `ppz set/unset/get` as discoverable verbs. Locked
// decision #20 introduced them as the new daemon-state CLI
// pattern (replacing `ppz source switch` / `ppz source clear`),
// but the initial Phase 1 commit added them to dispatch +
// completion without updating the usage text — so `ppz --help`
// users can't find them.
//
// Found by manual smoke test against a real daemon: `ppz --help |
// grep "ppz set"` returns nothing.
func TestUsage_MentionsSetGetUnset(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	for _, want := range []string{
		"ppz set ",
		"ppz unset ",
		"ppz get ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage() missing %q — new daemon-state verbs need to be discoverable in --help", want)
		}
	}
}

// TestTopLevelVerbs_IncludesSetGetUnset asserts the new
// daemon-state CLI pattern surfaces in completion. Locked decision
// #20: `ppz set [key] [value]`, `ppz unset [key]`, `ppz get [key]`.
func TestTopLevelVerbs_IncludesSetGetUnset(t *testing.T) {
	for _, want := range []string{"set", "unset", "get"} {
		found := false
		for _, v := range topLevelVerbs {
			if v == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("topLevelVerbs missing %q — pre-launch addition pending (locked decision #20)", want)
		}
	}
}
