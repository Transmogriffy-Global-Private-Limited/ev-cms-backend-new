package config

import (
	"encoding/base64"
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

func TestMailConfigurationRequiresOneEncryptedTransportAndPairedCredentials(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Mail.Enabled = true
	cfg.Mail.Host = "smtp.example.com"
	cfg.Mail.Port = 587
	cfg.Mail.FromAddress = "mailer@example.com"
	cfg.Mail.UseTLS = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("got %v, want missing encrypted transport error", err)
	}

	cfg.Mail.UseTLS = true
	cfg.Mail.UseSSL = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("got %v, want conflicting encrypted transport error", err)
	}

	cfg.Mail.UseSSL = false
	cfg.Mail.Username = "mailer"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "set together") {
		t.Fatalf("got %v, want paired SMTP credential error", err)
	}
}

func TestLoadHostingerImplicitSSLConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	environment := map[string]string{
		"DATABASE_URL":                   "postgres://localhost/test",
		"SUPERADMIN_EMAIL":               "admin@example.com",
		"SUPERADMIN_PASSWORD":            "a-long-password",
		"JWT_SIGNING_KEY_B64":            key,
		"JWT_ENCRYPTION_KEY_B64":         key,
		"OTP_HMAC_KEY_B64":               key,
		"MAIL_OUTBOX_ENCRYPTION_KEY_B64": key,
		"CREDENTIAL_ENCRYPTION_KEY_B64":  key,
		"MAIL_ENABLED":                   "true",
		"SMTP_HOST":                      "smtp.hostinger.com",
		"SMTP_PORT":                      "465",
		"SMTP_USERNAME":                  "team@transev.in",
		"SMTP_PASSWORD":                  "test-only-password",
		"SMTP_FROM_ADDRESS":              "team@transev.in",
		"SMTP_USE_TLS":                   "false",
		"SMTP_USE_SSL":                   "true",
	}
	for name, value := range environment {
		t.Setenv(name, value)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load Hostinger SMTP configuration: %v", err)
	}
	if cfg.Mail.Host != "smtp.hostinger.com" ||
		cfg.Mail.Port != 465 ||
		cfg.Mail.UseTLS ||
		!cfg.Mail.UseSSL {
		t.Fatalf("unexpected Hostinger SMTP transport: %#v", cfg.Mail)
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
			UseTLS: true, WorkerPoll: time.Second, SendTimeout: 5 * time.Second,
		},
		Credentials: Encryption{
			KeyID: "v1", Key: []byte(strings.Repeat("c", 32)),
		},
	}
}
