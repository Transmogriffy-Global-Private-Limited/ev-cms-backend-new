package models

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

type AuthChallenge struct {
	ID                uuid.UUID                      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID                      `gorm:"type:uuid;not null;index" json:"user_id"`
	Purpose           constants.AuthChallengePurpose `gorm:"type:varchar(30);not null" json:"purpose"`
	Scope             *constants.AuthScope           `gorm:"type:varchar(20)" json:"scope,omitempty"`
	CPOID             *uuid.UUID                     `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	CodeHash          []byte                         `gorm:"type:bytea;not null" json:"-"`
	ExpiresAt         time.Time                      `gorm:"not null;index" json:"expires_at"`
	ConsumedAt        *time.Time                     `json:"consumed_at,omitempty"`
	InvalidatedAt     *time.Time                     `json:"invalidated_at,omitempty"`
	Attempts          int                            `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts       int                            `gorm:"not null" json:"max_attempts"`
	ResendAvailableAt time.Time                      `gorm:"not null" json:"resend_available_at"`
	RequestIP         *string                        `gorm:"type:inet" json:"-"`
	UserAgent         string                         `gorm:"type:varchar(512);not null;default:''" json:"-"`
	CreatedAt         time.Time                      `gorm:"not null" json:"created_at"`
}

type AuthSession struct {
	ID           uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID           `gorm:"type:uuid;not null;index" json:"user_id"`
	Scope        constants.AuthScope `gorm:"type:varchar(20);not null" json:"scope"`
	CPOID        *uuid.UUID          `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	CustomerID   *uuid.UUID          `gorm:"type:uuid;index" json:"customer_id,omitempty"`
	Role         *constants.CPORole  `gorm:"type:varchar(20)" json:"role,omitempty"`
	TokenVersion int                 `gorm:"not null;default:1" json:"-"`
	IPAddress    *string             `gorm:"type:inet" json:"ip_address,omitempty"`
	UserAgent    string              `gorm:"type:varchar(512);not null;default:''" json:"user_agent"`
	CreatedAt    time.Time           `gorm:"not null" json:"created_at"`
	LastSeenAt   time.Time           `gorm:"not null" json:"last_seen_at"`
	ExpiresAt    time.Time           `gorm:"not null;index" json:"expires_at"`
	RevokedAt    *time.Time          `json:"revoked_at,omitempty"`
	RevokeReason *string             `gorm:"type:varchar(100)" json:"-"`
}

type AuthRefreshToken struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	TokenHash     string     `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	ExpiresAt     time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	ReplacementID *uuid.UUID `gorm:"type:uuid" json:"replacement_id,omitempty"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
}

type MailOutbox struct {
	ID                uuid.UUID                  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ToEmail           string                     `gorm:"type:varchar(320);not null" json:"-"`
	CPOID             *uuid.UUID                 `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	UserID            *uuid.UUID                 `gorm:"type:uuid;index" json:"user_id,omitempty"`
	Template          string                     `gorm:"type:varchar(50);not null" json:"template"`
	PayloadCiphertext []byte                     `gorm:"type:bytea;not null" json:"-"`
	EncryptionKeyID   string                     `gorm:"type:varchar(50);not null" json:"-"`
	Status            constants.MailOutboxStatus `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"status"`
	Attempts          int                        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts       int                        `gorm:"not null;default:8" json:"max_attempts"`
	AvailableAt       time.Time                  `gorm:"not null;index" json:"available_at"`
	LockedAt          *time.Time                 `json:"locked_at,omitempty"`
	LastError         *string                    `gorm:"type:varchar(500)" json:"-"`
	SentAt            *time.Time                 `json:"sent_at,omitempty"`
	CreatedAt         time.Time                  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time                  `gorm:"not null" json:"updated_at"`
}

type PlatformAnnouncement struct {
	ID              uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Audience        string                 `gorm:"type:varchar(20);not null" json:"audience"`
	CPOID           *uuid.UUID             `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	Title           string                 `gorm:"type:varchar(200);not null" json:"title"`
	Body            string                 `gorm:"type:text;not null" json:"body"`
	CreatedByUserID uuid.UUID              `gorm:"type:uuid;not null" json:"created_by_user_id"`
	CreatedAt       time.Time              `gorm:"not null" json:"created_at"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty"`
	Notifications   []PlatformNotification `gorm:"foreignKey:AnnouncementID" json:"-"`
}

type PlatformNotification struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnnouncementID  uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_platform_notification_recipient,priority:1" json:"announcement_id"`
	RecipientUserID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_platform_notification_recipient,priority:2;index" json:"recipient_user_id"`
	CPOID           *uuid.UUID `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	ReadAt          *time.Time `json:"read_at,omitempty"`
}

func (MailOutbox) TableName() string {
	return "mail_outbox"
}

type AuthRateLimit struct {
	ScopeKey        string     `gorm:"type:char(64);primaryKey" json:"-"`
	Action          string     `gorm:"type:varchar(50);primaryKey" json:"-"`
	WindowStartedAt time.Time  `gorm:"not null" json:"-"`
	AttemptCount    int        `gorm:"not null;default:0" json:"-"`
	BlockedUntil    *time.Time `json:"-"`
	UpdatedAt       time.Time  `gorm:"not null" json:"-"`
}

type CustomerSignupChallenge struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID             uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Email             string    `gorm:"type:varchar(320);not null" json:"email"`
	PasswordHash      string    `gorm:"type:varchar(255);not null" json:"-"`
	FullName          string    `gorm:"type:varchar(255);not null" json:"full_name"`
	Phone             *string   `gorm:"type:varchar(32)" json:"phone,omitempty"`
	CodeHash          []byte    `gorm:"type:bytea;not null" json:"-"`
	ExpiresAt         time.Time `gorm:"not null;index" json:"expires_at"`
	ConsumedAt        *time.Time
	InvalidatedAt     *time.Time
	Attempts          int       `gorm:"not null;default:0"`
	MaxAttempts       int       `gorm:"not null"`
	ResendAvailableAt time.Time `gorm:"not null" json:"resend_available_at"`
	RequestIP         *string   `gorm:"type:inet" json:"-"`
	UserAgent         string    `gorm:"type:varchar(512);not null;default:''" json:"-"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
}

type CPOIntegration struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID                uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_integration,priority:1;index" json:"cpo_id"`
	Provider             string    `gorm:"type:varchar(50);not null;uniqueIndex:uq_cpo_integration,priority:2" json:"provider"`
	CredentialCiphertext []byte    `gorm:"type:bytea;not null" json:"-"`
	EncryptionKeyID      string    `gorm:"type:varchar(50);not null" json:"-"`
	DisplayHint          string    `gorm:"type:varchar(100);not null" json:"display_hint"`
	IsActive             bool      `gorm:"not null;default:true" json:"is_active"`
	UpdatedByUserID      uuid.UUID `gorm:"type:uuid;not null" json:"updated_by_user_id"`
	CreatedAt            time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time `gorm:"not null" json:"updated_at"`
}
