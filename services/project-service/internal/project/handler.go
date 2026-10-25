package project

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
	var input CreateProjectRequest

	err := json.NewDecoder(r.Body).Decode(&input)

	if err != nil {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Invalid JSON body",
			Cause:   err,
		})
		return
	}

	err = h.validate.Struct(input)

	if err != nil {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Project name must be between 3 and 100 characters",
			Cause:   err,
		})
		return
	}

	project, err := h.service.Create(r.Context(), input.Name)

	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := httpx.WriteJSON(w, http.StatusCreated, project); err != nil {
		slog.Error("send project response", "error", err)
	}
}
