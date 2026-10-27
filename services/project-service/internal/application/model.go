package application

import (
	"time"
)

type Application struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	PlanID          string    `json:"plan_id"`
	Image           string    `json:"image"`
	Name            string    `json:"name"`
	Replicas        int       `json:"replicas"`
	CPURequestM     int       `json:"cpu_request_m"`
	CPULimitM       int       `json:"cpu_limit_m"`
	MemoryRequestMi int       `json:"memory_request_mi"`
	MemoryLimitMi   int       `json:"memory_limit_mi"`
	ContainerPort   int       `json:"container_port"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
