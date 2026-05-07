package envelope

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// v0.23.0: envelope carries `sender` (the broadcasting source) instead
// of `handle` (the destination). Sender is empty when the publisher
// has no current source set.
func TestNew_PopulatesSender(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	m := New("alpha", "hi", now)
	if m.Sender != "alpha" {
		t.Fatalf("Sender = %q, want %q", m.Sender, "alpha")
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

func TestNew_EmptySenderWhenNoCurrent(t *testing.T) {
	m := New("", "p", time.Now())
	if m.Sender != "" {
		t.Fatalf("Sender = %q, want empty", m.Sender)
	}
}

func TestMarshal_OmitsHandleField(t *testing.T) {
	m := New("alpha", "hi", time.Now())
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

// Old retained messages (pre-v0.23) carry "handle" but not "sender".
// They must parse cleanly: handle is silently dropped, sender stays
// empty (zero value for the missing field).
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
}

func TestUnmarshal_NewSenderRoundtrip(t *testing.T) {
	m := New("alpha", "hi", time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC))
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
}
