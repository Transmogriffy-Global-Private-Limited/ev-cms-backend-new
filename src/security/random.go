package security

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

func RandomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func RandomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random hex value: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func RandomDigits(length int) (string, error) {
	result := make([]byte, length)
	limit := big.NewInt(10)
	for index := range result {
		digit, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate random digit: %w", err)
		}
		result[index] = byte('0' + digit.Int64())
	}
	return string(result), nil
}
