// Package envelope is the JSON shape of every message published on
// <org_id>.<handle>.broadcast (per WIRE.md §3).
package envelope

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const MaxBytes = 65536 // 64 KiB cap on the encoded envelope.

// Sender is the source handle the message was published *from* — the
// broadcaster's current source at publish time. Empty string when the
// publisher had no current source set (e.g. `ppz send <dest>` from a
// session that never connected). Distinct from the destination handle,
// which is encoded only in the NATS subject (per WIRE.md §3).
type Message struct {
	ID        string    `json:"id"`
	Sender    string    `json:"sender"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

func New(sender, payload string, now time.Time) Message {
	return Message{
		ID:        uuid.NewString(),
		Sender:    sender,
		Payload:   payload,
		CreatedAt: now.UTC(),
	}
}

func (m Message) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func Unmarshal(b []byte) (Message, error) {
	var m Message
	err := json.Unmarshal(b, &m)
	return m, err
}
