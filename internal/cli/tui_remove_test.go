package cli

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRemovePipe(t *testing.T) {
	m := newTUIModel("me", "s", "/tmp/x.sock", make(chan tea.Msg, 8), context.Background())
	m.agents = append(m.agents, tItem{kind: kAgent, key: "alice", label: "alice"})
	cancelled := map[string]bool{}
	for _, name := range []string{"room-1", "status"} {
		n := name
		m.pipes = append(m.pipes, tItem{kind: kPipe, key: n, label: n})
		m.followed[n] = true
		m.pipeCancels[n] = func() { cancelled[n] = true }
	}
	// select the first pipe: flat index = len(agents)+0
	m.sel = len(m.agents)
	m.removePipe(m.sel)

	if len(m.pipes) != 1 || m.pipes[0].key != "status" {
		t.Fatalf("room-1 not removed: %+v", m.pipes)
	}
	if !cancelled["room-1"] {
		t.Errorf("follow not cancelled for removed pipe")
	}
	if m.followed["room-1"] || m.pipeCancels["room-1"] != nil {
		t.Errorf("bookkeeping not cleaned: followed=%v cancel=%v", m.followed["room-1"], m.pipeCancels["room-1"])
	}
	if m.sel < 0 || m.sel >= m.count() {
		t.Errorf("selection out of range after remove: sel=%d count=%d", m.sel, m.count())
	}

	// removePipe on an agent row is a no-op
	before := len(m.pipes)
	m.removePipe(0)
	if len(m.pipes) != before {
		t.Errorf("removePipe on an agent index mutated pipes")
	}
}
