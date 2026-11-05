package application

import "uuid"

type CreateApplicationRequest struct {
	Name          string `json:"name" validate:"required,min=3,max=50"`
	Image         string `json:"image" validate:"required"`
	PlanID        string `json:"plan_id" validate:"required,uuid"`
	ContainerPort int    `json:"container_port" validate:"required,min=1,max=65535"`
}

type CreateApplicationParams struct {
	ProjectID     string
	Name          string
	Image         string
	PlanID        string
	ContainerPort int
}

type CreateApplicationData struct {
	ProjectID       string
	PlanID          string
	Name            string
	Image           string
	ContainerPort   int
	Replicas        int
	CPURequestM     int
	CPULimitM       int
	MemoryRequestMi int
	MemoryLimitMi   int
}

type ApplicationDeployConfig struct {
	ID        uuid.UUID
	ProjectID uuid.UUID

	Image         string
	Replicas      int32
	ContainerPort int32

	CPURequestM     int32
	CPULimitM       int32
	MemoryRequestMi int32
	MemoryLimitMi   int32
}
