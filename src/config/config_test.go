package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid configuration failed: %v", err)
	}

	cfg.Superadmin.Email = "not-an-email"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid superadmin email to fail")
	}
}

func TestMailConfigurationRequiresTLSAndPairedCredentials(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Mail.Enabled = true
	cfg.Mail.Host = "smtp.example.com"
	cfg.Mail.Port = 587
	cfg.Mail.FromAddress = "mailer@example.com"
	cfg.Mail.TLSMode = "PLAINTEXT"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SMTP_TLS_MODE") {
		t.Fatalf("got %v, want TLS mode error", err)
	}

	cfg.Mail.TLSMode = "STARTTLS"
	cfg.Mail.Username = "mailer"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SMTP_PASSWORD") {
		t.Fatalf("got %v, want SMTP password error", err)
	}
}

func validTestConfig() Config {
	return Config{
		DatabaseURL: "postgres://localhost/test",
		HTTPAddress: "127.0.0.1:8080",
		Superadmin: Superadmin{
			Email: "admin@example.com", Password: "a-long-password", FullName: "Admin",
		},
		Auth: Auth{
			Issuer:            "ev-cms",
			Audience:          "ev-cms-api",
			AccessTTL:         15 * time.Minute,
			SessionTTL:        24 * time.Hour,
			OTPExpiry:         10 * time.Minute,
			OTPResendCooldown: time.Minute,
			LoginMaxAttempts:  5,
			LoginLockDuration: 15 * time.Minute,
			RateLimitWindow:   15 * time.Minute,
			RateLimitMax:      20,
			SigningKey:        []byte(strings.Repeat("s", 32)),
			EncryptionKey:     []byte(strings.Repeat("e", 32)),
			OTPHMACKey:        []byte(strings.Repeat("o", 32)),
			MailOutbox:        Encryption{KeyID: "v1", Key: []byte(strings.Repeat("m", 32))},
		},
		Mail: Mail{
			TLSMode: "STARTTLS", WorkerPoll: time.Second, SendTimeout: 5 * time.Second,
		},
		Credentials: Encryption{
			KeyID: "v1", Key: []byte(strings.Repeat("c", 32)),
		},
	}
}
