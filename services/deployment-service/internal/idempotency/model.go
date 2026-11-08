package idempotency

import "uuid"

type ProcessedRequest struct {
	Operation    string
	RequestID    uuid.UUID
	DeploymentID uuid.UUID
	Fingerprint  []byte
}
