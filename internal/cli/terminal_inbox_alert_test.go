package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

func TestTerminalInboxAlertStateMachineDefersWhileUserActive(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	sm := newTerminalInboxAlertStateMachine(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	})

	sm.ObserveUserInput(now, []byte("partial prompt"))
	sm.ObserveInboxUnread(now.Add(time.Second))

	if got := sm.ReadyAlert(now.Add(14 * time.Second)); got != "" {
		t.Fatalf("ReadyAlert while user active = %q, want empty", got)
	}
}

func TestTerminalInboxAlertStateMachineInjectsAfterIdle(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	sm := newTerminalInboxAlertStateMachine(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	})

	sm.ObserveUserInput(now, []byte("partial prompt"))
	sm.ObserveInboxUnread(now.Add(time.Second))

	got := sm.ReadyAlert(now.Add(16 * time.Second))
	if !strings.Contains(got, "You have received a message") {
		t.Fatalf("ReadyAlert after idle = %q, want inbox alert text", got)
	}
	if !strings.Contains(got, "ppz read inbox") {
		t.Fatalf("ReadyAlert after idle = %q, want ppz read inbox guidance", got)
	}
}

func TestTerminalInboxAlertStateMachineCoalescesMultipleUnreadMessages(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	sm := newTerminalInboxAlertStateMachine(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	})

	sm.ObserveInboxUnread(now)
	sm.ObserveInboxUnread(now.Add(time.Second))
	sm.ObserveInboxUnread(now.Add(2 * time.Second))

	first := sm.ReadyAlert(now.Add(16 * time.Second))
	second := sm.ReadyAlert(now.Add(17 * time.Second))

	if first == "" {
		t.Fatal("first ReadyAlert is empty, want one coalesced alert")
	}
	if second != "" {
		t.Fatalf("second ReadyAlert = %q, want empty after coalescing", second)
	}
}

func TestTerminalInboxAlertPumpWritesInboxAlertToPTYStdinAfterIdle(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	}, &ptyStdin)

	pump.ObserveUserInput(now, []byte("half typed command"))
	pump.ObserveInboxMessage(now.Add(time.Second), cliproto.ReadMessage{
		Handle:  "foo",
		Payload: "secret inbox payload",
	})

	if wrote := pump.Flush(now.Add(14 * time.Second)); wrote {
		t.Fatalf("Flush before idle wrote alert to PTY stdin: %q", ptyStdin.String())
	}
	if ptyStdin.Len() != 0 {
		t.Fatalf("PTY stdin before idle = %q, want empty", ptyStdin.String())
	}

	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("Flush after idle did not write alert to PTY stdin")
	}
	got := ptyStdin.String()
	if !strings.Contains(got, "You have received a message") {
		t.Fatalf("PTY stdin alert = %q, want inbox alert text", got)
	}
	if !strings.Contains(got, "ppz read inbox") {
		t.Fatalf("PTY stdin alert = %q, want ppz read inbox guidance", got)
	}
	if strings.Contains(got, "secret inbox payload") {
		t.Fatalf("PTY stdin alert leaked inbox payload: %q", got)
	}
	if wrote := pump.Flush(now.Add(17 * time.Second)); wrote {
		t.Fatalf("second Flush wrote duplicate alert to PTY stdin: %q", ptyStdin.String())
	}
}

func TestTerminalInboxAlertPumpCoalescesInboxMessagesIntoOnePTYAlert(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	}, &ptyStdin)

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Handle: "foo", Payload: "one"})
	pump.ObserveInboxMessage(now.Add(time.Second), cliproto.ReadMessage{Handle: "foo", Payload: "two"})
	pump.ObserveInboxMessage(now.Add(2*time.Second), cliproto.ReadMessage{Handle: "foo", Payload: "three"})

	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("Flush after idle did not write coalesced alert")
	}
	first := ptyStdin.String()
	if strings.Count(first, "You have received a message") != 1 {
		t.Fatalf("PTY stdin after coalesced alert = %q, want exactly one alert", first)
	}
	if wrote := pump.Flush(now.Add(17 * time.Second)); wrote {
		t.Fatalf("second Flush wrote duplicate coalesced alert: %q", ptyStdin.String())
	}
}

func TestTerminalInboxAlertPumpBuffersUserInputDuringAlertMode(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
	}, &ptyStdin)

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Handle: "foo"})
	pump.BeginAlertMode(now.Add(16 * time.Second))

	if forwarded := pump.ForwardUserInput(now.Add(16*time.Second), []byte("typed during alert")); forwarded {
		t.Fatalf("ForwardUserInput returned true during alert mode; user input should be buffered")
	}
	if strings.Contains(ptyStdin.String(), "typed during alert") {
		t.Fatalf("PTY stdin received user input during alert mode: %q", ptyStdin.String())
	}

	pump.EndAlertMode(now.Add(17 * time.Second))
	if !strings.Contains(ptyStdin.String(), "typed during alert") {
		t.Fatalf("PTY stdin after alert mode = %q, want buffered user input flushed", ptyStdin.String())
	}
}

func TestTerminalInboxAlertPumpCooldownSuppressesImmediateRepeatedAlerts(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Cooldown:  30 * time.Second,
		Message:   terminalInboxAlertMessage,
	}, &ptyStdin)

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Handle: "foo"})
	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("first Flush did not write alert")
	}

	pump.ObserveInboxMessage(now.Add(17*time.Second), cliproto.ReadMessage{Handle: "foo"})
	if wrote := pump.Flush(now.Add(20 * time.Second)); wrote {
		t.Fatalf("Flush during cooldown wrote repeated alert: %q", ptyStdin.String())
	}

	if wrote := pump.Flush(now.Add(47 * time.Second)); !wrote {
		t.Fatal("Flush after cooldown did not write pending repeated alert")
	}
	if strings.Count(ptyStdin.String(), "You have received a message") != 2 {
		t.Fatalf("PTY stdin after cooldown = %q, want two total alerts", ptyStdin.String())
	}
}
