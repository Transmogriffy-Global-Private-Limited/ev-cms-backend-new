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
