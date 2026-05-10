package db

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// Sources gain a `created_by_user_id` column (NOT NULL, references
// users) so `ppz ls` HUMAN can attribute every source row to a real
// user. The server stamps this from the caller at create time:
// API-key callers contribute the key's CreatedByUserID; OAuth
// callers contribute caller.UserID directly.

func TestSource_HasCreatedByUserIDField(t *testing.T) {
	uid := uuid.New()
	s := Source{
		ID:              uuid.New(),
		OrganisationID:  uuid.New(),
		Handle:          "chat",
		Kind:            SourceKindMessage,
		CreatedByUserID: uid,
	}
	if s.CreatedByUserID != uid {
		t.Fatalf("CreatedByUserID round-trip: got %v want %v", s.CreatedByUserID, uid)
	}

	field, ok := reflect.TypeOf(Source{}).FieldByName("CreatedByUserID")
	if !ok {
		t.Fatal("Source.CreatedByUserID field missing")
	}
	if field.Type != reflect.TypeOf(uuid.UUID{}) {
		t.Errorf("Source.CreatedByUserID type = %v, want uuid.UUID (NOT NULL)", field.Type)
	}
}

// InsertSource accepts (ctx, pool, orgID, createdBy, handle, kind).
// Compile-time pin.
func TestInsertSource_SignatureAcceptsCreatedBy(t *testing.T) {
	_ = func(pool *Pool, orgID, createdBy uuid.UUID) {
		_, _ = InsertSource(nil, pool, orgID, createdBy, "chat", SourceKindMessage)
	}
}
