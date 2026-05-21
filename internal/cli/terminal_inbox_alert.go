package cli

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

const terminalInboxAlertMessage = "Please run 'ppz read inbox' and action messages\n"

type terminalInboxAlertConfig struct {
	IdleAfter time.Duration
	Cooldown  time.Duration
	Message   string
	// Harness identifies which agent harness the wrapped PTY is
	// running (one of "claude" / "copilot" / "codex" / "agy" /
	// "pi", or empty for non-agent shares). Used by
	// submitInputForHarness to pick the right submit-key byte
	// sequence — claude reads `\x1b[13u` (kitty keyboard protocol
	// Enter), other harnesses' REPLs treat that escape as literal
	// bytes and need a plain `\r` to submit.
	Harness string
}

type terminalInboxAlertStateMachine struct {
	mu            sync.Mutex
	cfg           terminalInboxAlertConfig
	pending       bool
	pendingSince  time.Time
	lastUserInput time.Time
	lastAlert     time.Time
}

func newTerminalInboxAlertStateMachine(cfg terminalInboxAlertConfig) *terminalInboxAlertStateMachine {
	if cfg.IdleAfter <= 0 {
		cfg.IdleAfter = 15 * time.Second
	}
	if cfg.Message == "" {
		cfg.Message = terminalInboxAlertMessage
	}
	return &terminalInboxAlertStateMachine{cfg: cfg}
}

func (s *terminalInboxAlertStateMachine) ObserveUserInput(now time.Time, input []byte) {
	if len(input) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUserInput = now
}

func (s *terminalInboxAlertStateMachine) ObserveInboxUnread(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pending {
		s.pendingSince = now
	}
	s.pending = true
}

func (s *terminalInboxAlertStateMachine) ReadyAlert(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready(now) {
		return ""
	}
	s.pending = false
	s.lastAlert = now
	return s.cfg.Message
}

func (s *terminalInboxAlertStateMachine) ready(now time.Time) bool {
	if !s.pending {
		return false
	}
	if !s.lastUserInput.IsZero() && now.Sub(s.lastUserInput) < s.cfg.IdleAfter {
		return false
	}
	if s.lastUserInput.IsZero() && !s.pendingSince.IsZero() && now.Sub(s.pendingSince) < s.cfg.IdleAfter {
		return false
	}
	if s.cfg.Cooldown > 0 && !s.lastAlert.IsZero() && now.Sub(s.lastAlert) < s.cfg.Cooldown {
		return false
	}
	return true
}

type terminalInboxAlertPump struct {
	mu     sync.Mutex
	sm     *terminalInboxAlertStateMachine
	pty    io.Writer
	write  func(string)
	alert  bool
	buffer []byte
}

func newTerminalInboxAlertPump(cfg terminalInboxAlertConfig, pty io.Writer) *terminalInboxAlertPump {
	if cfg.Message == "" {
		cfg.Message = terminalInboxAlertMessage
	}
	harness := cfg.Harness
	return &terminalInboxAlertPump{
		sm:  newTerminalInboxAlertStateMachine(cfg),
		pty: pty,
		write: func(message string) {
			_ = submitAlertToPTY(pty, harness, message, time.Sleep)
		},
	}
}

func newTerminalInboxAlertPumpForPTY(cfg terminalInboxAlertConfig, pty *os.File) *terminalInboxAlertPump {
	pump := newTerminalInboxAlertPump(cfg, pty)
	harness := cfg.Harness
	pump.write = func(message string) {
		restore := setPTYInputEcho(pty.Fd(), false)
		defer restore()
		_ = submitAlertToPTY(pty, harness, message, time.Sleep)
	}
	return pump
}

func (p *terminalInboxAlertPump) ObserveUserInput(now time.Time, input []byte) {
	p.sm.ObserveUserInput(now, input)
}

func (p *terminalInboxAlertPump) ObserveInboxMessage(now time.Time, msg cliproto.ReadMessage) {
	p.sm.ObserveInboxUnread(now)
}

func (p *terminalInboxAlertPump) Flush(now time.Time) bool {
	alert := p.sm.ReadyAlert(now)
	if alert == "" {
		return false
	}
	p.BeginAlertMode(now)
	p.write(alert)
	p.EndAlertMode(now)
	return true
}

// submitAlertToPTY writes the alert message followed by a
// harness-appropriate Enter-equivalent into w. Claude Code reads
// `\x1b[13u` (kitty keyboard protocol Enter) as a single key event,
// so we send the message and terminator in one write. Every other
// harness's REPL needs a plain `\r` to submit — but only when the CR
// arrives slightly after the message bytes: copilot and codex were
// observed treating the CR as a literal newline inside the line
// rather than a submit when it shipped in the same write burst as
// the message. `ppz command -cr` already uses a 100ms pause between
// the message and the CR (see cmdCommand at command.go:93) and works
// reliably on both harnesses, so we mirror that pattern.
//
// sleep is injected so tests can verify the pause happened without
// blocking the test process. Production callers pass time.Sleep.
//
// Empty/unknown harness — non-agent `ppz terminal share` calls
// where PPZ_AGENT_HARNESS is unset, or a harness we haven't yet
// confirmed — falls into the `\r`+pause arm: a plain carriage return
// is the lowest-risk default since most line-discipline REPLs accept
// it as Enter, and the pause never hurts a REPL that would have
// accepted CR in the same burst.
func submitAlertToPTY(w io.Writer, harness, message string, sleep func(time.Duration)) error {
	trimmed := strings.TrimRight(message, "\r\n")
	if harness == "claude" {
		_, err := io.WriteString(w, trimmed+"\x1b[13u")
		return err
	}
	if _, err := io.WriteString(w, trimmed); err != nil {
		return err
	}
	sleep(100 * time.Millisecond)
	_, err := io.WriteString(w, "\r")
	return err
}

func (p *terminalInboxAlertPump) BeginAlertMode(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.alert = true
}

func (p *terminalInboxAlertPump) EndAlertMode(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.alert = false
	if len(p.buffer) > 0 {
		_, _ = p.pty.Write(p.buffer)
		p.buffer = nil
	}
}

func (p *terminalInboxAlertPump) ForwardUserInput(now time.Time, input []byte) bool {
	if len(input) == 0 {
		return true
	}
	p.ObserveUserInput(now, input)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.alert {
		p.buffer = append(p.buffer, input...)
		return false
	}
	_, _ = p.pty.Write(input)
	return true
}
