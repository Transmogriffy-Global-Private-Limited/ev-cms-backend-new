package security

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if strings.Contains(encoded, "correct horse") {
		t.Fatal("password hash contains plaintext")
	}
	matches, err := VerifyPassword("correct horse battery staple", encoded)
	if err != nil {
		t.Fatalf("verify correct password: %v", err)
	}
	if !matches {
		t.Fatal("correct password did not match")
	}
	matches, err = VerifyPassword("incorrect password", encoded)
	if err != nil {
		t.Fatalf("verify incorrect password: %v", err)
	}
	if matches {
		t.Fatal("incorrect password matched")
	}
}

func TestPasswordValidationAndMalformedHash(t *testing.T) {
	t.Parallel()

	if err := ValidatePassword("too-short"); err == nil {
		t.Fatal("expected short password to fail")
	}
	if _, err := VerifyPassword("irrelevant", "not-a-phc-hash"); !errors.Is(err, ErrInvalidPasswordHash) {
		t.Fatalf("got %v, want ErrInvalidPasswordHash", err)
	}
}
