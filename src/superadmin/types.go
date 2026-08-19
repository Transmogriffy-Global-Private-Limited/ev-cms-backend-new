package superadmin

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

type PageQuery struct {
	Limit    int
	Before   *time.Time
	BeforeID *uuid.UUID
}

type AdministratorQuery struct {
	PageQuery
	IncludeInactive bool
}

type InviteAdministratorRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

type ReasonRequest struct {
	Reason string `json:"reason"`
}

type AdministratorView struct {
	UserID            uuid.UUID  `json:"user_id"`
	Email             string     `json:"email"`
	FullName          string     `json:"full_name"`
	IdentityActive    bool       `json:"identity_active"`
	IdentityVerified  bool       `json:"identity_verified"`
	AuthorityActive   bool       `json:"authority_active"`
	StatusReason      string     `json:"status_reason"`
	StatusChangedAt   time.Time  `json:"status_changed_at"`
	StatusChangedByID *uuid.UUID `json:"status_changed_by_user_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type AdministratorPage struct {
	Administrators []AdministratorView `json:"administrators"`
	NextBefore     *time.Time          `json:"next_before,omitempty"`
	NextBeforeID   *uuid.UUID          `json:"next_before_id,omitempty"`
	HasMore        bool                `json:"has_more"`
}

type SecurityQuery struct {
	PageQuery
}

type LockedIdentityView struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	LockedUntil time.Time `json:"locked_until"`
}

type LockedIdentityPage struct {
	Identities   []LockedIdentityView `json:"identities"`
	NextBefore   *time.Time           `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID           `json:"next_before_id,omitempty"`
	HasMore      bool                 `json:"has_more"`
}

type SessionRevocationRequest struct {
	Reason string              `json:"reason"`
	Scope  constants.AuthScope `json:"scope"`
	CPOID  *uuid.UUID          `json:"cpo_id,omitempty"`
}

type SessionRevocationResponse struct {
	RevokedSessions      int64 `json:"revoked_sessions"`
	RevokedRefreshTokens int64 `json:"revoked_refresh_tokens"`
}

type MailQuery struct {
	PageQuery
	Status   constants.MailOutboxStatus
	Template string
	CPOID    *uuid.UUID
	UserID   *uuid.UUID
}

type MailJobView struct {
	ID           uuid.UUID                  `json:"id"`
	ToEmail      string                     `json:"to_email"`
	CPOID        *uuid.UUID                 `json:"cpo_id,omitempty"`
	UserID       *uuid.UUID                 `json:"user_id,omitempty"`
	Template     string                     `json:"template"`
	Status       constants.MailOutboxStatus `json:"status"`
	Attempts     int                        `json:"attempts"`
	MaxAttempts  int                        `json:"max_attempts"`
	AvailableAt  time.Time                  `json:"available_at"`
	LockedAt     *time.Time                 `json:"locked_at,omitempty"`
	ErrorPresent bool                       `json:"error_present"`
	SentAt       *time.Time                 `json:"sent_at,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}

type MailPage struct {
	Jobs         []MailJobView `json:"jobs"`
	NextBefore   *time.Time    `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID    `json:"next_before_id,omitempty"`
	HasMore      bool          `json:"has_more"`
}

type MailMetric struct {
	Template string                     `json:"template"`
	Status   constants.MailOutboxStatus `json:"status"`
	Count    int64                      `json:"count"`
}

type MailMetricsResponse struct {
	Metrics []MailMetric `json:"metrics"`
}

type MailReconcileResponse struct {
	Requeued int64 `json:"requeued"`
}

type MailRetentionRequest struct {
	Before time.Time `json:"before"`
	Reason string    `json:"reason"`
}

type MailRetentionResponse struct {
	Deleted int64 `json:"deleted"`
}

type AnnouncementRequest struct {
	Audience  string     `json:"audience"`
	CPOID     *uuid.UUID `json:"cpo_id,omitempty"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type AnnouncementView struct {
	ID              uuid.UUID  `json:"id"`
	Audience        string     `json:"audience"`
	CPOID           *uuid.UUID `json:"cpo_id,omitempty"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	CreatedByUserID uuid.UUID  `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RecipientCount  int64      `json:"recipient_count"`
}

type AnnouncementPage struct {
	Announcements []AnnouncementView `json:"announcements"`
	NextBefore    *time.Time         `json:"next_before,omitempty"`
	NextBeforeID  *uuid.UUID         `json:"next_before_id,omitempty"`
	HasMore       bool               `json:"has_more"`
}

type NotificationView struct {
	ID             uuid.UUID  `json:"id"`
	AnnouncementID uuid.UUID  `json:"announcement_id"`
	Audience       string     `json:"audience"`
	CPOID          *uuid.UUID `json:"cpo_id,omitempty"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
}

type NotificationPage struct {
	Notifications []NotificationView `json:"notifications"`
	NextBefore    *time.Time         `json:"next_before,omitempty"`
	NextBeforeID  *uuid.UUID         `json:"next_before_id,omitempty"`
	HasMore       bool               `json:"has_more"`
}

type OverviewResponse struct {
	CPOs                 map[string]int64 `json:"cpos"`
	ActivePlatformAdmins int64            `json:"active_platform_admins"`
	ActiveSessions       int64            `json:"active_sessions"`
	Mail                 map[string]int64 `json:"mail"`
	Workers              []WorkerStatus   `json:"workers"`
}

type WorkerStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
}

type StatusResponse struct {
	Service  string         `json:"service"`
	Version  string         `json:"version"`
	Database string         `json:"database"`
	Workers  []WorkerStatus `json:"workers"`
}

type CPOAssetsOverview struct {
	CPOs []CPOWithAssets `json:"cpos"`
}

type CPOWithAssets struct {
	ID           uuid.UUID   `json:"id"`
	BusinessName string      `json:"business_name"`
	Hubs         []HubInfo   `json:"hubs"`
	Chargers     []ChargerInfo `json:"chargers"`
}

type HubInfo struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type ChargerInfo struct {
	ID        uuid.UUID `json:"id"`
	ChargerID string    `json:"charger_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func viewAnnouncement(record models.PlatformAnnouncement, recipientCount int64) AnnouncementView {
	return AnnouncementView{
		ID: record.ID, Audience: record.Audience, CPOID: record.CPOID,
		Title: record.Title, Body: record.Body, CreatedByUserID: record.CreatedByUserID,
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt,
		RecipientCount: recipientCount,
	}
}
