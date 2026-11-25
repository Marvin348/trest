package crypto

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/Marvin348/trest/packages/secretcrypto"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentKeyProviderDecryptsWithRequestedKey(t *testing.T) {
	oldKey := bytes.Repeat([]byte{1}, 16)
	currentKey := bytes.Repeat([]byte{2}, 32)
	values, err := json.Marshal(map[string]string{
		"old":     base64.StdEncoding.EncodeToString(oldKey),
		"current": base64.StdEncoding.EncodeToString(currentKey),
	})
	require.NoError(t, err)
	t.Setenv("DEPLOYMENT_ENCRYPTION_KEYS", string(values))
	provider, err := NewEnvironmentKeyProvider()
	require.NoError(t, err)

	for keyID, key := range map[string][]byte{"old": oldKey, "current": currentKey} {
		encrypted, err := secretcrypto.Encrypt("secret", key, keyID, "app", "TOKEN")
		require.NoError(t, err)
		loadedKey, err := provider.GetKey(encrypted.KeyID)
		require.NoError(t, err)
		plaintext, err := secretcrypto.Decrypt(encrypted, loadedKey, "app", "TOKEN")
		require.NoError(t, err)
		require.Equal(t, "secret", plaintext)
		loadedKey[0] = 0
		unchangedKey, err := provider.GetKey(keyID)
		require.NoError(t, err)
		require.Equal(t, key, unchangedKey)
	}
	_, err = provider.GetKey("missing")
	require.ErrorContains(t, err, "not configured")
}

func TestEnvironmentKeyProviderRejectsInvalidConfiguration(t *testing.T) {
	for _, value := range []string{
		`invalid-json`, `null`, `[]`, `{"key":42}`,
		`{"":"YQ=="}`, `{"key":"not-base64"}`, `{"key":"YQ=="}`,
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DEPLOYMENT_ENCRYPTION_KEYS", value)
			_, err := NewEnvironmentKeyProvider()
			require.Error(t, err)
		})
	}
}

func TestEnvironmentKeyProviderAllowsNoConfiguredKeys(t *testing.T) {
	for _, value := range []string{"", "{}"} {
		t.Setenv("DEPLOYMENT_ENCRYPTION_KEYS", value)
		provider, err := NewEnvironmentKeyProvider()
		require.NoError(t, err)
		_, err = provider.GetKey("missing")
		require.ErrorContains(t, err, "not configured")
	}
}
