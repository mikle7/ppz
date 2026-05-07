package envelope

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// v0.23.0: envelope carries `sender` (the broadcasting source) instead
// of `handle` (the destination), plus an optional `subject` (header-line
// metadata — user-set or system-set "ack:..."). Sender / subject are
// empty when the publisher has none.
func TestNew_PopulatesSenderAndSubject(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	m := New("alpha", "status update", "hi", now)
	if m.Sender != "alpha" {
		t.Fatalf("Sender = %q, want %q", m.Sender, "alpha")
	}
	if m.Subject != "status update" {
		t.Fatalf("Subject = %q, want %q", m.Subject, "status update")
	}
	if m.Payload != "hi" {
		t.Fatalf("Payload = %q", m.Payload)
	}
	if !m.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v", m.CreatedAt)
	}
	if m.ID == "" {
		t.Fatalf("ID empty")
	}
}

func TestNew_EmptySenderAndSubject(t *testing.T) {
	m := New("", "", "p", time.Now())
	if m.Sender != "" {
		t.Fatalf("Sender = %q, want empty", m.Sender)
	}
	if m.Subject != "" {
		t.Fatalf("Subject = %q, want empty", m.Subject)
	}
}

func TestMarshal_OmitsHandleField(t *testing.T) {
	m := New("alpha", "", "hi", time.Now())
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `"handle"`) {
		t.Fatalf("marshalled envelope still contains \"handle\": %s", s)
	}
	if !strings.Contains(s, `"sender":"alpha"`) {
		t.Fatalf("marshalled envelope missing sender: %s", s)
	}
}

// Subject MUST always serialise — even when empty — so receivers see a
// stable shape. Conversely, decoding must accept envelopes that omit
// the field entirely (legacy + future older publishers).
func TestMarshal_AlwaysIncludesSubject(t *testing.T) {
	m := New("alpha", "", "hi", time.Now())
	b, _ := m.Marshal()
	if !strings.Contains(string(b), `"subject":""`) {
		t.Fatalf("empty subject must still appear on the wire: %s", b)
	}
	m2 := New("alpha", "ack:read", "hi", time.Now())
	b2, _ := m2.Marshal()
	if !strings.Contains(string(b2), `"subject":"ack:read"`) {
		t.Fatalf("non-empty subject missing: %s", b2)
	}
}

// Old retained messages (pre-v0.23) carry "handle" but not "sender" /
// "subject". They must parse cleanly: handle silently dropped, the new
// fields zero-value to "".
func TestUnmarshal_LegacyHandleParsesCleanly(t *testing.T) {
	legacy := []byte(`{"id":"abc","handle":"chat","payload":"hi","created_at":"2026-05-07T12:00:00Z"}`)
	m, err := Unmarshal(legacy)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.ID != "abc" {
		t.Fatalf("ID = %q", m.ID)
	}
	if m.Payload != "hi" {
		t.Fatalf("Payload = %q", m.Payload)
	}
	if m.Sender != "" {
		t.Fatalf("Sender from legacy envelope = %q, want empty", m.Sender)
	}
	if m.Subject != "" {
		t.Fatalf("Subject from legacy envelope = %q, want empty", m.Subject)
	}
}

func TestUnmarshal_NewShapeRoundtrip(t *testing.T) {
	m := New("alpha", "ack:read", "hi", time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sender != "alpha" {
		t.Fatalf("Sender = %q", got.Sender)
	}
	if got.Subject != "ack:read" {
		t.Fatalf("Subject = %q", got.Subject)
	}
	// Cross-check that the wire shape is what we expect.
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["handle"]; ok {
		t.Fatalf("wire envelope still has \"handle\" key: %v", raw)
	}
	if raw["sender"] != "alpha" {
		t.Fatalf("wire sender = %v, want alpha", raw["sender"])
	}
	if raw["subject"] != "ack:read" {
		t.Fatalf("wire subject = %v, want ack:read", raw["subject"])
	}
}
