package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const secretBoxVersion byte = 1

type SecretBox struct {
	keyID string
	aead  cipher.AEAD
}

func NewSecretBox(keyID string, key []byte) (*SecretBox, error) {
	if keyID == "" {
		return nil, errors.New("encryption key ID is required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize AES-GCM: %w", err)
	}
	return &SecretBox{keyID: keyID, aead: aead}, nil
}

func (box *SecretBox) KeyID() string {
	return box.keyID
}

func (box *SecretBox) Seal(plaintext, additionalData []byte) ([]byte, error) {
	nonce := make([]byte, box.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	output := make([]byte, 1, 1+len(nonce)+len(plaintext)+box.aead.Overhead())
	output[0] = secretBoxVersion
	output = append(output, nonce...)
	output = box.aead.Seal(output, nonce, plaintext, additionalData)
	return output, nil
}

func (box *SecretBox) Open(ciphertext, additionalData []byte) ([]byte, error) {
	if len(ciphertext) < 1+box.aead.NonceSize()+box.aead.Overhead() ||
		ciphertext[0] != secretBoxVersion {
		return nil, errors.New("invalid encrypted payload")
	}
	nonceEnd := 1 + box.aead.NonceSize()
	plaintext, err := box.aead.Open(
		nil,
		ciphertext[1:nonceEnd],
		ciphertext[nonceEnd:],
		additionalData,
	)
	if err != nil {
		return nil, errors.New("decrypt payload")
	}
	return plaintext, nil
}
