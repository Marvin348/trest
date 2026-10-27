package env

import "time"

type EnvironmentVariable struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Key           string    `json:"key"`
	Value         string    `json:"value"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
