package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"example.com/trest/services/project-service/internal/apperror"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, err error) {
	var appErr *apperror.AppError

	if !errors.As(err, &appErr) {
		writeJSONError(
			w,
			http.StatusInternalServerError,
			"INTERNAL_ERROR",
			"Internal server error",
		)
		return
	}

	status := http.StatusInternalServerError

	switch appErr.Code {
	case apperror.NotFound:
		status = http.StatusNotFound
	case apperror.Conflict:
		status = http.StatusConflict
	case apperror.Forbidden:
		status = http.StatusForbidden
	case apperror.Validation:
		status = http.StatusBadRequest
	}

	writeJSONError(w, status, string(appErr.Code), appErr.Message)
}

func writeJSONError(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
