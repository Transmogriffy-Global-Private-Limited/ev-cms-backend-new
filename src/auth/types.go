package auth

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

type LoginRequest struct {
	Email    string              `json:"email"`
	Password string              `json:"password"`
	Scope    constants.AuthScope `json:"scope"`
	CPOID    *uuid.UUID          `json:"cpo_id,omitempty"`
}

type ChallengeRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	Code        string    `json:"code"`
}

type ResendRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	Code        string    `json:"code"`
	NewPassword string    `json:"new_password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type ChallengeResponse struct {
	ChallengeID       uuid.UUID `json:"challenge_id"`
	ExpiresAt         time.Time `json:"expires_at"`
	ResendAvailableAt time.Time `json:"resend_available_at"`
}

type TokenResponse struct {
	AccessToken          string                  `json:"access_token"`
	AccessTokenExpiresAt time.Time               `json:"access_token_expires_at"`
	RefreshToken         string                  `json:"refresh_token"`
	SessionExpiresAt     time.Time               `json:"session_expires_at"`
	TokenType            string                  `json:"token_type"`
	CPOAppID             *string                 `json:"cpo_app_id,omitempty"`
	CPOAppIDMode         *constants.CPOAppIDMode `json:"cpo_app_id_mode,omitempty"`
	MustChangePassword   bool                    `json:"must_change_password"`
}

type SessionView struct {
	ID         uuid.UUID           `json:"id"`
	Scope      constants.AuthScope `json:"scope"`
	CPOID      *uuid.UUID          `json:"cpo_id,omitempty"`
	Role       *constants.CPORole  `json:"role,omitempty"`
	IPAddress  *string             `json:"ip_address,omitempty"`
	UserAgent  string              `json:"user_agent"`
	CreatedAt  time.Time           `json:"created_at"`
	LastSeenAt time.Time           `json:"last_seen_at"`
	ExpiresAt  time.Time           `json:"expires_at"`
	IsCurrent  bool                `json:"is_current"`
}

type UserView struct {
	ID                 uuid.UUID  `json:"id"`
	Email              string     `json:"email"`
	FullName           string     `json:"full_name"`
	IsVerified         bool       `json:"is_verified"`
	MFAEnabled         bool       `json:"mfa_enabled"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}

type MeResponse struct {
	User         UserView                `json:"user"`
	Scope        constants.AuthScope     `json:"scope"`
	CPOID        *uuid.UUID              `json:"cpo_id,omitempty"`
	Role         *constants.CPORole      `json:"role,omitempty"`
	CPOAppID     *string                 `json:"cpo_app_id,omitempty"`
	CPOAppIDMode *constants.CPOAppIDMode `json:"cpo_app_id_mode,omitempty"`
}

type Principal struct {
	UserID       uuid.UUID
	SessionID    uuid.UUID
	Scope        constants.AuthScope
	CPOID        *uuid.UUID
	Role         *constants.CPORole
	CPOAppID     *string
	CPOAppIDMode *constants.CPOAppIDMode
	TokenVersion int
	User         UserView
}

type RequestMetadata struct {
	IPAddress *string
	UserAgent string
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (err *APIError) Error() string {
	return err.Code
}
