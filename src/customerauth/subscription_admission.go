package customerauth

import (
	"context"
	"errors"
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
	// A CPO can have terminal historical rows beside its one current row. An old
	// EXPIRED row must not override a later ACTIVE renewal, so current state has
	// precedence over terminal history for commercial admission.
	var current models.CPOSubscription
	result := query.
		Where("cpo_id = ? AND status IN ?", cpoID, customerCommandCurrentSubscriptionStatuses).
		Order("current_period_ends_at DESC, updated_at DESC, id DESC").
		First(&current)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load current CPO subscription admission state: %w", result.Error)
	}
	if result.Error == nil {
		if !customerSubscriptionAdmissionBlocked(&current, nil, now) {
			return nil
		}
		return customerSubscriptionExpiredError()
	}

	var expired models.CPOSubscription
	result = query.
		Where("cpo_id = ? AND status = ?", cpoID, "EXPIRED").
		Order("updated_at DESC, id DESC").
		First(&expired)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load expired CPO subscription admission state: %w", result.Error)
	}
	if customerSubscriptionAdmissionBlocked(nil, subscriptionPointer(result, expired), now) {
		return customerSubscriptionExpiredError()
	}
	return nil
}

func customerSubscriptionAdmissionBlocked(current, expired *models.CPOSubscription, now time.Time) bool {
	if current != nil {
		return subscriptionBlocksCustomerCommands(*current, now)
	}
	return expired != nil && subscriptionBlocksCustomerCommands(*expired, now)
}

func customerSubscriptionExpiredError() error {
	return &APIError{http.StatusForbidden, "cpo_subscription_expired", "New charging and wallet recharge requests are unavailable until this charging provider renews its subscription."}
}

func subscriptionPointer(result *gorm.DB, subscription models.CPOSubscription) *models.CPOSubscription {
	if result.Error != nil {
		return nil
	}
	return &subscription
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
