package cli

// e2e coverage that `ppz chat` persists through the chatstore: drive the real
// chat flow (fake daemon → follow goroutines → model.Update, plus real sends),
// then assert the on-disk store reflects it after a flush + reopen ("restart").
//
// These are RED until the store is wired into the model (ingest on
// inbound/pipe/send, MarkRead on open, AddPipe on add, hydrate on launch). The
// chat model has a `store` field but doesn't populate it yet, so every store
// assertion below fails today — which is the point.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pipescloud/ppz/internal/chatstore"
	"github.com/pipescloud/ppz/internal/cliproto"
)

func setupChat(t *testing.T, inbox []cliproto.ReadMessage, who []cliproto.WhoEntry) (home, sock string, store *chatstore.Store) {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "ppz-chat-store-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	sock = filepath.Join(home, "d.sock")
	startFakeDaemon(t, sock, &fakeDaemon{
		whoEntries: who, inbox: inbox,
		sends: &recorder[cliproto.SendRequest]{}, reads: &recorder[cliproto.ReadRequest]{},
	})
	store, err = chatstore.Open(home, "james")
	if err != nil {
		t.Fatal(err)
	}
	return home, sock, store
}

func startInboxFollow(ctx context.Context, sock string, events chan tea.Msg) {
	go whoPoller(ctx, sock, events)
	go streamRead(ctx, sock,
		cliproto.ReadRequest{Handle: "james", Channel: "inbox", Follow: true, Session: "ppz-tui", Sender: "james"},
		func(rm cliproto.ReadMessage) tea.Msg { return inboundMsg{rm} }, events)
}

func chatModel(sock string, store *chatstore.Store, ctx context.Context, events chan tea.Msg) tea.Model {
	tm := newTUIModel("james", "ppz-tui", sock, events, ctx)
	tm.store = store
	var m tea.Model = tm
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func pumpUntil(m tea.Model, events chan tea.Msg, cond func(tuiModel) bool) tea.Model {
	deadline := time.After(3 * time.Second)
	for {
		if cond(m.(tuiModel)) {
			return m
		}
		select {
		case msg := <-events:
			m, _ = m.Update(msg)
		case <-deadline:
			return m
		}
	}
}

func openChat(t *testing.T, m tea.Model, agent string) tea.Model {
	t.Helper()
	tm := m.(tuiModel)
	idx := agentIndex(tm, agent)
	if idx < 0 {
		t.Fatalf("agent %q not in roster", agent)
	}
	tm.sel = idx
	var mm tea.Model = tm
	mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open → mark read + focus chat
	return mm
}

func sendChat(m tea.Model, text string) tea.Model {
	tm := m.(tuiModel)
	tm.chatTi.SetValue(text)
	var mm tea.Model = tm
	mm, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd() // performs the real IPCSend
	}
	return mm
}

func msgCount(a tuiModel, name string) int {
	if it, ok := findAgent(a, name); ok {
		return len(it.msgs)
	}
	return 0
}

// History — received messages and my own sent reply survive a restart.
func TestChatStoreE2E_HistoryPersists(t *testing.T) {
	home, sock, store := setupChat(t,
		[]cliproto.ReadMessage{
			{ID: "i1", Sender: "alice", Payload: "hey", CreatedAt: "2026-07-09T09:00:00Z"},
			{ID: "i2", Sender: "alice", Payload: "you there?", CreatedAt: "2026-07-09T09:01:00Z"},
		},
		[]cliproto.WhoEntry{{Handle: "alice", Payload: `{"interval_sec":60}`, ArrivedAt: time.Now()}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 256)
	startInboxFollow(ctx, sock, events)

	m := chatModel(sock, store, ctx, events)
	m = pumpUntil(m, events, func(a tuiModel) bool { return msgCount(a, "alice") >= 2 })
	m = openChat(t, m, "alice")
	_ = sendChat(m, "hi alice")
	cancel()
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	reopened, err := chatstore.Open(home, "james")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reopened.Messages(chatstore.KindAgent, "alice")
	if len(got) != 3 {
		t.Fatalf("want 3 persisted messages (2 received + my sent reply), got %d", len(got))
	}
	var sawSent bool
	for _, mm := range got {
		if mm.Dir == chatstore.DirOut && mm.Payload == "hi alice" {
			sawSent = true
		}
	}
	if !sawSent {
		t.Errorf("my sent reply was not persisted")
	}
}

// Read markers — a DM I opened stays read across a restart; one I didn't stays
// unread. This is the "everything unread again on restart" bug.
func TestChatStoreE2E_ReadMarkersPersist(t *testing.T) {
	home, sock, store := setupChat(t,
		[]cliproto.ReadMessage{
			{ID: "a1", Sender: "alice", Payload: "1", CreatedAt: "2026-07-09T09:00:00Z"},
			{ID: "a2", Sender: "alice", Payload: "2", CreatedAt: "2026-07-09T09:01:00Z"},
			{ID: "b1", Sender: "bob", Payload: "1", CreatedAt: "2026-07-09T09:02:00Z"},
			{ID: "b2", Sender: "bob", Payload: "2", CreatedAt: "2026-07-09T09:03:00Z"},
		},
		[]cliproto.WhoEntry{
			{Handle: "alice", Payload: `{"interval_sec":60}`, ArrivedAt: time.Now()},
			{Handle: "bob", Payload: `{"interval_sec":60}`, ArrivedAt: time.Now()},
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 256)
	startInboxFollow(ctx, sock, events)

	m := chatModel(sock, store, ctx, events)
	m = pumpUntil(m, events, func(a tuiModel) bool { return msgCount(a, "alice") >= 2 && msgCount(a, "bob") >= 2 })
	m = openChat(t, m, "alice") // read alice only; leave bob unread
	_ = m
	cancel()
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	reopened, _ := chatstore.Open(home, "james")
	if got, _ := reopened.Messages(chatstore.KindAgent, "alice"); len(got) != 2 {
		t.Fatalf("alice history not persisted: got %d messages", len(got))
	}
	if u, _ := reopened.Unread(chatstore.KindAgent, "alice"); u != 0 {
		t.Errorf("alice was read before restart, want 0 unread, got %d", u)
	}
	if u, _ := reopened.Unread(chatstore.KindAgent, "bob"); u != 2 {
		t.Errorf("bob was never opened, want 2 unread after restart, got %d", u)
	}
}

// Added pipes survive a restart.
func TestChatStoreE2E_AddedPipePersists(t *testing.T) {
	home, sock, store := setupChat(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan tea.Msg, 256)

	tm := chatModel(sock, store, ctx, events).(tuiModel)
	tm.addPipe("room-1")
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	reopened, _ := chatstore.Open(home, "james")
	ws, _ := reopened.Windows()
	found := false
	for _, w := range ws {
		if w.Kind == chatstore.KindPipe && w.Name == "room-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added pipe room-1 not persisted; windows=%+v", ws)
	}
}
