package cliproto

import (
	"bytes"
	"strings"
	"testing"
)

// PrintOrgList renders `ppz org list` as an aligned NAME/ROLE table
// (mirrors `ppz ls`) and tags the active org with green "(current)"
// when colour is enabled.

func TestPrintOrgList_AlignsNameAndRole(t *testing.T) {
	orgs := []OrgInfo{
		{ID: "1", Name: "alpha", Role: "owner"},
		{ID: "2", Name: "long-named-org", Role: "member"},
	}
	var buf bytes.Buffer
	PrintOrgList(&buf, orgs, false)

	got := buf.String()
	wantLines := []string{
		"NAME            ROLE",
		"alpha           owner",
		"long-named-org  member",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("missing aligned line %q in:\n%s", line, got)
		}
	}
}

func TestPrintOrgList_MarksCurrentWithGreenSuffix(t *testing.T) {
	orgs := []OrgInfo{
		{ID: "1", Name: "alpha", Role: "owner"},
		{ID: "2", Name: "beta", Role: "member", Current: true},
	}
	var buf bytes.Buffer
	PrintOrgList(&buf, orgs, true)

	got := buf.String()
	if !strings.Contains(got, "beta") {
		t.Fatalf("beta row missing: %q", got)
	}
	// Green ANSI wrap on "(current)".
	if !strings.Contains(got, "\x1b[32m(current)\x1b[0m") {
		t.Errorf("expected green-wrapped (current) marker on the active row; got:\n%s", got)
	}
	// Inactive rows must not carry the marker.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "alpha") && strings.Contains(line, "(current)") {
			t.Errorf("alpha is inactive; should not have (current): %q", line)
		}
	}
}

func TestPrintOrgList_ColorOff_PlainCurrent(t *testing.T) {
	orgs := []OrgInfo{
		{ID: "1", Name: "alpha", Role: "owner", Current: true},
	}
	var buf bytes.Buffer
	PrintOrgList(&buf, orgs, false)

	got := buf.String()
	if strings.Contains(got, "\x1b[") {
		t.Errorf("colour off: must not emit ANSI escape; got %q", got)
	}
	if !strings.Contains(got, "(current)") {
		t.Errorf("colour off: still want plain (current) suffix; got %q", got)
	}
}

func TestPrintOrgList_EmptyInputEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	PrintOrgList(&buf, nil, false)
	if buf.Len() != 0 {
		t.Errorf("empty input should produce no output (no orphan header); got %q", buf.String())
	}
}
