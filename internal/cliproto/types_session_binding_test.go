package cliproto

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// Tests WP-1, WP-2, WP-3 from docs/specs/session-binding.md (refined).
//
// Round-trip serialization of session-binding IPC types + the wire-
// compat guard for the AncestorPIDs field on session-using requests.

// WP-1: RegisterAgentBindingRequest round-trips.
func TestRegisterAgentBindingRequest_RoundTrip(t *testing.T) {
	in := RegisterAgentBindingRequest{Handle: "cindy", SharePID: 41203}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out RegisterAgentBindingRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n  in=%+v\n out=%+v", in, out)
	}
}

// WP-1.b: RegisterAgentBindingReply round-trips with all fields.
func TestRegisterAgentBindingReply_RoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 14, 33, 0, time.UTC)
	in := RegisterAgentBindingReply{
		Handle:       "cindy",
		SharePID:     41203,
		SessionKey:   "agent:cindy",
		RegisteredAt: now,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out RegisterAgentBindingReply
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n  in=%+v\n out=%+v", in, out)
	}
}

// WP-1.c: UnregisterAgentBindingRequest round-trips with SharePID.
func TestUnregisterAgentBindingRequest_RoundTrip(t *testing.T) {
	in := UnregisterAgentBindingRequest{SharePID: 41203}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out UnregisterAgentBindingRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SharePID != 41203 {
		t.Errorf("SharePID = %d, want 41203", out.SharePID)
	}
}

// WP-2: old-format StatusRequest (Session only, no AncestorPIDs)
// still decodes cleanly. Wire compat.
func TestStatusRequest_OldFormatStillDecodes(t *testing.T) {
	oldWire := `{"session":"sid-12345"}`
	var got StatusRequest
	if err := json.Unmarshal([]byte(oldWire), &got); err != nil {
		t.Fatalf("decode old-format StatusRequest: %v", err)
	}
	if got.Session != "sid-12345" {
		t.Errorf("Session = %q, want %q", got.Session, "sid-12345")
	}
}

// WP-2.b: new-format payload with AncestorPIDs decodes (current decoder
// gracefully ignores unknown field). Note: we don't add AncestorPIDs
// to StatusRequest in this test file; production code will. Wire-shape
// test only.
func TestStatusRequest_NewFormatGracefulDowngrade(t *testing.T) {
	newWire := `{"session":"","ancestor_pids":[50000,41203]}`
	var got StatusRequest
	if err := json.Unmarshal([]byte(newWire), &got); err != nil {
		t.Fatalf("decode new-format StatusRequest: %v", err)
	}
	if got.Session != "" {
		t.Errorf("Session = %q, want empty (new CLI signals 'please resolve')", got.Session)
	}
}

// WP-3: AncestorPIDs field round-trips, including empty/nil distinction.
// When production code adds the field to session-using requests, this
// guard ensures the wire shape stays consistent.
//
// NOTE: this test exercises a synthetic struct that mirrors what the
// real StatusRequest WILL look like post-impl. It's intentionally
// here as a wire-shape pin, not a real type — impl will move the
// field onto the existing types.
func TestAncestorPIDs_WireShape(t *testing.T) {
	type sessionRequestShape struct {
		Session      string `json:"session,omitempty"`
		AncestorPIDs []int  `json:"ancestor_pids,omitempty"`
	}

	// Empty AncestorPIDs MUST omitempty — wire compat with old daemons.
	emptyWire, _ := json.Marshal(sessionRequestShape{Session: "foo"})
	if contains(string(emptyWire), "ancestor_pids") {
		t.Errorf("empty AncestorPIDs leaked into wire: %s (must omitempty)", string(emptyWire))
	}

	// Populated AncestorPIDs round-trips.
	in := sessionRequestShape{Session: "", AncestorPIDs: []int{50000, 41203, 1}}
	b, _ := json.Marshal(in)
	var out sessionRequestShape
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(in.AncestorPIDs, out.AncestorPIDs) {
		t.Errorf("AncestorPIDs round-trip: in=%v out=%v", in.AncestorPIDs, out.AncestorPIDs)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
