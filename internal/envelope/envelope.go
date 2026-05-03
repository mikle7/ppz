// Package envelope is the JSON shape of every message published on
// <org_id>.<handle>.broadcast (per WIRE.md §3).
package envelope

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const MaxBytes = 65536 // 64 KiB cap on the encoded envelope.

type Message struct {
	ID        string    `json:"id"`
	Handle    string    `json:"handle"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

func New(handle, payload string, now time.Time) Message {
	return Message{
		ID:        uuid.NewString(),
		Handle:    handle,
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
