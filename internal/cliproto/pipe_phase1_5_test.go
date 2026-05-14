package cliproto

import (
	"reflect"
	"testing"
)

// Phase 1.5: PipeCreateRequest grows fields for the four-role grammar
// per locked decision #18 — <account>.<manifold?>.<source?>.<pipe>.
//
// - Manifold:     the hierarchical-grouping segment string ('' = root)
// - SourceHandle: pointer to the actor identity name; nil = uncollared
//                 (sourceless pipe; symmetric many-to-many semantics)
//
// The existing Handle field is retained as a Phase-1 backward-compat
// alias for SourceHandle until Cycle B finishes threading the new
// fields through the daemon. Cycle E's docs commit removes Handle.

func TestPipeCreateRequest_HasManifoldField(t *testing.T) {
	field, ok := reflect.TypeOf(PipeCreateRequest{}).FieldByName("Manifold")
	if !ok {
		t.Fatal("PipeCreateRequest.Manifold field missing")
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("PipeCreateRequest.Manifold type = %v, want string", field.Type)
	}
	if tag := field.Tag.Get("json"); tag != "manifold" && tag != "manifold,omitempty" {
		t.Errorf("PipeCreateRequest.Manifold json tag = %q, want manifold or manifold,omitempty", tag)
	}
}

// SourceHandle is a pointer (*string) so nil distinguishes "uncollared"
// from "" (the empty handle isn't valid; nil is the only way to
// represent absence cleanly through JSON omitempty).
func TestPipeCreateRequest_HasNullableSourceHandle(t *testing.T) {
	field, ok := reflect.TypeOf(PipeCreateRequest{}).FieldByName("SourceHandle")
	if !ok {
		t.Fatal("PipeCreateRequest.SourceHandle field missing — uncollared pipes need a way to signal 'no source' via nil")
	}
	if field.Type.Kind() != reflect.Ptr {
		t.Fatalf("PipeCreateRequest.SourceHandle kind = %v, want pointer (*string)", field.Type.Kind())
	}
	if field.Type.Elem().Kind() != reflect.String {
		t.Errorf("PipeCreateRequest.SourceHandle elem = %v, want string", field.Type.Elem())
	}
	// Pin the JSON tag too — the omitempty + pointer combination is what
	// distinguishes "uncollared" (field omitted) from "explicit empty
	// source handle" on the wire. Drift on either would change the wire
	// shape silently.
	if tag := field.Tag.Get("json"); tag != "source_handle,omitempty" {
		t.Errorf("PipeCreateRequest.SourceHandle json tag = %q, want source_handle,omitempty", tag)
	}
}

// Phase 1.5 Cycle C: StatusReply carries the current namespace so the
// CLI can render the `namespace: …` line without a second IPC
// round-trip. Empty string when no namespace is set — same as
// Current/handle today.
func TestStatusReply_HasCurrentNamespaceField(t *testing.T) {
	field, ok := reflect.TypeOf(StatusReply{}).FieldByName("CurrentNamespace")
	if !ok {
		t.Fatal("StatusReply.CurrentNamespace field missing — needed for `ppz status` to render the namespace line")
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("StatusReply.CurrentNamespace type = %v, want string", field.Type)
	}
}
