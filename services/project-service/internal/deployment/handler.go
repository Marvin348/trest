package deployment

import (
	"log/slog"
	"net/http"
	"uuid"

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

func (h *Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	applicationID, err := uuid.Parse(r.PathValue("applicationId"))
	if err != nil || applicationID == uuid.Nil() {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Invalid applicationId",
			Cause:   err,
		})
		return
	}

	requestID, err := uuid.Parse(r.Header.Get("Idempotency-Key"))
	if err != nil || requestID == uuid.Nil() {
		httpx.WriteError(w, &apperror.AppError{
			Code:    apperror.Validation,
			Message: "Invalid Idempotency-Key",
			Cause:   err,
		})
		return
	}

	res, err := h.service.Deploy(r.Context(), applicationID, requestID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := httpx.WriteJSON(w, http.StatusCreated, res); err != nil {
		slog.Error("send deploy response", "error", err)
	}
}
