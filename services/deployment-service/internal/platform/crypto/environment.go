package crypto

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EnvironmentKeyProvider loads a JSON object mapping key IDs to base64 AES keys.
type EnvironmentKeyProvider struct {
	keys map[string][]byte
}

func NewEnvironmentKeyProvider() (*EnvironmentKeyProvider, error) {
	provider := &EnvironmentKeyProvider{keys: make(map[string][]byte)}
	encodedKeys := os.Getenv("DEPLOYMENT_ENCRYPTION_KEYS")
	if strings.TrimSpace(encodedKeys) == "" {
		return provider, nil
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(encodedKeys), &values); err != nil || values == nil {
		return nil, fmt.Errorf("DEPLOYMENT_ENCRYPTION_KEYS must be a JSON object of key IDs and base64 keys")
	}
	for keyID, encodedKey := range values {
		if strings.TrimSpace(keyID) == "" {
			return nil, fmt.Errorf("encryption key ID must not be empty")
		}
		key, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil {
			return nil, fmt.Errorf("encryption key %q must be base64 encoded", keyID)
		}
		if _, err := aes.NewCipher(key); err != nil {
			return nil, fmt.Errorf("encryption key %q must contain 16, 24, or 32 bytes", keyID)
		}
		provider.keys[keyID] = key
	}
	return provider, nil
}

func (p *EnvironmentKeyProvider) GetKey(keyID string) ([]byte, error) {
	key, ok := p.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("encryption key %q is not configured", keyID)
	}
	return append([]byte(nil), key...), nil
}
