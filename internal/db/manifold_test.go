package db

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Phase 1.5: Source rows carry an explicit manifold (hierarchical-grouping)
// segment. Empty string '' represents the root namespace (a real value,
// never NULL). See PHASE-1.5-IMPLEMENTATION-PLAN.md in pipes-internal.

func TestSource_HasManifoldField(t *testing.T) {
	field, ok := reflect.TypeOf(Source{}).FieldByName("Manifold")
	if !ok {
		t.Fatal("Source.Manifold field missing")
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("Source.Manifold type = %v, want string", field.Type)
	}
}

// Phase 1.5: Pipe rows carry an explicit manifold (mirrors Source). Empty
// string '' for the root.

func TestPipe_HasManifoldField(t *testing.T) {
	field, ok := reflect.TypeOf(Pipe{}).FieldByName("Manifold")
	if !ok {
		t.Fatal("Pipe.Manifold field missing")
	}
	if field.Type.Kind() != reflect.String {
		t.Errorf("Pipe.Manifold type = %v, want string", field.Type)
	}
}

// Phase 1.5: Pipe.SourceID becomes nullable (*uuid.UUID). Uncollared
// (sourceless) pipes have no source — NULL on the DB column.

func TestPipe_SourceID_IsNullablePointer(t *testing.T) {
	field, ok := reflect.TypeOf(Pipe{}).FieldByName("SourceID")
	if !ok {
		t.Fatal("Pipe.SourceID field missing")
	}
	if field.Type.Kind() != reflect.Ptr {
		t.Fatalf("Pipe.SourceID kind = %v, want pointer (*uuid.UUID) — uncollared pipes have no source", field.Type.Kind())
	}
	if field.Type.Elem() != reflect.TypeOf(uuid.UUID{}) {
		t.Errorf("Pipe.SourceID elem = %v, want uuid.UUID", field.Type.Elem())
	}
}

// InsertSource: signature accepts an explicit manifold string. Compile-time
// pin — this won't build until the implementation lands.

func TestInsertSource_SignatureAcceptsManifold(t *testing.T) {
	_ = func(pool *Pool, accountID, createdBy uuid.UUID, manifold, handle string, kind SourceKind) {
		_, _ = InsertSource(nil, pool, accountID, createdBy, manifold, handle, kind)
	}
}

// InsertPipe: signature accepts accountID + manifold + nullable sourceID.
// AccountID is needed because the pipe row is no longer reachable via
// source.account_id when source_id IS NULL (uncollared pipes).

func TestInsertPipe_SignatureAcceptsManifoldAndNullableSourceID(t *testing.T) {
	_ = func(pool *Pool, accountID uuid.UUID, manifold string, sourceID *uuid.UUID, createdBy uuid.UUID, name string, ttl *int, maxMsgs *int, maxBytes *int64) {
		_, _ = InsertPipe(nil, pool, accountID, manifold, sourceID, createdBy, name, ttl, maxMsgs, maxBytes)
	}
}

// Phase 1.5 migration 0002_manifold.sql is embedded and contains the
// expected ALTER TABLE statements. Cheap structural check — doesn't need a
// running Postgres. Schema-level assertions (column types, constraints)
// happen in integration tests in commit 2's GREEN phase.

func TestMigration0002_AddsManifoldAndNullableSourceID(t *testing.T) {
	data, err := migrationsFS.ReadFile("migrations/0002_manifold.sql")
	if err != nil {
		t.Fatalf("migrations/0002_manifold.sql missing: %v", err)
	}
	s := string(data)
	for _, needle := range []string{
		"ALTER TABLE sources ADD COLUMN IF NOT EXISTS manifold",
		"ALTER TABLE pipes ADD COLUMN IF NOT EXISTS manifold",
		"ALTER TABLE pipes ALTER COLUMN source_id DROP NOT NULL",
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("0002_manifold.sql missing expected statement: %q", needle)
		}
	}
}
