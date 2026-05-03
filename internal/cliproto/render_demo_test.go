package cliproto

import (
	"os"
	"testing"
)

// TestRender_DumpFixtureSnapshot writes the rendered fixture to
// /tmp/ppz-fixture-rendered.txt so we can eyeball it. Skipped unless
// PPZ_RENDER_DEMO=1 to keep regular `go test` output clean.
func TestRender_DumpFixtureSnapshot(t *testing.T) {
	if os.Getenv("PPZ_RENDER_DEMO") != "1" {
		t.Skip("set PPZ_RENDER_DEMO=1 to dump rendered fixture")
	}
	in := loadFixture(t)
	out := RenderTerminal(in, DefaultRenderCols, DefaultRenderRows)
	if err := os.WriteFile("/tmp/ppz-fixture-rendered.txt", []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes to /tmp/ppz-fixture-rendered.txt", len(out))
}
