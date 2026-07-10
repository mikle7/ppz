package cli

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pipescloud/ppz/internal/chatstore"
	"github.com/pipescloud/ppz/internal/cliproto"
)

func menuHasLoadingSquare(m tuiModel) bool {
	menu := ansi.Strip(m.renderMenu(m.menuW(), m.h-2))
	for _, ln := range strings.Split(menu, "\n") {
		if strings.Contains(ln, "INBOXES") {
			return strings.ContainsAny(ln, "◰◳◲◱")
		}
	}
	return false
}

func findSource(m tuiModel, name string) (tItem, bool) {
	for _, s := range m.sources {
		if s.key == name {
			return s, true
		}
	}
	return tItem{}, false
}

func sourceKeys(m tuiModel) []string {
	out := make([]string, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, s.key)
	}
	return out
}

func sourceFlatIndex(m tuiModel, name string) int {
	for j, s := range m.sources {
		if s.key == name {
			return len(m.agents) + j
		}
	}
	return -1
}

func newInboxModel(t *testing.T) tuiModel {
	t.Helper()
	m := newTUIModel("james", "s", "/tmp/x.sock", make(chan tea.Msg, 8), context.Background())
	var mm tea.Model = m
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return mm.(tuiModel)
}

// A message source's chat title renders as a DM, never as a pipe.
func TestChatTitle_ByKind(t *testing.T) {
	if got := tChatTitle(tItem{kind: kSource, key: "laurent", label: "laurent"}); strings.Contains(got, "pipe") || !strings.Contains(got, "dm") {
		t.Errorf("a message source must render as a DM, not a pipe: %q", got)
	}
	if got := tChatTitle(tItem{kind: kPipe, label: "room"}); !strings.Contains(got, "pipe") {
		t.Errorf("a pipe should render as a pipe: %q", got)
	}
	if got := tChatTitle(tItem{kind: kAgent, label: "alice", status: "online"}); !strings.Contains(got, "dm") {
		t.Errorf("an agent should render as a DM: %q", got)
	}
}

// INBOXES shows a loading square while the source list is fetching, and it
// clears once loaded.
func TestInboxes_LoadingSquare(t *testing.T) {
	m := newInboxModel(t) // sourcesLoaded == false
	if !menuHasLoadingSquare(m) {
		t.Error("expected a loading square in the INBOXES header while sources load")
	}
	m.sourcesLoaded = true
	if menuHasLoadingSquare(m) {
		t.Error("loading square should clear once sources are loaded")
	}
}

// The source list populates INBOXES with message-kind sources only, excluding
// pty sources (those are AGENTS) and yourself.
func TestApplySources_PopulatesInboxes(t *testing.T) {
	m := newInboxModel(t)
	m.applySources([]cliproto.Source{
		{Handle: "laurent", Kind: "message"},
		{Handle: "shraddha", Kind: "message"},
		{Handle: "alice", Kind: "pty"},     // an agent, not an inbox source
		{Handle: "james", Kind: "message"}, // self, excluded
	})
	got := sourceKeys(m)
	want := map[string]bool{"laurent": true, "shraddha": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Fatalf("INBOXES = %v, want laurent+shraddha only", got)
	}
	if _, ok := findAgent(m, "alice"); ok {
		t.Errorf("a pty source must not be added to INBOXES logic as an agent here")
	}
}

// An inbound DM from a known message source lands in INBOXES, not AGENTS.
func TestRouteInbound_MessageSourceGoesToInboxes(t *testing.T) {
	m := newInboxModel(t)
	m.applySources([]cliproto.Source{{Handle: "laurent", Kind: "message"}})
	m.routeInbound(cliproto.ReadMessage{ID: "m1", Sender: "laurent", Payload: "hi", CreatedAt: "2026-07-10T09:00:00Z"})

	if _, ok := findAgent(m, "laurent"); ok {
		t.Errorf("laurent (message source) must not appear in AGENTS")
	}
	s, ok := findSource(m, "laurent")
	if !ok {
		t.Fatalf("laurent's DM not routed to INBOXES")
	}
	if len(s.msgs) != 1 || s.unread != 1 {
		t.Errorf("laurent DM not recorded in INBOXES: msgs=%d unread=%d", len(s.msgs), s.unread)
	}
}

// An inbound DM from a handle that isn't a known message source still defaults
// to AGENTS (unchanged behavior).
func TestRouteInbound_UnknownSenderStaysInAgents(t *testing.T) {
	m := newInboxModel(t)
	m.applySources([]cliproto.Source{{Handle: "laurent", Kind: "message"}})
	m.routeInbound(cliproto.ReadMessage{ID: "a1", Sender: "alice", Payload: "yo", CreatedAt: "2026-07-10T09:00:00Z"})

	if _, ok := findAgent(m, "alice"); !ok {
		t.Errorf("an unknown sender should default to AGENTS")
	}
	if _, ok := findSource(m, "alice"); ok {
		t.Errorf("alice must not appear in INBOXES")
	}
}

