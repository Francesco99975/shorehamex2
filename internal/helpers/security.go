package helpers

import (
	"crypto/rand"
	"encoding/base64"
	"io"
)

func GenerateNonce() (string, error) {
	bytes := make([]byte, 16) // 16 bytes nonce
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// Internal function — can be tested with fake reader
func generateNonceWithReader(reader io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(reader, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
