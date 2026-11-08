package eventstore

import (
	"encoding/json"
	"time"
	"uuid"
)

type Event struct {
	ID            uuid.UUID
	DeploymentID  uuid.UUID
	Type          string
	Version       int64
	SchemaVersion int
	Payload       json.RawMessage
	CreatedAt     time.Time
}
