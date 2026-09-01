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

	cfg = validTestConfig()
	cfg.LogLevel = "TRACE"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("got %v, want invalid log level error", err)
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
		"CORS_ALLOW_ALL":                 "true",
		"API_DOCS_ENABLED":               "false",
		"LOG_LEVEL":                      "DEBUG",
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
	if !cfg.CORSAllowAll {
		t.Fatal("expected CORS_ALLOW_ALL=true to enable permissive CORS")
	}
	if cfg.APIDocsEnabled {
		t.Fatal("expected API_DOCS_ENABLED=false to disable API documentation")
	}
	if cfg.LogLevel != LogLevelDebug {
		t.Fatalf("log level = %q, want DEBUG", cfg.LogLevel)
	}
}

func TestAPIDocumentationDefaultsEnabled(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	if !cfg.APIDocsEnabled {
		t.Fatal("API documentation must default enabled for backward compatibility")
	}
}

func validTestConfig() Config {
	return Config{
		DatabaseURL:    "postgres://localhost/test",
		HTTPAddress:    "127.0.0.1:8080",
		APIDocsEnabled: true,
		LogLevel:       LogLevelInfo,
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
			UseTLS: true, WorkerPoll: time.Second, SendTimeout: 5 * time.Second, DisplayLocation: time.UTC,
		},
		Frontend: FrontendLinks{
			AdminLoginVerifyTemplate:      "https://cms.example.invalid/auth/verify#challenge_id={challenge_id}",
			AdminPasswordResetTemplate:    "https://cms.example.invalid/auth/reset-password#challenge_id={challenge_id}",
			CustomerLoginVerifyTemplate:   "https://app.example.invalid/auth/verify#challenge_id={challenge_id}",
			CustomerSignupVerifyTemplate:  "https://app.example.invalid/auth/verify-signup#challenge_id={challenge_id}",
			CustomerPasswordResetTemplate: "https://app.example.invalid/auth/reset-password#challenge_id={challenge_id}",
			CPOOnboardingTemplate:         "https://cms.example.invalid/login#cpo_id={cpo_id}",
			CPOSupportTicketTemplate:      "https://cms.example.invalid/support/tickets/{ticket_id}",
			CPOSubscriptionURL:            "https://cms.example.invalid/subscription",
		},
		Platform: Platform{
			EventRetention:    7 * 24 * time.Hour,
			TraceRetention:    30 * 24 * time.Hour,
			RealtimePoll:      time.Second,
			RealtimeHeartbeat: 15 * time.Second,
			RealtimeBatchSize: 100,
			WorkerStaleAfter:  2 * time.Minute,
			MaintenanceEvery:  time.Minute,
		},
		Credentials: Encryption{
			KeyID: "v1", Key: []byte(strings.Repeat("c", 32)),
		},
	}
}

func TestPlatformOperationalConfigurationValidation(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Platform.RealtimeHeartbeat = cfg.Platform.RealtimePoll
	if err := cfg.Validate(); err == nil ||
		!strings.Contains(err.Error(), "HEARTBEAT_INTERVAL") {
		t.Fatalf("got %v, want realtime interval validation", err)
	}

	cfg = validTestConfig()
	cfg.Platform.RealtimeBatchSize = 501
	if err := cfg.Validate(); err == nil ||
		!strings.Contains(err.Error(), "BATCH_SIZE") {
		t.Fatalf("got %v, want realtime batch validation", err)
	}

	cfg = validTestConfig()
	cfg.Platform.WorkerStaleAfter = cfg.Platform.MaintenanceEvery
	if err := cfg.Validate(); err == nil ||
		!strings.Contains(err.Error(), "WORKER_STALE_AFTER") {
		t.Fatalf("got %v, want worker stale interval validation", err)
	}
}

func TestHALConnectionFreshnessConfigurationValidation(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.HAL = HAL{
		BaseURL:              "http://127.0.0.1:29080",
		CMSBearerToken:       "cms-test-token",
		FactBearerToken:      "fact-test-token",
		RequestTimeout:       5 * time.Second,
		MeterStaleAfter:      30 * time.Second,
		ConnectionStaleAfter: 15 * time.Minute,
		StartReconcileAfter:  2 * time.Minute,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid HAL freshness configuration failed: %v", err)
	}
	cfg.HAL.ConnectionStaleAfter = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "HAL v1 durations") {
		t.Fatalf("connection stale duration validation=%v", err)
	}
}
