package application

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
