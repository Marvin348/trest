package deployment

import (
	"github.com/Marvin348/trest/packages/secretcrypto"
	"uuid"
)

type CreateDeploymentParams struct {
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
}
