package server

import (
	"strings"
	"testing"
)

// Mobile responsiveness — every served HTML template needs the
// width=device-width viewport meta tag, otherwise mobile browsers
// fall back to a ~980px virtual viewport and shrink the page to
// fit. style.css already has media-query rules for the marketing
// landing page; the admin-page rules land alongside this test.
//
// Surfaces tested:
//   - All embedded templates carry the viewport meta tag.
//   - style.css contains admin-page responsive rules (nav-tab stack,
//     full-width forms, table reflow, padding adjustments).
//   - landing.html demo-pair responsive rules survive (existing pre-
//     Phase-2 CSS — regression guard).

const viewportTag = `<meta name="viewport" content="width=device-width, initial-scale=1">`

// TestTemplates_AllCarryViewportMeta — every *.html template the
// server serves must include the viewport meta tag. Without it,
// mobile browsers render at desktop scale + zoom out, which is the
// "tiny on mobile" symptom users were seeing.
func TestTemplates_AllCarryViewportMeta(t *testing.T) {
	files, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("templateFS.ReadDir: %v", err)
	}
	for _, f := range files {
		t.Run(f.Name(), func(t *testing.T) {
			data, err := templateFS.ReadFile("templates/" + f.Name())
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(string(data), `name="viewport"`) {
				t.Errorf("templates/%s missing viewport meta tag", f.Name())
			}
			if !strings.Contains(string(data), `width=device-width`) {
				t.Errorf("templates/%s viewport content missing width=device-width", f.Name())
			}
		})
	}
}

// TestStyleCSS_HasAdminResponsiveRules — style.css must contain
// @media rules that adjust the admin-page surfaces for narrow
// viewports. We check for selector-fragment landmarks so the test
// pins the *intent* (tab nav stacks, forms reflow, tables scroll)
// rather than exact byte-for-byte CSS, which would brittle-fail on
// every cosmetic change.
func TestStyleCSS_HasAdminResponsiveRules(t *testing.T) {
	data, err := assetsFS.ReadFile("assets/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	css := string(data)

	// A mobile breakpoint exists for the admin surface. The marketing
	// landing already has @media (max-width: 720px); the admin one can
	// reuse the same breakpoint or pick a different one — just needs to
	// exist at narrow widths.
	if !strings.Contains(css, "@media") {
		t.Fatal("style.css has no @media rules at all")
	}

	// Specific responsive intents the admin pages need. These are
	// fragments that should appear inside an @media block; the test
	// doesn't enforce which block, just that the responsive rule is
	// somewhere in the file.
	for _, fragment := range []string{
		// Stacked tab nav: the .tabs flexbox should switch direction
		// on narrow widths so account-page tabs don't overflow.
		".tabs",
		// Forms scoped to .panel-form (login_password.html) should
		// pick up a full-width treatment.
		".panel-form",
		// The pipes table on the account page needs to scroll
		// horizontally on narrow viewports instead of overflowing.
		"#pipes",
	} {
		if !strings.Contains(css, fragment) {
			t.Errorf("style.css missing responsive selector landmark %q", fragment)
		}
	}
}

// TestStyleCSS_LandingResponsiveRulesPresent — regression guard.
// The @media (max-width: 720px) block now covers the admin pages
// (the landing demos that originally seeded it were stripped — they
// now live in pipes-internal). Test stays so a future change can't
// silently drop the breakpoint.
func TestStyleCSS_LandingResponsiveRulesPresent(t *testing.T) {
	data, err := assetsFS.ReadFile("assets/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	css := string(data)
	if !strings.Contains(css, "@media (max-width: 720px)") {
		t.Error("style.css missing the @media (max-width: 720px) block")
	}
}

// TestStyleCSS_LandingPaneTunedForNarrow removed — the landing demo
// panes it pinned have been deleted along with the marketing CSS
// chunk (the demos now live in pipes-internal). No .pane selector
// to tune on phone-width viewports anymore.
