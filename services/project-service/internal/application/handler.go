package application

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"example.com/trest/services/project-service/internal/apperror"
	"example.com/trest/services/project-service/internal/httpx"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	service  *Service
	validate *validator.Validate
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
		validate: validator.New(
			validator.WithRequiredStructEnabled(),
		),
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")

	if err := h.validate.Var(projectID, "required,uuid"); err != nil {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Invalid projectId",
			Cause:   err,
		})
		return
	}

	var input CreateApplicationRequest

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Invalid JSON body",
			Cause:   err,
		})
		return
	}

	if err := h.validate.Struct(input); err != nil {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Application validation failed",
			Cause:   err,
		})
		return
	}

	params := CreateApplicationParams{
		ProjectID:     projectID,
		Name:          input.Name,
		Image:         input.Image,
		PlanID:        input.PlanID,
		ContainerPort: input.ContainerPort,
	}

	applicaton, err := h.service.Create(r.Context(), params)

	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := httpx.WriteJSON(w, http.StatusCreated, applicaton); err != nil {
		slog.Error("send applicaton response", "error", err)
	}
}
