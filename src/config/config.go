package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const defaultHTTPAddress = "127.0.0.1:8080"

type Config struct {
	DatabaseURL    string
	HTTPAddress    string
	CORSAllowAll   bool
	APIDocsEnabled bool
	Superadmin     Superadmin
	Auth           Auth
	Mail           Mail
	Platform       Platform
	Credentials    Encryption
}

type Superadmin struct {
	Email    string
	Password string
	FullName string
}

type Encryption struct {
	KeyID string
	Key   []byte
}

type Auth struct {
	Issuer            string
	Audience          string
	AccessTTL         time.Duration
	SessionTTL        time.Duration
	OTPExpiry         time.Duration
	OTPResendCooldown time.Duration
	SigningKey        []byte
	EncryptionKey     []byte
	OTPHMACKey        []byte
	MailOutbox        Encryption
	LoginMaxAttempts  int
	LoginLockDuration time.Duration
	RateLimitWindow   time.Duration
	RateLimitMax      int
}

type Mail struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	UseTLS      bool
	UseSSL      bool
	WorkerPoll  time.Duration
	SendTimeout time.Duration
}

type Platform struct {
	EventRetention    time.Duration
	RealtimePoll      time.Duration
	RealtimeHeartbeat time.Duration
	RealtimeBatchSize int
	WorkerStaleAfter  time.Duration
	MaintenanceEvery  time.Duration
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}
	mailEnabled, err := boolOrDefault("MAIL_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	corsAllowAll, err := boolOrDefault("CORS_ALLOW_ALL", false)
	if err != nil {
		return Config{}, err
	}
	apiDocsEnabled, err := boolOrDefault("API_DOCS_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	smtpUseTLS, err := boolOrDefault("SMTP_USE_TLS", false)
	if err != nil {
		return Config{}, err
	}
	smtpUseSSL, err := boolOrDefault("SMTP_USE_SSL", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddress:    envOrDefault("HTTP_ADDR", defaultHTTPAddress),
		CORSAllowAll:   corsAllowAll,
		APIDocsEnabled: apiDocsEnabled,
		Superadmin: Superadmin{
			Email:    normalizeEmail(os.Getenv("SUPERADMIN_EMAIL")),
			Password: os.Getenv("SUPERADMIN_PASSWORD"),
			FullName: envOrDefault("SUPERADMIN_FULL_NAME", "Platform Superadmin"),
		},
		Auth: Auth{
			Issuer:            envOrDefault("AUTH_ISSUER", "ev-cms"),
			Audience:          envOrDefault("AUTH_AUDIENCE", "ev-cms-api"),
			AccessTTL:         durationOrDefault("AUTH_ACCESS_TTL", 15*time.Minute),
			SessionTTL:        durationOrDefault("AUTH_SESSION_TTL", 30*24*time.Hour),
			OTPExpiry:         durationOrDefault("AUTH_OTP_TTL", 10*time.Minute),
			OTPResendCooldown: durationOrDefault("AUTH_OTP_RESEND_COOLDOWN", time.Minute),
			LoginMaxAttempts:  intOrDefault("AUTH_LOGIN_MAX_ATTEMPTS", 5),
			LoginLockDuration: durationOrDefault("AUTH_LOGIN_LOCK_DURATION", 15*time.Minute),
			RateLimitWindow:   durationOrDefault("AUTH_RATE_LIMIT_WINDOW", 15*time.Minute),
			RateLimitMax:      intOrDefault("AUTH_RATE_LIMIT_MAX", 100),
		},
		Mail: Mail{
			Enabled:     mailEnabled,
			Host:        strings.TrimSpace(os.Getenv("SMTP_HOST")),
			Port:        intOrDefault("SMTP_PORT", 587),
			Username:    os.Getenv("SMTP_USERNAME"),
			Password:    os.Getenv("SMTP_PASSWORD"),
			FromAddress: normalizeEmail(os.Getenv("SMTP_FROM_ADDRESS")),
			FromName:    envOrDefault("SMTP_FROM_NAME", "TransEV CMS"),
			UseTLS:      smtpUseTLS,
			UseSSL:      smtpUseSSL,
			WorkerPoll:  durationOrDefault("MAIL_WORKER_POLL_INTERVAL", 2*time.Second),
			SendTimeout: durationOrDefault("MAIL_SEND_TIMEOUT", 15*time.Second),
		},
		Platform: Platform{
			EventRetention:    durationOrDefault("PLATFORM_EVENT_RETENTION", 7*24*time.Hour),
			RealtimePoll:      durationOrDefault("PLATFORM_REALTIME_POLL_INTERVAL", time.Second),
			RealtimeHeartbeat: durationOrDefault("PLATFORM_REALTIME_HEARTBEAT_INTERVAL", 15*time.Second),
			RealtimeBatchSize: intOrDefault("PLATFORM_REALTIME_BATCH_SIZE", 100),
			WorkerStaleAfter:  durationOrDefault("PLATFORM_WORKER_STALE_AFTER", 2*time.Minute),
			MaintenanceEvery:  durationOrDefault("PLATFORM_MAINTENANCE_INTERVAL", time.Minute),
		},
	}

	if cfg.Auth.SigningKey, err = decodeKey("JWT_SIGNING_KEY_B64", 32, false); err != nil {
		return Config{}, err
	}
	if cfg.Auth.EncryptionKey, err = decodeKey("JWT_ENCRYPTION_KEY_B64", 32, true); err != nil {
		return Config{}, err
	}
	if cfg.Auth.OTPHMACKey, err = decodeKey("OTP_HMAC_KEY_B64", 32, false); err != nil {
		return Config{}, err
	}
	if cfg.Auth.MailOutbox.Key, err = decodeKey("MAIL_OUTBOX_ENCRYPTION_KEY_B64", 32, true); err != nil {
		return Config{}, err
	}
	cfg.Auth.MailOutbox.KeyID = envOrDefault("MAIL_OUTBOX_ENCRYPTION_KEY_ID", "v1")
	if cfg.Credentials.Key, err = decodeKey("CREDENTIAL_ENCRYPTION_KEY_B64", 32, true); err != nil {
		return Config{}, err
	}
	cfg.Credentials.KeyID = envOrDefault("CREDENTIAL_ENCRYPTION_KEY_ID", "v1")

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	switch {
	case cfg.DatabaseURL == "":
		return errors.New("DATABASE_URL is required")
	case cfg.Superadmin.Email == "":
		return errors.New("SUPERADMIN_EMAIL is required")
	case !validEmail(cfg.Superadmin.Email):
		return errors.New("SUPERADMIN_EMAIL must be a valid email address")
	case cfg.Superadmin.Password == "":
		return errors.New("SUPERADMIN_PASSWORD is required")
	case len(cfg.Superadmin.Password) < 10:
		return errors.New("SUPERADMIN_PASSWORD must contain at least 10 characters")
	case cfg.Auth.Issuer == "" || cfg.Auth.Audience == "":
		return errors.New("AUTH_ISSUER and AUTH_AUDIENCE must not be blank")
	case cfg.Auth.AccessTTL <= 0 || cfg.Auth.SessionTTL <= cfg.Auth.AccessTTL:
		return errors.New("AUTH_SESSION_TTL must be longer than positive AUTH_ACCESS_TTL")
	case cfg.Auth.OTPExpiry <= 0 || cfg.Auth.OTPResendCooldown <= 0:
		return errors.New("OTP durations must be positive")
	case cfg.Auth.LoginMaxAttempts < 1 || cfg.Auth.RateLimitMax < 1:
		return errors.New("authentication attempt limits must be positive")
	case cfg.Mail.WorkerPoll <= 0 || cfg.Mail.SendTimeout <= 0:
		return errors.New("mail worker durations must be positive")
	case cfg.Platform.EventRetention <= 0:
		return errors.New("PLATFORM_EVENT_RETENTION must be positive")
	case cfg.Platform.RealtimePoll <= 0 ||
		cfg.Platform.RealtimeHeartbeat <= cfg.Platform.RealtimePoll:
		return errors.New("PLATFORM_REALTIME_HEARTBEAT_INTERVAL must be longer than positive PLATFORM_REALTIME_POLL_INTERVAL")
	case cfg.Platform.RealtimeBatchSize < 1 || cfg.Platform.RealtimeBatchSize > 500:
		return errors.New("PLATFORM_REALTIME_BATCH_SIZE must be between 1 and 500")
	case cfg.Platform.WorkerStaleAfter <= 0:
		return errors.New("PLATFORM_WORKER_STALE_AFTER must be positive")
	case cfg.Platform.MaintenanceEvery <= 0:
		return errors.New("PLATFORM_MAINTENANCE_INTERVAL must be positive")
	case cfg.Platform.WorkerStaleAfter <= cfg.Platform.MaintenanceEvery:
		return errors.New("PLATFORM_WORKER_STALE_AFTER must be longer than PLATFORM_MAINTENANCE_INTERVAL")
	}

	if cfg.Mail.Enabled {
		switch {
		case cfg.Mail.Host == "":
			return errors.New("SMTP_HOST is required when MAIL_ENABLED=true")
		case cfg.Mail.Port < 1 || cfg.Mail.Port > 65535:
			return errors.New("SMTP_PORT must be between 1 and 65535")
		case cfg.Mail.FromAddress == "":
			return errors.New("SMTP_FROM_ADDRESS is required when MAIL_ENABLED=true")
		case !validEmail(cfg.Mail.FromAddress):
			return errors.New("SMTP_FROM_ADDRESS must be a valid email address")
		case cfg.Mail.UseTLS == cfg.Mail.UseSSL:
			return errors.New("exactly one of SMTP_USE_TLS or SMTP_USE_SSL must be true")
		case (cfg.Mail.Username == "") != (cfg.Mail.Password == ""):
			return errors.New("SMTP_USERNAME and SMTP_PASSWORD must be set together")
		}
	}
	return nil
}

func validEmail(value string) bool {
	if len(value) > 320 {
		return false
	}
	address, err := netmail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationOrDefault(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}

func intOrDefault(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func boolOrDefault(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func decodeKey(name string, minimum int, exact bool) ([]byte, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be standard base64: %w", name, err)
	}
	if exact && len(key) != minimum {
		return nil, fmt.Errorf("%s must decode to exactly %d bytes", name, minimum)
	}
	if !exact && len(key) < minimum {
		return nil, fmt.Errorf("%s must decode to at least %d bytes", name, minimum)
	}
	return key, nil
}
