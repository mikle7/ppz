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

// HUMAN column: the daemon attaches the source's creator username on
// every Source row (CreatedBy) and — for user-created pipes — the
// pipe's own creator on PipeInfo.CreatedBy. The CLI renders HUMAN as
// the rightmost column on `ppz ls` and as `human` in `ppz ls --json`.
//
// JSON tag is "created_by" (canonical, matches the column on the
// underlying tables) — the formatter maps that to the user-facing
// "human" key when emitting `ppz ls --json`. This test pins the wire
// shape, not the rendered shape.
func TestSource_HasCreatedByField(t *testing.T) {
	s := Source{Handle: "chat", CreatedBy: "foo"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"created_by":"foo"`) {
		t.Fatalf("created_by missing from Source wire: %s", string(b))
	}
	var got Source
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.CreatedBy != "foo" {
		t.Fatalf("CreatedBy lost in round-trip: %q", got.CreatedBy)
	}
}

func TestPipeInfo_HasCreatedByField(t *testing.T) {
	p := PipeInfo{Pipe: "archive", CreatedBy: "bar"}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"created_by":"bar"`) {
		t.Fatalf("created_by missing from PipeInfo wire: %s", string(b))
	}
	var got PipeInfo
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.CreatedBy != "bar" {
		t.Fatalf("CreatedBy lost in round-trip: %q", got.CreatedBy)
	}
}

// CreatedBy is omitempty so the daemon's intermediate representation
// (where Source-level creator may be unset on a partial enrichment)
// doesn't leak `"created_by":""` over the wire. Auto-pipe inheritance
// happens at render time, not on the wire.
func TestSourceAndPipeInfo_CreatedByOmitemptyOnWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
	}{
		{"empty Source", string(mustMarshal(t, Source{Handle: "x"}))},
		{"empty PipeInfo", string(mustMarshal(t, PipeInfo{Pipe: "y"}))},
	} {
		if strings.Contains(tc.json, `"created_by"`) {
			t.Errorf("%s wire %s leaks empty created_by", tc.name, tc.json)
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
