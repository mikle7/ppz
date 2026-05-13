package db

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// API keys gain a `created_by_user_id` column (NOT NULL, references
// users) so we can attribute every key — and every source/pipe
// created via that key — back to a real user. These tests pin the
// struct shape and the InsertAPIKey signature; DB-state tests live
// in the e2e suite.

func TestAPIKey_HasCreatedByUserIDField(t *testing.T) {
	uid := uuid.New()
	k := APIKey{
		ID:              uuid.New(),
		AccountID:  uuid.New(),
		CreatedByUserID: uid,
		KeyPrefix:       "abcdefgh",
		Label:           "test",
	}
	if k.CreatedByUserID != uid {
		t.Fatalf("CreatedByUserID round-trip mismatch: got %v want %v", k.CreatedByUserID, uid)
	}

	// CreatedByUserID is uuid.UUID (NOT NULL), not a *uuid.UUID. We
	// assert the field type so a future "let's make it nullable"
	// regression is caught at the unit-test layer rather than via
	// surprise NULLs at runtime.
	field, ok := reflect.TypeOf(APIKey{}).FieldByName("CreatedByUserID")
	if !ok {
		t.Fatal("APIKey.CreatedByUserID field missing")
	}
	if field.Type != reflect.TypeOf(uuid.UUID{}) {
		t.Errorf("APIKey.CreatedByUserID type = %v, want uuid.UUID (NOT NULL contract)", field.Type)
	}
}

// InsertAPIKey signature must accept a creator user id alongside the
// org id and label. The compile-time call here is the test — if the
// signature drifts back to (ctx, pool, accountID, label) this file stops
// compiling, surfacing the regression.
func TestInsertAPIKey_SignatureAcceptsCreatedBy(t *testing.T) {
	// Compile-time only: we never call this. The closure pins the
	// signature without needing a live pool.
	_ = func(pool *Pool, accountID, createdBy uuid.UUID) {
		_, _, _ = InsertAPIKey(nil, pool, accountID, createdBy, "label")
	}
}
