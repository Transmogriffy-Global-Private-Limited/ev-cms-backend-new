package security

import (
	"bytes"
	"testing"
)

func TestSecretBoxBindsAdditionalData(t *testing.T) {
	t.Parallel()

	box, err := NewSecretBox("v1", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("create secret box: %v", err)
	}
	plaintext := []byte("sensitive-value")
	aad := []byte("tenant-a:RAZORPAY")
	ciphertext, err := box.Seal(plaintext, aad)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	opened, err := box.Open(ciphertext, aad)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("got %q, want %q", opened, plaintext)
	}
	if _, err := box.Open(ciphertext, []byte("tenant-b:RAZORPAY")); err == nil {
		t.Fatal("expected different additional data to fail")
	}
}

func TestSecretBoxRejectsInvalidKeyLength(t *testing.T) {
	t.Parallel()

	if _, err := NewSecretBox("v1", []byte("short")); err == nil {
		t.Fatal("expected invalid AES key length to fail")
	}
}
