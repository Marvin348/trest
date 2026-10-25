package project

type CreateProjectRequest struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}