package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// Tests for the Phase 1 source-verb reshape. See:
//   - docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo)
//   - Locked decisions #18, #19, #20, #21 in
//     OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md
//
// Phase 1 reshaped `ppz source` from a four-subverb family (create /
// switch / clear / destroy) to a *single* surviving subverb:
//
//   ppz source create HANDLE   — claim a bare actor identity (a
//                                 message-kind source, with inbox
//                                 auto-pipe). Distinct from
//                                 `ppz terminal create` (pty pipe
//                                 set) and `ppz agent create` (agent
//                                 pipe set + harness).
//
// The other three subverbs were replaced:
//
//   ppz source switch HANDLE   → ppz set handle HANDLE
//   ppz source clear           → ppz unset handle
//   ppz source destroy HANDLE  → ppz pipe destroy --recursive HANDLE

// TestSubverbs_SourceHasOnlyCreate asserts the completion engine's
// subverbs map for `source` contains only "create" — switch / clear /
// destroy are intentionally gone.
func TestSubverbs_SourceHasOnlyCreate(t *testing.T) {
	subs, ok := subverbs["source"]
	if !ok {
		t.Fatalf("subverbs has no key %q — source create must remain reachable", "source")
	}
	if len(subs) != 1 || subs[0] != "create" {
		t.Errorf("subverbs[%q] = %v, want [\"create\"]", "source", subs)
	}
}

// TestUsage_MentionsSourceCreate asserts the top-level usage text
// still advertises `ppz source create HANDLE` as the bare-actor verb,
// and does NOT advertise the retired switch / clear / destroy subverbs.
func TestUsage_MentionsSourceCreate(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	if !strings.Contains(text, "ppz source create") {
		t.Errorf("usage() missing %q — source create is the bare-actor entry point", "ppz source create")
	}
	for _, banned := range []string{
		"ppz source switch",
		"ppz source clear",
		"ppz source destroy",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — that subverb was removed in Phase 1", banned)
		}
	}
}

// TestTopLevelVerbs_IncludesSetGetUnset asserts the new
// daemon-state CLI pattern surfaces. Locked decision #20:
// `ppz set [key] [value]`, `ppz unset [key]`, `ppz get [key]`.
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
