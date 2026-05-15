package server

import (
	"bytes"
	"strings"
	"testing"
)

// Phase 2 follow-up: empty-dashboard onboarding.
//
// When a freshly-logged-in user has no pipes anywhere (in any org
// they own/belong to), the dashboard renders a get-started panel
// with onboarding instructions instead of an empty Organisations
// list. Once the user creates a pipe the panel hides itself.

// onboardingMarker is the data-attribute the template uses to mark
// the onboarding section. Tests grep for it; UI integration tests
// (browser) would also key off it. Keeping it as a constant lets
// the test + template stay in sync without string duplication risk.
const onboardingMarker = `data-onboarding="empty"`

// TestDashboard_ShowsOnboardingWhenNoPipes — the dashboard template
// renders the onboarding section when `.HasNoPipes` is true (the
// handler sets this when the user owns/belongs to no orgs OR has
// owned orgs that contain zero pipes).
func TestDashboard_ShowsOnboardingWhenNoPipes(t *testing.T) {
	data := map[string]any{
		"Invites":    nil,
		"Orgs":       nil,
		"HasNoPipes": true,
		"Version":    "v-test",
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "index.html", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, onboardingMarker) {
		t.Errorf("dashboard body missing %q when HasNoPipes=true", onboardingMarker)
	}
	for _, hint := range []string{
		"ppz source create",
		"ppz pipe create",
	} {
		if !strings.Contains(body, hint) {
			t.Errorf("onboarding body missing CLI hint %q", hint)
		}
	}
}

// TestDashboard_HidesOnboardingWhenHasPipes — the onboarding panel
// must NOT render when `.HasNoPipes` is false. Prevents the panel
// leaking into the "established user" path where it's just noise.
func TestDashboard_HidesOnboardingWhenHasPipes(t *testing.T) {
	data := map[string]any{
		"Invites":    nil,
		"Orgs":       nil,
		"HasNoPipes": false,
		"Version":    "v-test",
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "index.html", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	body := buf.String()
	if strings.Contains(body, onboardingMarker) {
		t.Errorf("dashboard body should NOT contain %q when HasNoPipes=false", onboardingMarker)
	}
}
