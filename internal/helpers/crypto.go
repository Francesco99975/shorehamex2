package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

func ComputeFileChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])
	return checksum
}

func ParseBase64Key(keyStr string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(plaintext []byte, key []byte) (string, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(ciphertext string, key []byte) ([]byte, error) {
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	nonce, ciphertext2 := ciphertextBytes[:nonceSize], ciphertextBytes[nonceSize:]
	return aead.Open(nil, nonce, ciphertext2, nil)
}
