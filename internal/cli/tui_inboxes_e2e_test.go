package cli

// e2e for the INBOXES section: the real IPCList wire path (sourcePoller) plus
// the inbox follow and a send, all against a fake daemon over a unix socket.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestChatE2E_MessageSourceRosterAndDM(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-chat-src-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")

	fd := &fakeDaemon{
		whoEntries: []cliproto.WhoEntry{{Handle: "alice", Payload: `{"interval_sec":60}`, ArrivedAt: time.Now()}},
		sources: []cliproto.Source{
			{Handle: "laurent", Kind: "message"}, // → INBOXES
			{Handle: "alice", Kind: "pty"},       // an agent, not INBOXES
			{Handle: "james", Kind: "message"},   // self, excluded
		},
		inbox: []cliproto.ReadMessage{
			{ID: "l1", Sender: "laurent", Payload: "ping", CreatedAt: "2026-07-10T09:00:00Z"},
		},
		sends: &recorder[cliproto.SendRequest]{}, reads: &recorder[cliproto.ReadRequest]{},
	}
	startFakeDaemon(t, sock, fd)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan tea.Msg, 256)
	go whoPoller(ctx, sock, events)
	go sourcePoller(ctx, sock, "ppz-tui", events)
	go streamRead(ctx, sock,
		cliproto.ReadRequest{Handle: "james", Channel: "inbox", Follow: true, Session: "ppz-tui", Sender: "james"},
		func(rm cliproto.ReadMessage) tea.Msg { return inboundMsg{rm} }, events)

	var m tea.Model = newTUIModel("james", "ppz-tui", sock, events, ctx)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	deadline := time.After(3 * time.Second)
	for {
		stop := false
		select {
		case msg := <-events:
			m, _ = m.Update(msg)
		case <-deadline:
			stop = true
		}
		if s, ok := findSource(m.(tuiModel), "laurent"); ok && len(s.msgs) >= 1 {
			break
		}
		if stop {
			break
		}
	}

	tm := m.(tuiModel)
	if _, ok := findAgent(tm, "laurent"); ok {
		t.Error("laurent (message source) must not be in AGENTS")
	}
	if _, ok := findSource(tm, "alice"); ok {
		t.Error("alice (pty) must not be in INBOXES")
	}
	if _, ok := findSource(tm, "james"); ok {
		t.Error("self must be excluded from INBOXES")
	}
	s, ok := findSource(tm, "laurent")
	if !ok {
		t.Fatal("laurent not discovered into INBOXES via IPCList")
	}
	if len(s.msgs) != 1 || s.unread != 1 {
		t.Errorf("laurent DM not routed to INBOXES: msgs=%d unread=%d", len(s.msgs), s.unread)
	}

	// DM laurent: open + send → a real IPCSend targeting laurent.inbox.
	tm.sel = sourceFlatIndex(tm, "laurent")
	tm.focus = fChat
	tm.chatTi.SetValue("hi laurent")
	var after tea.Model = tm
	after, cmd := after.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a send Cmd")
	}
	if res, ok := cmd().(sendResultMsg); !ok || res.err != "" {
		t.Fatalf("send to source failed: ok=%v", ok)
	}
	if fd.sends.count() != 1 {
		t.Fatalf("want exactly 1 send, got %d", fd.sends.count())
	}
	if sr := fd.sends.at(0); sr.Handle != "laurent" || sr.Channel != "inbox" || sr.Payload != "hi laurent" {
		t.Errorf("send wire wrong: %+v", sr)
	}
}
