package customerauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var customerCommandCurrentSubscriptionStatuses = []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}

// requireCustomerCommercialAdmission blocks only new customer-paid commands.
// It intentionally does not gate session stop, HAL fact ingestion, settlement,
// start-intent replay, or verification of a recharge already created before a
// subscription expired: those operations preserve already-established truth.
func (service *Service) requireCustomerCommercialAdmission(ctx context.Context, cpoID uuid.UUID, now time.Time) error {
	return requireCustomerCommercialAdmission(service.database.WithContext(ctx), cpoID, now)
}

func requireCustomerCommercialAdmission(query *gorm.DB, cpoID uuid.UUID, now time.Time) error {
	var subscriptions []models.CPOSubscription
	if err := query.
		Where("cpo_id = ? AND (status = ? OR (status IN ? AND current_period_ends_at <= ?))", cpoID, "EXPIRED", customerCommandCurrentSubscriptionStatuses, now).
		Order("current_period_ends_at DESC, updated_at DESC").
		Find(&subscriptions).Error; err != nil {
		return fmt.Errorf("load CPO subscription admission state: %w", err)
	}
	for _, subscription := range subscriptions {
		if subscriptionBlocksCustomerCommands(subscription, now) {
			return &APIError{http.StatusForbidden, "cpo_subscription_expired", "New charging and wallet recharge requests are unavailable until this charging provider renews its subscription."}
		}
	}
	return nil
}

func subscriptionBlocksCustomerCommands(subscription models.CPOSubscription, now time.Time) bool {
	if subscription.Status == "EXPIRED" {
		return true
	}
	for _, status := range customerCommandCurrentSubscriptionStatuses {
		if subscription.Status == status {
			return !subscription.CurrentPeriodEndsAt.After(now)
		}
	}
	return false
}
