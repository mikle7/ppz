package db

import (
	"reflect"
	"strings"
	"testing"
)

// Phase 2: users.password_hash backs auth_mode=password. Nullable on the
// DB column because OAuth-only and pre-Phase-2 users have no password.
// See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md Cycle A.

func TestUser_HasPasswordHashField(t *testing.T) {
	field, ok := reflect.TypeOf(User{}).FieldByName("PasswordHash")
	if !ok {
		t.Fatal("User.PasswordHash field missing")
	}
	if field.Type.Kind() != reflect.Ptr {
		t.Fatalf("User.PasswordHash kind = %v, want pointer (*string) — nullable for OAuth-only users", field.Type.Kind())
	}
	if field.Type.Elem().Kind() != reflect.String {
		t.Errorf("User.PasswordHash elem = %v, want string", field.Type.Elem())
	}
}

// Phase 2 migration 0003_password_hash.sql is embedded and adds the
// nullable column. Cheap structural check — doesn't need a running
// Postgres. Schema-level integration tests run separately.

func TestMigration0003_AddsPasswordHashColumn(t *testing.T) {
	data, err := migrationsFS.ReadFile("migrations/0003_password_hash.sql")
	if err != nil {
		t.Fatalf("migrations/0003_password_hash.sql missing: %v", err)
	}
	s := string(data)
	for _, needle := range []string{
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash",
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("0003_password_hash.sql missing expected statement: %q", needle)
		}
	}
}
