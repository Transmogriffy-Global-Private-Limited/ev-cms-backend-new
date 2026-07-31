package cpo

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

type CreateRequest struct {
	Slug         string                   `json:"slug"`
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        *string                  `json:"gstin,omitempty"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        string                   `json:"state"`
	Pincode      string                   `json:"pincode"`
	Admin        InitialAdminRequest      `json:"admin"`
}

type InitialAdminRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

type SetAppIDRequest struct {
	AppID string `json:"app_id"`
}

type UpdateProfileRequest struct {
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        *string                  `json:"gstin"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        string                   `json:"state"`
	Pincode      string                   `json:"pincode"`
}

type LifecycleRequest struct {
	Reason string `json:"reason"`
}

type PrimaryAdminRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Reason   string `json:"reason"`
}

type ReasonRequest struct {
	Reason string `json:"reason"`
}

type ListQuery struct {
	Search   string
	Status   *constants.CPOStatus
	AppMode  *constants.CPOAppIDMode
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

type View struct {
	ID                    uuid.UUID                `json:"id"`
	Slug                  string                   `json:"slug"`
	BusinessName          string                   `json:"business_name"`
	CompanyType           constants.CPOCompanyType `json:"company_type"`
	GSTIN                 *string                  `json:"gstin,omitempty"`
	Address               string                   `json:"address"`
	City                  string                   `json:"city"`
	State                 string                   `json:"state"`
	Pincode               string                   `json:"pincode"`
	Status                constants.CPOStatus      `json:"status"`
	StatusReason          string                   `json:"status_reason"`
	StatusChangedAt       time.Time                `json:"status_changed_at"`
	StatusChangedByUserID *uuid.UUID               `json:"status_changed_by_user_id,omitempty"`
	AppID                 string                   `json:"app_id"`
	AppIDMode             constants.CPOAppIDMode   `json:"app_id_mode"`
	AppIDUpdatedAt        time.Time                `json:"app_id_updated_at"`
	CreatedAt             time.Time                `json:"created_at"`
	UpdatedAt             time.Time                `json:"updated_at"`
}

type InitialAdminView struct {
	UserID          uuid.UUID         `json:"user_id"`
	Email           string            `json:"email"`
	FullName        string            `json:"full_name"`
	Role            constants.CPORole `json:"role"`
	IdentityCreated bool              `json:"identity_created"`
}

type PrimaryAdminView struct {
	UserID                   uuid.UUID                  `json:"user_id"`
	Email                    string                     `json:"email"`
	FullName                 string                     `json:"full_name"`
	Role                     constants.CPORole          `json:"role"`
	MembershipStatus         constants.MembershipStatus `json:"membership_status"`
	IdentityActive           bool                       `json:"identity_active"`
	IdentityVerified         bool                       `json:"identity_verified"`
	MustChangePassword       bool                       `json:"must_change_password"`
	LastLoginAt              *time.Time                 `json:"last_login_at,omitempty"`
	LatestOnboardingDelivery *OnboardingDeliveryView    `json:"latest_onboarding_delivery,omitempty"`
}

type OnboardingDeliveryView struct {
	JobID     uuid.UUID                  `json:"job_id"`
	Template  string                     `json:"template"`
	Status    constants.MailOutboxStatus `json:"status"`
	Attempts  int                        `json:"attempts"`
	SentAt    *time.Time                 `json:"sent_at,omitempty"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type CreateResponse struct {
	CPO   View             `json:"cpo"`
	Admin InitialAdminView `json:"admin"`
}

type ListResponse struct {
	CPOs         []View     `json:"cpos"`
	NextBefore   *time.Time `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID `json:"next_before_id,omitempty"`
	HasMore      bool       `json:"has_more"`
}

type SessionRevocationResponse struct {
	RevokedSessions      int64 `json:"revoked_sessions"`
	RevokedRefreshTokens int64 `json:"revoked_refresh_tokens"`
}
