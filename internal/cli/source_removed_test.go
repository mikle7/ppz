package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// RED tests for the pre-launch removal of `ppz source`. See:
//   - docs/PHASE-1-IMPLEMENTATION-PLAN.md (private repo)
//   - Locked decisions #18, #19, #20, #21 in
//     OSS-PIPESCLOUD-ARCHITECTURE-SPLIT.md
//
// The source concept is being dropped entirely — every pipe becomes a
// standalone addressable path, the `sources` table goes, and the
// daemon's "current source" state becomes "current handle". The
// `ppz source` CLI verb is replaced by:
//
//   ppz source create HANDLE   → ppz terminal create HANDLE / ppz agent create HANDLE
//   ppz source switch HANDLE   → ppz set handle HANDLE
//   ppz source clear           → ppz unset handle
//   ppz source destroy HANDLE  → ppz pipe destroy --recursive HANDLE
//
// These tests pin the post-cycle-3 surface. They fail today (the
// source verb still dispatches; set/unset/get verbs don't exist) and
// pass once commits 7-8 land the implementation.

// TestTopLevelVerbs_ExcludesSource asserts the completion engine no
// longer advertises `source`.
func TestTopLevelVerbs_ExcludesSource(t *testing.T) {
	for _, v := range topLevelVerbs {
		if v == "source" {
			t.Errorf("topLevelVerbs still contains %q — pre-launch removal pending", v)
		}
	}
}

// TestSubverbs_ExcludesSource asserts the completion subverb map has
// no `source` key.
func TestSubverbs_ExcludesSource(t *testing.T) {
	if _, ok := subverbs["source"]; ok {
		t.Errorf("subverbs still has key %q — pre-launch removal pending", "source")
	}
}

// TestUsage_OmitsSourceVerbs asserts the top-level usage text does
// not mention `ppz source <verb>` invocations.
func TestUsage_OmitsSourceVerbs(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	usage(w)
	_ = w.Close()
	out, _ := io.ReadAll(r)
	_ = r.Close()

	text := string(out)
	for _, banned := range []string{
		"ppz source create",
		"ppz source switch",
		"ppz source clear",
		"ppz source destroy",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("usage() still mentions %q — pre-launch removal pending", banned)
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
