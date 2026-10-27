package application

import "errors"

var (
	ErrDuplicateApplication = errors.New("application already exists")
	ErrProjectNotFound      = errors.New("project not found")
	ErrPlanNotFound         = errors.New("plan not found")
)
