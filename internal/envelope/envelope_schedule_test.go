package envelope

// RED — docs/specs/schedule.md. Messages published by the server-side
// scheduler carry the originating schedule's short id so receivers can
// distinguish scheduled messages from live sends. omitempty keeps the
// field invisible on every plain send (wire-compatible with old
// daemons).

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMessage_ScheduleIDField(t *testing.T) {
	field, ok := reflect.TypeOf(Message{}).FieldByName("ScheduleID")
	if !ok {
		t.Fatal("Message.ScheduleID field missing")
	}
	if field.Type.Kind() != reflect.String {
		t.Fatalf("Message.ScheduleID type = %v, want string", field.Type)
	}
	tag := field.Tag.Get("json")
	if tag != "schedule_id,omitempty" {
		t.Fatalf("json tag = %q, want %q", tag, "schedule_id,omitempty")
	}
}

func TestMessage_ScheduleIDOmittedWhenEmpty(t *testing.T) {
	raw, err := json.Marshal(Message{Sender: "alice", Payload: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "schedule_id") {
		t.Fatalf("plain sends must not carry schedule_id: %s", raw)
	}
}
