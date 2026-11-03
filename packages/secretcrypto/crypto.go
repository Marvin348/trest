package secretcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func Encrypt(plaintext string, key []byte, keyID string,
	applicationID string, variableName string,
) (EncryptedValue, error) {

	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedValue{}, fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedValue{}, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	if _, err := rand.Read(nonce); err != nil {
		return EncryptedValue{}, fmt.Errorf("generate nonce: %w", err)
	}

	associatedData := buildAssociatedData(applicationID, variableName)

	ciphertext := gcm.Seal(
		nil,
		nonce,
		[]byte(plaintext),
		associatedData,
	)

	return EncryptedValue{
		Version:    1,
		KeyID:      keyID,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func Decrypt(value EncryptedValue, key []byte,
	applicationID string,
	variableName string) (string, error) {

	if value.Version != 1 {
		return "", fmt.Errorf("unsupported encryption version: %d", value.Version)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(value.Nonce)
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(value.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid nonce size")
	}

	associatedData := buildAssociatedData(applicationID, variableName)

	plaintext, err := gcm.Open(
		nil,
		nonce,
		ciphertext,
		associatedData,
	)
	if err != nil {
		return "", fmt.Errorf("decrypt environment variable: %w", err)
	}

	return string(plaintext), nil
}

func buildAssociatedData(applicationID, variableName string) []byte {
	return []byte(
		fmt.Appendf(nil, "%d:%s%d:%s",
			len(applicationID), applicationID,
			len(variableName), variableName,
		),
	)
}
