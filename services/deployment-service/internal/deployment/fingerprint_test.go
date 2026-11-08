package deployment

import (
	"testing"
	"uuid"

	"github.com/Marvin348/trest/packages/secretcrypto"
	"github.com/stretchr/testify/require"
)

func TestFingerprintCreateDeployment(t *testing.T) {
	t.Run("same request produces same fingerprint", func(t *testing.T) {
		params := fingerprintTestParams()

		first, firstErr := fingerprintCreateDeployment(params)
		second, secondErr := fingerprintCreateDeployment(params)

		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		require.Equal(t, first, second)
	})

	t.Run("environment map order does not affect fingerprint", func(t *testing.T) {
		apiKey := encryptedTestValue("api-key")
		databaseURL := encryptedTestValue("database-url")

		firstParams := fingerprintTestParams()
		firstParams.Environment = make(map[string]secretcrypto.EncryptedValue)
		firstParams.Environment["API_KEY"] = apiKey
		firstParams.Environment["DATABASE_URL"] = databaseURL

		secondParams := fingerprintTestParams()
		secondParams.Environment = make(map[string]secretcrypto.EncryptedValue)
		secondParams.Environment["DATABASE_URL"] = databaseURL
		secondParams.Environment["API_KEY"] = apiKey

		first, firstErr := fingerprintCreateDeployment(firstParams)
		second, secondErr := fingerprintCreateDeployment(secondParams)

		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		require.Equal(t, first, second)
	})

	t.Run("different environment changes fingerprint", func(t *testing.T) {
		firstParams := fingerprintTestParams()
		firstParams.Environment = map[string]secretcrypto.EncryptedValue{
			"API_KEY": encryptedTestValue("first-value"),
		}

		secondParams := fingerprintTestParams()
		secondParams.Environment = map[string]secretcrypto.EncryptedValue{
			"API_KEY": encryptedTestValue("second-value"),
		}

		first, firstErr := fingerprintCreateDeployment(firstParams)
		second, secondErr := fingerprintCreateDeployment(secondParams)

		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		require.NotEqual(t, first, second)
	})

	t.Run("different deployment data changes fingerprint", func(t *testing.T) {
		firstParams := fingerprintTestParams()
		secondParams := fingerprintTestParams()
		secondParams.Image = "caddy:2.10"

		first, firstErr := fingerprintCreateDeployment(firstParams)
		second, secondErr := fingerprintCreateDeployment(secondParams)

		require.NoError(t, firstErr)
		require.NoError(t, secondErr)
		require.NotEqual(t, first, second)
	})
}

func fingerprintTestParams() CreateDeploymentParams {
	return CreateDeploymentParams{
		RequestID:       uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		ProjectID:       uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		ApplicationID:   uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Image:           "nginx:1.27",
		Replicas:        3,
		ContainerPort:   8080,
		CPURequestM:     100,
		CPULimitM:       250,
		MemoryRequestMi: 256,
		MemoryLimitMi:   512,
	}
}

func encryptedTestValue(value string) secretcrypto.EncryptedValue {
	return secretcrypto.EncryptedValue{
		Version:    1,
		KeyID:      "test-key",
		Nonce:      "test-nonce",
		Ciphertext: value,
	}
}
