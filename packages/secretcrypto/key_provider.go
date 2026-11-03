package secretcrypto

type KeyProvider interface {
	GetKey(keyID string) ([]byte, error)
}
