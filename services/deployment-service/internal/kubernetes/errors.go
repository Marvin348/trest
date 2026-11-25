package kubernetes

import "errors"

var (
	ErrDeploymentMismatch = errors.New(
		"existing kubernetes deployment does not match requested configuration",
	)
	ErrSecretMismatch = errors.New("existing kubernetes secret does not match requested data")
)
