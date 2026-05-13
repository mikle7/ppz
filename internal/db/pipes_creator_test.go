package db

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// Pipes (the user-creatable rows in the `pipes` table — NOT the
// auto-provisioned broadcast/inbox/stdin/stdout/stdctrl set) gain a
// `created_by_user_id` column. `ppz ls` HUMAN reads this when
// rendering custom pipes; auto-pipes inherit the source's creator.

func TestPipe_HasCreatedByUserIDField(t *testing.T) {
	uid := uuid.New()
	srcID := uuid.New()
	p := Pipe{
		ID:              uuid.New(),
		SourceID:        &srcID,
		Name:            "archive",
		CreatedByUserID: uid,
	}
	if p.CreatedByUserID != uid {
		t.Fatalf("CreatedByUserID round-trip: got %v want %v", p.CreatedByUserID, uid)
	}

	field, ok := reflect.TypeOf(Pipe{}).FieldByName("CreatedByUserID")
	if !ok {
		t.Fatal("Pipe.CreatedByUserID field missing")
	}
	if field.Type != reflect.TypeOf(uuid.UUID{}) {
		t.Errorf("Pipe.CreatedByUserID type = %v, want uuid.UUID (NOT NULL)", field.Type)
	}
}

// InsertPipe accepts (ctx, pool, accountID, manifold, sourceID, createdBy,
// name, ttl, maxMsgs, maxBytes). Compile-time pin. Phase 1.5 added the
// accountID + manifold args and made sourceID a *uuid.UUID for uncollared
// pipes; this pin preserves the original "createdBy is required" intent.
func TestInsertPipe_SignatureAcceptsCreatedBy(t *testing.T) {
	_ = func(pool *Pool, accountID, sourceID, createdBy uuid.UUID) {
		var ttl, maxMsgs *int
		var maxBytes *int64
		_, _ = InsertPipe(nil, pool, accountID, "", &sourceID, createdBy, "archive", ttl, maxMsgs, maxBytes)
	}
}
