package cliproto

import (
	"encoding/json"
	"strings"
	"testing"
)

// v0.25.0: ReadMessage mirrors the envelope's new InReplyTo /
// AckRequested fields (so the daemon can ship them through to CLI
// renderers). JSON tags must align with envelope: in_reply_to,
// ack_requested.
func TestReadMessage_HasReplyAndAckFieldsMatchingEnvelope(t *testing.T) {
	rm := ReadMessage{
		ID:           "abc",
		Sender:       "alpha",
		Subject:      "ack:read",
		Payload:      "",
		CreatedAt:    "2026-05-07T12:00:00Z",
		InReplyTo:    "11111111-2222-3333-4444-555566667777",
		AckRequested: true,
	}
	b, err := json.Marshal(rm)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"in_reply_to":"11111111-2222-3333-4444-555566667777"`) {
		t.Fatalf("in_reply_to missing from ReadMessage wire: %s", s)
	}
	if !strings.Contains(s, `"ack_requested":true`) {
		t.Fatalf("ack_requested missing from ReadMessage wire: %s", s)
	}
	// Round-trip parse confirms the JSON tags.
	var got ReadMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.InReplyTo != rm.InReplyTo {
		t.Fatalf("InReplyTo lost in round-trip: %q", got.InReplyTo)
	}
	if !got.AckRequested {
		t.Fatalf("AckRequested lost in round-trip")
	}
}

// v0.25.0: BroadcastRequest carries the new send-side flags through IPC.
// Per spec §3, the JSON tags align with the envelope (in_reply_to /
// ack_requested) — *not* with the older MsgSubject precedent.
func TestBroadcastRequest_HasReplyAndAckFields(t *testing.T) {
	br := BroadcastRequest{
		Handle:       "foo",
		Channel:      "inbox",
		Payload:      "hi",
		MsgSubject:   "user-subject",
		InReplyTo:    "11111111-2222-3333-4444-555566667777",
		AckRequested: true,
	}
	b, err := json.Marshal(br)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"in_reply_to":"11111111-2222-3333-4444-555566667777"`) {
		t.Fatalf("in_reply_to missing from BroadcastRequest wire: %s", s)
	}
	if !strings.Contains(s, `"ack_requested":true`) {
		t.Fatalf("ack_requested missing from BroadcastRequest wire: %s", s)
	}
	// Round-trip preserves both.
	var got BroadcastRequest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.InReplyTo != br.InReplyTo || !got.AckRequested {
		t.Fatalf("round-trip lost fields: %+v", got)
	}
}
