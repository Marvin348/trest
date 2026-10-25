package apperror

import "fmt"

type Code string

const (
	NotFound   Code = "NOT_FOUND"
	Conflict   Code = "CONFLICT"
	Forbidden  Code = "FORBIDDEN"
	Validation Code = "VALIDATION_ERROR"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}

	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}
