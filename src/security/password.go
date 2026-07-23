package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 4
	argonSaltLength  = 16
	argonKeyLength   = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

func ValidatePassword(password string) error {
	if len(password) < 10 {
		return errors.New("password must contain at least 10 characters")
	}
	if len(password) > 128 {
		return errors.New("password must contain at most 128 characters")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		argonKeyLength,
	)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrInvalidPasswordHash
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 ||
		!strings.HasPrefix(parameters[0], "m=") ||
		!strings.HasPrefix(parameters[1], "t=") ||
		!strings.HasPrefix(parameters[2], "p=") {
		return false, ErrInvalidPasswordHash
	}
	parsedMemory, err := strconv.ParseUint(strings.TrimPrefix(parameters[0], "m="), 10, 32)
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	parsedIterations, err := strconv.ParseUint(strings.TrimPrefix(parameters[1], "t="), 10, 32)
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	parsedParallelism, err := strconv.ParseUint(strings.TrimPrefix(parameters[2], "p="), 10, 8)
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	memory = uint32(parsedMemory)
	iterations = uint32(parsedIterations)
	parallelism = uint8(parsedParallelism)
	if memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, ErrInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, ErrInvalidPasswordHash
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
