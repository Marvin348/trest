package env

import (
	"github.com/Marvin348/trest/packages/secretcrypto"
	"time"
)

type EnvironmentVariable struct {
	ID            string                      `json:"id"`
	ApplicationID string                      `json:"application_id"`
	Key           string                      `json:"key"`
	Value         secretcrypto.EncryptedValue `json:"-"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}
