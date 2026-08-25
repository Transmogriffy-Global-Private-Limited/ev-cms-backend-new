package models

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionPlan is a platform-managed catalog entry. Published versions
// are immutable so issued subscriptions retain the terms they were given.
type SubscriptionPlan struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string    `gorm:"type:varchar(80);not null;uniqueIndex" json:"code"`
	Name        string    `gorm:"type:varchar(150);not null" json:"name"`
	Description string    `gorm:"type:varchar(2000);not null;default:''" json:"description"`
	Status      string    `gorm:"type:varchar(20);not null;default:'DRAFT'" json:"status"`
	CreatedBy   uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

type SubscriptionPlanVersion struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PlanID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uq_plan_version,priority:1" json:"plan_id"`
	Version         int        `gorm:"not null;uniqueIndex:uq_plan_version,priority:2" json:"version"`
	Status          string     `gorm:"type:varchar(20);not null;default:'DRAFT'" json:"status"`
	Currency        string     `gorm:"type:char(3);not null" json:"currency"`
	PriceMinor      int64      `gorm:"not null" json:"price_minor"`
	BillingInterval string     `gorm:"type:varchar(20);not null" json:"billing_interval"`
	IntervalCount   int        `gorm:"not null;default:1" json:"interval_count"`
	TrialDays       int        `gorm:"not null;default:0" json:"trial_days"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	PublishedBy     *uuid.UUID `gorm:"type:uuid" json:"published_by,omitempty"`
	CreatedAt       time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null" json:"updated_at"`
}

// CPOSubscription represents a manually issued subscription period. Its dates
// are records; no background worker changes this state automatically.
type CPOSubscription struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID                   uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	PlanVersionID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"plan_version_id"`
	Status                  string     `gorm:"type:varchar(20);not null" json:"status"`
	StartsAt                time.Time  `gorm:"not null" json:"starts_at"`
	TrialEndsAt             *time.Time `json:"trial_ends_at,omitempty"`
	CurrentPeriodStartsAt   time.Time  `gorm:"not null" json:"current_period_starts_at"`
	CurrentPeriodEndsAt     time.Time  `gorm:"not null" json:"current_period_ends_at"`
	CancelAtPeriodEnd       bool       `gorm:"not null;default:false" json:"cancel_at_period_end"`
	PendingPlanVersionID    *uuid.UUID `gorm:"type:uuid" json:"pending_plan_version_id,omitempty"`
	PendingChangeAt         *time.Time `json:"pending_change_at,omitempty"`
	PendingChangeBy         *uuid.UUID `gorm:"type:uuid" json:"-"`
	CancellationScheduledBy *uuid.UUID `gorm:"type:uuid" json:"-"`
	CancelledAt             *time.Time `json:"cancelled_at,omitempty"`
	EndedAt                 *time.Time `json:"ended_at,omitempty"`
	CreatedBy               uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt               time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt               time.Time  `gorm:"not null" json:"updated_at"`
}

type CPOSubscriptionHistory struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubscriptionID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"subscription_id"`
	CPOID                 uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	PreviousStatus        *string    `gorm:"type:varchar(20)" json:"previous_status,omitempty"`
	NextStatus            string     `gorm:"type:varchar(20);not null" json:"next_status"`
	PreviousPlanVersionID *uuid.UUID `gorm:"type:uuid" json:"previous_plan_version_id,omitempty"`
	NextPlanVersionID     uuid.UUID  `gorm:"type:uuid;not null" json:"next_plan_version_id"`
	ActorUserID           uuid.UUID  `gorm:"type:uuid;not null" json:"actor_user_id"`
	Reason                string     `gorm:"type:varchar(500);not null" json:"reason"`
	IdempotencyKey        string     `gorm:"type:varchar(120);not null" json:"idempotency_key"`
	EffectiveAt           time.Time  `gorm:"not null" json:"effective_at"`
	Metadata              JSONB      `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt             time.Time  `gorm:"not null" json:"created_at"`
}

// CPOSubscriptionLifecycleEvent records an exactly-once lifecycle warning or
// expiry transition produced by the CMS scheduler. It is durable evidence,
// not a replacement for the manual subscription history.
type CPOSubscriptionLifecycleEvent struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubscriptionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_subscription_lifecycle_event,priority:1;index" json:"subscription_id"`
	Kind           string    `gorm:"type:varchar(30);not null;uniqueIndex:uq_cpo_subscription_lifecycle_event,priority:2" json:"kind"`
	EffectiveAt    time.Time `gorm:"not null" json:"effective_at"`
	CreatedAt      time.Time `gorm:"not null" json:"created_at"`
}

// TableName preserves the singular table created by the subscription
// migrations. GORM's default pluralization would query the nonexistent
// cpo_subscription_histories table.
func (CPOSubscriptionHistory) TableName() string {
	return "cpo_subscription_history"
}
