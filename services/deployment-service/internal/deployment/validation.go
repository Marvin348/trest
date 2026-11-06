package deployment

import (
	"fmt"
	"strings"
)

func validateCreateDeploymentParams(params CreateDeploymentParams) error {

	if strings.TrimSpace(params.Image) == "" {
		return fmt.Errorf("image cannot be empty: %w", ErrInvalidDeployment)
	}

	if params.Replicas < 1 {
		return fmt.Errorf("replicas must be at least 1 (got %d): %w",
			params.Replicas, ErrInvalidDeployment)
	}

	if params.ContainerPort < 1 || params.ContainerPort > 65535 {
		return fmt.Errorf("container port must be between 1 and 65535 (got %d): %w",
			params.ContainerPort, ErrInvalidDeployment)
	}

	if params.CPURequestM <= 0 {
		return fmt.Errorf("cpu request must be greater than 0 (got %d): %w",
			params.CPURequestM, ErrInvalidDeployment)
	}

	if params.CPULimitM < params.CPURequestM {
		return fmt.Errorf("cpu limit (%dm) cannot be less than cpu request (%dm): %w",
			params.CPULimitM, params.CPURequestM, ErrInvalidDeployment)
	}

	if params.MemoryRequestMi <= 0 {
		return fmt.Errorf("memory request must be greater than 0 (got %d): %w",
			params.MemoryRequestMi, ErrInvalidDeployment)
	}

	if params.MemoryLimitMi < params.MemoryRequestMi {
		return fmt.Errorf("memory limit (%dMi) cannot be less than memory request (%dMi): %w",
			params.MemoryLimitMi, params.MemoryRequestMi, ErrInvalidDeployment)
	}

	return nil
}
