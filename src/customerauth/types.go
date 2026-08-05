package customerauth

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SignupRequest struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	FullName string  `json:"full_name"`
	Phone    *string `json:"phone,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ChallengeRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
	Code        string    `json:"code"`
}

type ResendRequest struct {
	ChallengeID uuid.UUID `json:"challenge_id"`
}

type ChallengeResponse struct {
	ChallengeID       uuid.UUID `json:"challenge_id"`
	ExpiresAt         time.Time `json:"expires_at"`
	ResendAvailableAt time.Time `json:"resend_available_at"`
}

type SignupResponse struct {
	CustomerID uuid.UUID `json:"customer_id"`
	CPOID      uuid.UUID `json:"cpo_id"`
	WalletID   uuid.UUID `json:"wallet_id"`
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

// UpdateProfileRequest deliberately tracks whether phone was present so the
// API can distinguish an omitted field (preserve it) from explicit null
// (clear it).
type UpdateProfileRequest struct {
	FullName string  `json:"full_name"`
	Phone    *string `json:"phone,omitempty"`
	phoneSet bool
}

func (request *UpdateProfileRequest) UnmarshalJSON(data []byte) error {
	type wireRequest struct {
		FullName string  `json:"full_name"`
		Phone    *string `json:"phone"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireRequest
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, request.phoneSet = fields["phone"]
	request.FullName = wire.FullName
	request.Phone = wire.Phone
	return nil
}

type TokenResponse struct {
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
	RefreshToken         string    `json:"refresh_token"`
	SessionExpiresAt     time.Time `json:"session_expires_at"`
	TokenType            string    `json:"token_type"`
	CustomerID           uuid.UUID `json:"customer_id"`
	CPOID                uuid.UUID `json:"cpo_id"`
	CPOAppID             string    `json:"cpo_app_id"`
}

type UserView struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	FullName    string     `json:"full_name"`
	Phone       *string    `json:"phone,omitempty"`
	IsVerified  bool       `json:"is_verified"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type CustomerView struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status"`
	UserGroupID *uuid.UUID `json:"user_group_id,omitempty"`
}

type CPOView struct {
	ID           uuid.UUID `json:"id"`
	BusinessName string    `json:"business_name"`
	AppID        string    `json:"app_id"`
	AppIDMode    string    `json:"app_id_mode"`
}

type WalletView struct {
	ID       uuid.UUID `json:"id"`
	Balance  string    `json:"balance"`
	Currency string    `json:"currency"`
}

type MeResponse struct {
	User     UserView     `json:"user"`
	Customer CustomerView `json:"customer"`
	CPO      CPOView      `json:"cpo"`
	Wallet   WalletView   `json:"wallet"`
}

type SessionView struct {
	ID         uuid.UUID `json:"id"`
	IPAddress  *string   `json:"ip_address,omitempty"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsCurrent  bool      `json:"is_current"`
}

type Principal struct {
	// UserID is a source-compatibility alias of CustomerID. It never refers to
	// the global administrative users table.
	UserID       uuid.UUID
	CustomerID   uuid.UUID
	CPOID        uuid.UUID
	SessionID    uuid.UUID
	CPOAppID     string
	TokenVersion int
	User         UserView
	Customer     CustomerView
	CPO          CPOView
	Wallet       WalletView
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
