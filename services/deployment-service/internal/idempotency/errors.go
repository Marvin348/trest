package idempotency

import "errors"

var (
	ErrAlreadyProcessed = errors.New("request already processed")
	ErrNotFound         = errors.New("processed request not found")
)
