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
	if !strings.Contains(got, "Please run 'ppz read inbox' and action messages") {
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

func TestTerminalInboxAlertPumpWritesClaudeSubmittedInboxAlertToPTYStdinAfterIdle(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
		Harness:   "claude",
	}, &ptyStdin)

	pump.ObserveUserInput(now, []byte("half typed command"))
	pump.ObserveInboxMessage(now.Add(time.Second), cliproto.ReadMessage{
		Sender:  "foo",
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
	if !strings.HasPrefix(got, "Please run 'ppz read inbox' and action messages") {
		t.Fatalf("PTY stdin alert = %q, want plain Claude instruction", got)
	}
	if !strings.HasSuffix(got, "\x1b[13u") {
		t.Fatalf("PTY stdin alert = %q, want Claude CSI-Enter-terminated instruction", got)
	}
	if strings.Contains(got, "ppz alert") {
		t.Fatalf("PTY stdin alert = %q, should not include ppz alert wrapper", got)
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

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Sender: "foo", Payload: "one"})
	pump.ObserveInboxMessage(now.Add(time.Second), cliproto.ReadMessage{Sender: "foo", Payload: "two"})
	pump.ObserveInboxMessage(now.Add(2*time.Second), cliproto.ReadMessage{Sender: "foo", Payload: "three"})

	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("Flush after idle did not write coalesced alert")
	}
	first := ptyStdin.String()
	if strings.Count(first, "Please run 'ppz read inbox' and action messages") != 1 {
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

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Sender: "foo"})
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

// TestSubmitInputForHarness_Claude pins today's behaviour: the
// claude branch must return `\x1b[13u` (kitty keyboard protocol
// Enter) so Claude Code's REPL submits the alert as a clean user
// turn instead of leaving the literal escape sequence in its input
// buffer.
func TestSubmitInputForHarness_Claude(t *testing.T) {
	got := submitInputForHarness("claude", "hello\n")
	if !strings.HasSuffix(got, "\x1b[13u") {
		t.Errorf("submitInputForHarness(\"claude\") = %q, want \\x1b[13u suffix (kitty keyboard Enter); claude's REPL relies on this to submit", got)
	}
	if !strings.HasPrefix(got, "hello") {
		t.Errorf("submitInputForHarness(\"claude\") = %q, want `hello` prefix (message preserved)", got)
	}
	if strings.HasSuffix(got, "\n\x1b[13u") || strings.HasSuffix(got, "\r\x1b[13u") {
		t.Errorf("submitInputForHarness(\"claude\") = %q, want trailing CR/LF stripped before the kitty terminator", got)
	}
}

// TestSubmitInputForHarness_NonClaudeUsesCarriageReturn pins the
// fix for the bug surfaced on copilot: every non-claude harness
// must get a plain `\r` terminator. The kitty-Enter escape
// (`\x1b[13u`) is only honoured by Claude Code; everyone else's
// REPL leaves the bytes in the input buffer literally, which is
// what made the alert visible as a `Please run 'ppz read inbox'
// and action messages` string sitting unsubmitted on copilot
// rather than triggering a turn. The empty/unknown harness arm
// is the safest default for non-agent `ppz terminal share` calls
// where PPZ_AGENT_HARNESS is unset.
func TestSubmitInputForHarness_NonClaudeUsesCarriageReturn(t *testing.T) {
	for _, h := range []string{"copilot", "codex", "agy", "pi", "", "bogus"} {
		t.Run(h, func(t *testing.T) {
			got := submitInputForHarness(h, "hello\n")
			if !strings.HasSuffix(got, "\r") {
				t.Errorf("submitInputForHarness(%q) = %q, want trailing `\\r` so the harness's REPL submits on plain carriage return (kitty Enter is claude-only)", h, got)
			}
			if strings.Contains(got, "\x1b[13u") {
				t.Errorf("submitInputForHarness(%q) = %q, must not contain kitty Enter escape — copilot/codex/agy/pi treat it as literal bytes and the alert lands unsubmitted in the input buffer", h, got)
			}
			if !strings.HasPrefix(got, "hello") {
				t.Errorf("submitInputForHarness(%q) = %q, want `hello` prefix (message preserved)", h, got)
			}
			if strings.HasSuffix(got, "\n\r") || strings.HasSuffix(got, "\r\r") {
				t.Errorf("submitInputForHarness(%q) = %q, want trailing CR/LF stripped before the carriage-return terminator", h, got)
			}
		})
	}
}

// TestTerminalInboxAlertPump_CopilotHarness_UsesCarriageReturnSubmit
// pins the integration: the pump must thread cfg.Harness through
// to its write callback so the on-PTY bytes carry the
// harness-appropriate submit terminator. Without the wiring,
// configuring Harness: "copilot" would still produce the kitty
// escape and copilot's input buffer keeps showing the literal
// alert message — exactly the user-observed bug.
func TestTerminalInboxAlertPump_CopilotHarness_UsesCarriageReturnSubmit(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	var ptyStdin bytes.Buffer
	pump := newTerminalInboxAlertPump(terminalInboxAlertConfig{
		IdleAfter: 15 * time.Second,
		Message:   terminalInboxAlertMessage,
		Harness:   "copilot",
	}, &ptyStdin)

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Sender: "foo"})
	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("Flush after idle did not write alert")
	}
	got := ptyStdin.String()
	if !strings.HasPrefix(got, "Please run 'ppz read inbox' and action messages") {
		t.Errorf("PTY stdin alert = %q, want plain alert text prefix", got)
	}
	if !strings.HasSuffix(got, "\r") {
		t.Errorf("PTY stdin alert = %q, want trailing `\\r` (copilot's REPL submits on carriage return; kitty Enter would leave the alert literal in the input buffer)", got)
	}
	if strings.Contains(got, "\x1b[13u") {
		t.Errorf("PTY stdin alert = %q, must not contain claude's kitty Enter escape on copilot harness", got)
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

	pump.ObserveInboxMessage(now, cliproto.ReadMessage{Sender: "foo"})
	if wrote := pump.Flush(now.Add(16 * time.Second)); !wrote {
		t.Fatal("first Flush did not write alert")
	}

	pump.ObserveInboxMessage(now.Add(17*time.Second), cliproto.ReadMessage{Sender: "foo"})
	if wrote := pump.Flush(now.Add(20 * time.Second)); wrote {
		t.Fatalf("Flush during cooldown wrote repeated alert: %q", ptyStdin.String())
	}

	if wrote := pump.Flush(now.Add(47 * time.Second)); !wrote {
		t.Fatal("Flush after cooldown did not write pending repeated alert")
	}
	if strings.Count(ptyStdin.String(), "Please run 'ppz read inbox' and action messages") != 2 {
		t.Fatalf("PTY stdin after cooldown = %q, want two total alerts", ptyStdin.String())
	}
}
