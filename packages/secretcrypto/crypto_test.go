package secretcrypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestSecretCrypto(t *testing.T) {
	key := make([]byte, 32)

	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	t.Run("encrypts and decrypts successfully", func(t *testing.T) {
		applicationID := "app-123"
		variableName := "DATABASE_PASSWORD"

		testCases := []struct {
			name      string
			plaintext string
		}{
			{name: "empty", plaintext: ""},
			{name: "regular", plaintext: "my-secret-password"},
			{name: "unicode", plaintext: "Passwort-äöü-🔐"},
			{name: "multiline", plaintext: "line one\nline two"},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				encrypted, err := Encrypt(
					testCase.plaintext,
					key,
					"env-key-v1",
					applicationID,
					variableName,
				)
				if err != nil {
					t.Fatal(err)
				}

				decrypted, err := Decrypt(encrypted, key, applicationID, variableName)
				if err != nil {
					t.Fatal(err)
				}

				if decrypted != testCase.plaintext {
					t.Fatalf("expected %q, got %q", testCase.plaintext, decrypted)
				}
			})
		}
	})

	t.Run("rejects wrong application ID", func(t *testing.T) {
		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Decrypt(
			encrypted,
			key,
			"app-999",
			"DATABASE_PASSWORD",
		)

		if err == nil {
			t.Fatal("expected decryption to fail with wrong application ID")
		}
	})

	t.Run("rejects tampered ciphertext", func(t *testing.T) {
		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		ciphertext, err := base64.StdEncoding.DecodeString(encrypted.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}

		ciphertext[0] ^= 0x01
		encrypted.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)

		_, err = Decrypt(
			encrypted,
			key,
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err == nil {
			t.Fatal("expected authenticated decryption to reject tampered ciphertext")
		}
	})

	t.Run("rejects tampered nonce", func(t *testing.T) {
		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		nonce, err := base64.StdEncoding.DecodeString(encrypted.Nonce)
		if err != nil {
			t.Fatal(err)
		}

		nonce[0] ^= 0x01
		encrypted.Nonce = base64.StdEncoding.EncodeToString(nonce)

		_, err = Decrypt(
			encrypted,
			key,
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err == nil {
			t.Fatal("expected authenticated decryption to reject tampered nonce")
		}
	})

	t.Run("rejects wrong key", func(t *testing.T) {
		wrongKey := make([]byte, 32)

		if _, err := rand.Read(wrongKey); err != nil {
			t.Fatal(err)
		}

		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Decrypt(
			encrypted,
			wrongKey,
			"app-123",
			"DATABASE_PASSWORD",
		)

		if err == nil {
			t.Fatal("expected decryption to fail with wrong key")
		}
	})

	t.Run("rejects invalid AES key lengths", func(t *testing.T) {
		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		for _, keyLength := range []int{0, 15, 17, 23, 25, 31, 33} {
			invalidKey := make([]byte, keyLength)

			t.Run(fmt.Sprintf("encrypt/%d_bytes", keyLength), func(t *testing.T) {
				_, err := Encrypt(
					"my-secret-password",
					invalidKey,
					"env-key-v1",
					"app-123",
					"DATABASE_PASSWORD",
				)
				if err == nil {
					t.Fatalf("expected encryption to reject a %d-byte key", keyLength)
				}
			})

			t.Run(fmt.Sprintf("decrypt/%d_bytes", keyLength), func(t *testing.T) {
				_, err := Decrypt(
					encrypted,
					invalidKey,
					"app-123",
					"DATABASE_PASSWORD",
				)
				if err == nil {
					t.Fatalf("expected decryption to reject a %d-byte key", keyLength)
				}
			})
		}
	})

	t.Run("rejects wrong env name", func(t *testing.T) {
		encrypted, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Decrypt(
			encrypted,
			key,
			"app-123",
			"WRONG NAME",
		)

		if err == nil {
			t.Fatal("expected decryption to fail with wrong env name")
		}
	})

	t.Run("uses a different nonce for each encryption", func(t *testing.T) {
		first, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		second, err := Encrypt(
			"my-secret-password",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		if first.Nonce == second.Nonce {
			t.Fatal("expected encryptions to use different nonces")
		}

		if first.Ciphertext == second.Ciphertext {
			t.Fatal("expected encryptions to produce different ciphertexts")
		}
	})

	t.Run("rejects unsupported version", func(t *testing.T) {
		encrypted, err := Encrypt(
			"secret",
			key,
			"env-key-v1",
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err != nil {
			t.Fatal(err)
		}

		encrypted.Version = 999

		_, err = Decrypt(
			encrypted,
			key,
			"app-123",
			"DATABASE_PASSWORD",
		)
		if err == nil {
			t.Fatal("expected unsupported version to be rejected")
		}
	})

	t.Run("rejects malformed encrypted values", func(t *testing.T) {
		testCases := []struct {
			name   string
			mutate func(t *testing.T, value *EncryptedValue)
		}{
			{
				name: "invalid nonce base64",
				mutate: func(_ *testing.T, value *EncryptedValue) {
					value.Nonce = "%%%"
				},
			},
			{
				name: "invalid ciphertext base64",
				mutate: func(_ *testing.T, value *EncryptedValue) {
					value.Ciphertext = "%%%"
				},
			},
			{
				name: "wrong nonce length",
				mutate: func(t *testing.T, value *EncryptedValue) {
					t.Helper()

					nonce, err := base64.StdEncoding.DecodeString(value.Nonce)
					if err != nil {
						t.Fatal(err)
					}

					value.Nonce = base64.StdEncoding.EncodeToString(nonce[:len(nonce)-1])
				},
			},
			{
				name: "empty ciphertext",
				mutate: func(_ *testing.T, value *EncryptedValue) {
					value.Ciphertext = ""
				},
			},
			{
				name: "truncated ciphertext",
				mutate: func(t *testing.T, value *EncryptedValue) {
					t.Helper()

					ciphertext, err := base64.StdEncoding.DecodeString(value.Ciphertext)
					if err != nil {
						t.Fatal(err)
					}

					value.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext[:len(ciphertext)-1])
				},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				encrypted, err := Encrypt(
					"my-secret-password",
					key,
					"env-key-v1",
					"app-123",
					"DATABASE_PASSWORD",
				)
				if err != nil {
					t.Fatal(err)
				}

				testCase.mutate(t, &encrypted)

				_, err = Decrypt(
					encrypted,
					key,
					"app-123",
					"DATABASE_PASSWORD",
				)
				if err == nil {
					t.Fatal("expected malformed encrypted value to be rejected")
				}
			})
		}
	})
}
