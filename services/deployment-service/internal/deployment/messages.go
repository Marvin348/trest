package deployment

import (
	"uuid"

	"github.com/Marvin348/trest/packages/secretcrypto"
)

type DeploymentRequestedMessage struct {
	DeploymentID  uuid.UUID `json:"deployment_id"`
	ProjectID     uuid.UUID `json:"project_id"`
	ApplicationID uuid.UUID `json:"application_id"`

	Image         string `json:"image"`
	Replicas      int32  `json:"replicas"`
	ContainerPort int32  `json:"container_port"`

	CPURequestM     int32 `json:"cpu_request_m"`
	CPULimitM       int32 `json:"cpu_limit_m"`
	MemoryRequestMi int32 `json:"memory_request_mi"`
	MemoryLimitMi   int32 `json:"memory_limit_mi"`

	Environment map[string]secretcrypto.EncryptedValue `json:"environment"`
}

func newDeploymentRequestedMessage(params CreateDeploymentParams,
	deploymentID uuid.UUID,
) DeploymentRequestedMessage {
	return DeploymentRequestedMessage{
		DeploymentID:    deploymentID,
		ProjectID:       params.ProjectID,
		ApplicationID:   params.ApplicationID,
		Image:           params.Image,
		Replicas:        params.Replicas,
		ContainerPort:   params.ContainerPort,
		CPURequestM:     params.CPURequestM,
		CPULimitM:       params.CPULimitM,
		MemoryRequestMi: params.MemoryRequestMi,
		MemoryLimitMi:   params.MemoryLimitMi,
		Environment:     params.Environment,
	}
}
