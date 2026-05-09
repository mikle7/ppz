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
	p := Pipe{
		ID:              uuid.New(),
		SourceID:        uuid.New(),
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

// InsertPipe accepts (ctx, pool, sourceID, createdBy, name, ttl,
// maxMsgs, maxBytes). Compile-time pin.
func TestInsertPipe_SignatureAcceptsCreatedBy(t *testing.T) {
	_ = func(pool *Pool, sourceID, createdBy uuid.UUID) {
		var ttl, maxMsgs *int
		var maxBytes *int64
		_, _ = InsertPipe(nil, pool, sourceID, createdBy, "archive", ttl, maxMsgs, maxBytes)
	}
}
