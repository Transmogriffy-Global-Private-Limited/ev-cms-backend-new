package security

import "testing"

func TestRandomDigitsShape(t *testing.T) {
	t.Parallel()

	value, err := RandomDigits(6)
	if err != nil {
		t.Fatalf("generate digits: %v", err)
	}
	if len(value) != 6 {
		t.Fatalf("got length %d, want 6", len(value))
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			t.Fatalf("got non-digit %q", character)
		}
	}
}

func TestRandomTokensDiffer(t *testing.T) {
	t.Parallel()

	first, err := RandomToken(32)
	if err != nil {
		t.Fatalf("generate first token: %v", err)
	}
	second, err := RandomToken(32)
	if err != nil {
		t.Fatalf("generate second token: %v", err)
	}
	if first == second {
		t.Fatal("independent random tokens matched")
	}
}
