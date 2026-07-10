package cli

import "testing"

// Guards the hand-computed geometry in itemAtY/isAddY against renderMenu, which
// has no other test. Three sections; layout (screen rows):
// title(0) box-top(1) "AGENTS"(2) agents(3..) blank "INBOXES" sources blank
// "PIPES" pipes blank "[+ add pipe]".
func TestMenuHitGeometry(t *testing.T) {
	m := tuiModel{w: 100, h: 30}
	m.agents = []tItem{{kind: kAgent, key: "alice"}, {kind: kAgent, key: "bob"}}
	m.sources = []tItem{{kind: kSource, key: "laurent"}}
	m.pipes = []tItem{{kind: kPipe, key: "room-1"}}

	cases := []struct {
		y, want int // want = flat index, or -1
	}{
		{0, -1}, {1, -1}, {2, -1}, // title, box border, AGENTS header
		{3, 0}, {4, 1}, // alice, bob
		{5, -1}, {6, -1}, // blank, INBOXES header
		{7, 2},           // laurent (source, flat index 2)
		{8, -1}, {9, -1}, // blank, PIPES header
		{10, 3},  // room-1 (pipe, flat index 3)
		{11, -1}, // blank before [+ add pipe]
	}
	for _, c := range cases {
		if got := m.itemAtY(c.y, 1); got != c.want {
			t.Errorf("itemAtY(y=%d) = %d, want %d", c.y, got, c.want)
		}
	}
	if !m.isAddY(12, 1) { // [+ add pipe] at 8 + agents(2) + sources(1) + pipes(1) = 12
		t.Errorf("isAddY(12) = false, want true")
	}
	if m.isAddY(10, 1) {
		t.Errorf("isAddY(10) matched a pipe row")
	}
	// clicks past the menu column are ignored
	if got := m.itemAtY(3, m.menuW()); got != -1 {
		t.Errorf("click at menu edge should miss, got %d", got)
	}
}
