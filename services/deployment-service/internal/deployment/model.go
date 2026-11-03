package deployment

import (
	"github.com/Marvin348/trest/packages/secretcrypto"
	"time"
	"uuid"
)

type Status string

const (
	StatusQueued    Status = "QUEUED"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

type Deployment struct {
	ID            uuid.UUID
	RequestID     uuid.UUID
	ProjectID     uuid.UUID
	ApplicationID uuid.UUID

	Image         string
	Replicas      int32
	ContainerPort int32

	CPURequestM     int32
	CPULimitM       int32
	MemoryRequestMi int32
	MemoryLimitMi   int32

	Environment map[string]secretcrypto.EncryptedValue

	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}
