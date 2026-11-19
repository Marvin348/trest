package deployment

import "errors"

var (
	ErrAlreadyExists       = errors.New("deployment already exists")
	ErrNotFound            = errors.New("deployment not found")
	ErrInvalidDeployment   = errors.New("invalid deployment")
	ErrIdempotencyConflict = errors.New(
		"request ID already used for a different deployment")
	ErrInvalidTransition = errors.New("invalid deployment transition")
	ErrNoEvents          = errors.New("deployment has no events")
)
