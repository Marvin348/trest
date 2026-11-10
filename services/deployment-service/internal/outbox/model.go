package outbox

import (
	"encoding/json"
	"time"
	"uuid"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusPublished Status = "published"
	StatusFailed    Status = "failed"
)

type Message struct {
	ID          uuid.UUID
	Type        string
	Payload     json.RawMessage
	Status      string
	Attempts    int
	CreatedAt   time.Time
	PublishedAt *time.Time
}
