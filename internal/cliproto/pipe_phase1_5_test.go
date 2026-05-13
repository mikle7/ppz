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
}