// Sending to a message source behaves like an agent DM: optimistic echo into
// the source window, and the send targets <handle>.inbox as a KindSource
// outbound (its reply comes back on our inbox, never via a follow).
func TestSend_ToMessageSource_EchoesAndPersistsAsSource(t *testing.T) {
	store, err := chatstore.Open(t.TempDir(), "james")
	if err != nil {
		t.Fatal(err)
	}
	m := newInboxModel(t)
	m.store = store
	m.applySources([]cliproto.Source{{Handle: "laurent", Kind: "message"}})
	idx := sourceFlatIndex(m, "laurent")
	if idx < 0 {
		t.Fatal("laurent not in INBOXES after applySources")
	}
	m.sel = idx
	m.focus = fChat
	m.chatTi.SetValue("hi laurent")

	var mm tea.Model = m
	mm, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter in a focused source chat should return a send Cmd")
	}
	s, ok := findSource(mm.(tuiModel), "laurent")
	if !ok || len(s.msgs) != 1 || !s.msgs[0].you {
		t.Fatalf("outbound not echoed into the source DM: %+v", s)
	}
	res, ok := cmd().(sendResultMsg)
	if !ok {
		t.Fatalf("send should return a sendResultMsg")
	}
	if res.kind != chatstore.KindSource || res.name != "laurent" {
		t.Fatalf("send should target a KindSource DM to laurent, got kind=%q name=%q", res.kind, res.name)
	}

	// The real Cmd would hit the daemon; with no daemon here it errors, so feed
	// a *successful* result of the same kind through Update to exercise the
	// persist-on-success branch (the bug: it only fired for KindAgent).
	out := chatstore.Message{ID: "o1", Dir: chatstore.DirOut, Sender: "james", Payload: "hi laurent", CreatedAt: "2026-07-10T09:00:00Z"}
	mm, _ = mm.Update(sendResultMsg{kind: chatstore.KindSource, name: "laurent", msg: out})

	// The outbound must be persisted under KindSource (it can't be re-hydrated
	// from the wire), so it survives a restart.
	got, _ := store.Messages(chatstore.KindSource, "laurent")
	if len(got) != 1 || got[0].Dir != chatstore.DirOut || got[0].Payload != "hi laurent" {
		t.Fatalf("outbound not persisted as KindSource: %+v", got)
	}
}

// A DM from a source that arrives before it's classified lands in AGENTS (and
// is stored as KindAgent); once classified, upsertSource must migrate it in the
// store too — not just the model — so a restart doesn't rebuild two windows.
func TestUpsertSource_ReconcilesAgentWindowIntoSource(t *testing.T) {
	home := t.TempDir()
	store, err := chatstore.Open(home, "james")
	if err != nil {
		t.Fatal(err)
	}
	m := newInboxModel(t)
	m.store = store

	// Inbound arrives before classification → AGENTS + stored as KindAgent.
	m.routeInbound(cliproto.ReadMessage{ID: "x1", Sender: "laurent", Payload: "early", CreatedAt: "2026-07-10T09:00:00Z"})
	if _, ok := findAgent(m, "laurent"); !ok {
		t.Fatal("precondition: laurent should be in AGENTS before classification")
	}

	// Now classify → migrate model AND store.
	m.applySources([]cliproto.Source{{Handle: "laurent", Kind: "message"}})
	if _, ok := findAgent(m, "laurent"); ok {
		t.Error("laurent should have left AGENTS after migration")
	}
	if _, ok := findSource(m, "laurent"); !ok {
		t.Error("laurent should be in INBOXES after migration")
	}
	if got, _ := store.Messages(chatstore.KindAgent, "laurent"); len(got) != 0 {
		t.Errorf("store still holds a KindAgent window after migration: %d msgs", len(got))
	}
	if got, _ := store.Messages(chatstore.KindSource, "laurent"); len(got) != 1 {
		t.Errorf("store KindSource window should hold the migrated message, got %d", len(got))
	}

	// A fresh reopen must see laurent once (as a source), never split across
	// AGENTS and INBOXES.
	store.Flush()
	s2, _ := chatstore.Open(home, "james")
	if got, _ := s2.Messages(chatstore.KindAgent, "laurent"); len(got) != 0 {
		t.Errorf("restart: stray KindAgent window persisted: %d msgs", len(got))
	}
	if got, _ := s2.Messages(chatstore.KindSource, "laurent"); len(got) != 1 {
		t.Errorf("restart: KindSource window missing/wrong: %d msgs", len(got))
	}
}
