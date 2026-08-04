package subscriptions

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

type PlanTermsInput struct {
	Currency        string `json:"currency"`
	PriceMinor      int64  `json:"price_minor"`
	BillingInterval string `json:"billing_interval"`
	IntervalCount   int    `json:"interval_count"`
	TrialDays       int    `json:"trial_days"`
}

type CreatePlanRequest struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Terms       PlanTermsInput `json:"terms"`
}

type UpdateDraftRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Terms       PlanTermsInput `json:"terms"`
}

type PlanView struct {
	Plan      models.SubscriptionPlan          `json:"plan"`
	Draft     *models.SubscriptionPlanVersion  `json:"draft,omitempty"`
	Published []models.SubscriptionPlanVersion `json:"published_versions"`
}

type IssueRequest struct {
	PlanVersionID  uuid.UUID  `json:"plan_version_id"`
	StartsAt       *time.Time `json:"starts_at"`
	Reason         string     `json:"reason"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type RenewRequest struct {
	StartsAt       *time.Time `json:"starts_at"`
	Reason         string     `json:"reason"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type ChangePlanRequest struct {
	PlanVersionID  uuid.UUID `json:"plan_version_id"`
	Reason         string    `json:"reason"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type TransitionRequest struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type SubscriptionView struct {
	Subscription models.CPOSubscription         `json:"subscription"`
	Plan         models.SubscriptionPlan        `json:"plan"`
	Version      models.SubscriptionPlanVersion `json:"plan_version"`
}
