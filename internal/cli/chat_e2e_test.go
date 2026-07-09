package cli

// End-to-end coverage of `ppz chat`'s daemon boundary: a fake daemon over a
// real unix socket, driven through the actual background goroutines
// (whoPoller, streamRead) and the real model.Update event routing, plus a real
// send round-trip. This exercises the wire path the unit tests skip — IPC
// framing, ReadEvent streaming, sender-fanout routing, and the send request
// shape — without a TTY (bubbletea's runtime is stood in for by pumping the
// events channel into Update, exactly as the runtime would).

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pipescloud/ppz/internal/cliproto"
)

type fakeDaemon struct {
	whoEntries []cliproto.WhoEntry
	inbox      []cliproto.ReadMessage // streamed to an inbox follow
	sends      *recorder[cliproto.SendRequest]
	reads      *recorder[cliproto.ReadRequest]
	sendErr    bool // when set, IPCSend replies with an error
}

// startFakeDaemon serves the three IPC verbs `ppz chat` uses. IPCRead (a
// follow) streams the canned inbox then holds the connection open until
// teardown, so streamRead doesn't reconnect-and-replay mid-test.
func startFakeDaemon(t *testing.T, sock string, fd *fakeDaemon) {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen fake daemon: %v", err)
	}
	stop := make(chan struct{})
	t.Cleanup(func() {
		close(stop)
		_ = ln.Close()
		_ = os.Remove(sock)
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var req struct {
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if json.NewDecoder(conn).Decode(&req) != nil {
					return
				}
				enc := json.NewEncoder(conn)
				switch req.Method {
				case cliproto.IPCWho:
					_ = enc.Encode(map[string]any{"result": cliproto.WhoReply{Entries: fd.whoEntries}})
				case cliproto.IPCSend:
					var sr cliproto.SendRequest
					_ = json.Unmarshal(req.Params, &sr)
					fd.sends.add(sr)
					if fd.sendErr {
						_ = enc.Encode(map[string]any{"error": cliproto.New(cliproto.EDeliveryUnconfirmed)})
						break
					}
					_ = enc.Encode(map[string]any{"result": cliproto.SendReply{
						ID: "testid", Subject: sr.Handle + "." + sr.Channel, Bytes: len(sr.Payload),
					}})
				case cliproto.IPCRead:
					var rr cliproto.ReadRequest
					_ = json.Unmarshal(req.Params, &rr)
					fd.reads.add(rr)
					for i := range fd.inbox {
						m := fd.inbox[i]
						_ = enc.Encode(cliproto.ReadEvent{Message: &m})
					}
					<-stop // keep the follow open (no reconnect/replay)
				}
			}(conn)
		}
	}()
}

func TestChatE2E_RosterInboxAndSend(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ppz-chat-e2e-") // short path: macOS caps unix sockets at ~104 chars
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "daemon.sock")
	fd := &fakeDaemon{
		whoEntries: []cliproto.WhoEntry{
			{Handle: "alice", Payload: `{"interval_sec":60,"agent_state":"idle"}`, ArrivedAt: time.Now()},
			{Handle: "james", Payload: `{"interval_sec":60}`, ArrivedAt: time.Now()}, // self, must be filtered out
		},
		inbox: []cliproto.ReadMessage{
			{ID: "i1", Sender: "alice", Payload: "hey james", CreatedAt: "2026-07-09T09:00:00Z"},
			{ID: "i2", Sender: "alice", Payload: "you there?", CreatedAt: "2026-07-09T09:01:00Z"},
		},
		sends: &recorder[cliproto.SendRequest]{},
		reads: &recorder[cliproto.ReadRequest]{},
	}
	startFakeDaemon(t, sock, fd)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan tea.Msg, 256)
	go whoPoller(ctx, sock, events)
	go streamRead(ctx, sock,
		cliproto.ReadRequest{Handle: "james", Channel: "inbox", Follow: true, Session: "ppz-tui", Sender: "james"},
		func(rm cliproto.ReadMessage) tea.Msg { return inboundMsg{rm} }, events)

	var m tea.Model = newTUIModel("james", "ppz-tui", sock, events, ctx)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Pump events (as the runtime would) until alice's DM has both inbound
	// messages AND her roster status arrived, or we time out.
	deadline := time.After(3 * time.Second)
	for {
		stop := false
		select {
		case msg := <-events:
			m, _ = m.Update(msg)
		case <-deadline:
			stop = true
		}
		if a, ok := findAgent(m.(tuiModel), "alice"); ok && len(a.msgs) >= 2 && a.status == "online" {
			break
		}
		if stop {
			break
		}
	}

	tm := m.(tuiModel)
	if _, ok := findAgent(tm, "james"); ok {
		t.Errorf("self handle must be filtered out of the roster")
	}
	a, ok := findAgent(tm, "alice")
	if !ok {
		t.Fatal("alice not in roster after who + inbox follow")
	}
	if a.status != "online" {
		t.Errorf("alice status = %q, want online (from who)", a.status)
	}
	if len(a.msgs) != 2 {
		t.Fatalf("want 2 inbound msgs fanned out to alice's DM by sender, got %d", len(a.msgs))
	}
	if a.unread != 2 {
		t.Errorf("want unread 2, got %d", a.unread)
	}
	// The inbox follow went out with the right target.
	if fd.reads.count() == 0 {
		t.Fatal("no ReadRequest reached the daemon")
	}
	if rr := fd.reads.at(0); rr.Handle != "james" || rr.Channel != "inbox" || !rr.Follow {
		t.Errorf("inbox follow wire wrong: %+v", rr)
	}

	// Now a real send round-trip: open alice's DM, type, press enter → the
	// returned Cmd performs the actual IPCSend against the daemon.
	tm.sel = agentIndex(tm, "alice")
	tm.focus = fChat
	tm.chatTi.SetValue("hi alice")
	var after tea.Model = tm
	after, cmd := after.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter in a focused chat should return a send Cmd")
	}
	if res, ok := cmd().(sendResultMsg); !ok || res.err != "" {
		t.Fatalf("send Cmd should succeed against the fake daemon: ok=%v err=%q", ok, res.err)
	}

	if fd.sends.count() != 1 {
		t.Fatalf("want exactly 1 send recorded, got %d", fd.sends.count())
	}
	sr := fd.sends.at(0)
	if sr.Handle != "alice" || sr.Channel != "inbox" || sr.Payload != "hi alice" {
		t.Errorf("send wire wrong: %+v", sr)
	}
	if sr.BareTarget != "alice" || sr.Sender != "james" {
		t.Errorf("send bare/sender wrong: bare=%q sender=%q", sr.BareTarget, sr.Sender)
	}
	// The outbound is locally echoed into the DM (never read back from the wire).
	if a2, ok := findAgent(after.(tuiModel), "alice"); !ok || len(a2.msgs) != 3 {
		t.Errorf("outbound not echoed into alice's DM: msgs=%d", len(a2.msgs))
	}
}

func findAgent(m tuiModel, name string) (tItem, bool) {
	for _, a := range m.agents {
		if a.key == name {
			return a, true
		}
	}
	return tItem{}, false
}

func agentIndex(m tuiModel, name string) int {
	for i, a := range m.agents {
		if a.key == name {
			return i
		}
	}
	return -1
}
