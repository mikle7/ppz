package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

const terminalInboxAlertMessage = "You have received a message. Read unread messages with: ppz read inbox\n"

type terminalInboxAlertConfig struct {
	IdleAfter time.Duration
	Cooldown  time.Duration
	Message   string
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
	alert  bool
	buffer []byte
}

func newTerminalInboxAlertPump(cfg terminalInboxAlertConfig, pty io.Writer) *terminalInboxAlertPump {
	if cfg.Message == "" {
		cfg.Message = terminalInboxAlertMessage
	}
	return &terminalInboxAlertPump{
		sm:  newTerminalInboxAlertStateMachine(cfg),
		pty: pty,
	}
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
	_, _ = io.WriteString(p.pty, shellEchoCommand(alert))
	p.EndAlertMode(now)
	return true
}

func shellEchoCommand(message string) string {
	return fmt.Sprintf("echo %s\n", shellSingleQuote(message))
}

func shellSingleQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
			continue
		}
		if r == '\r' || r == '\n' {
			continue
		}
		out += string(r)
	}
	return out + "'"
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
