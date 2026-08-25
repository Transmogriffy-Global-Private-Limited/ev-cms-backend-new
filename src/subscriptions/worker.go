package subscriptions

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const lifecycleWorkerName = "subscription-lifecycle"

// RunLifecycle records each warning threshold once and expires elapsed periods.
// It deliberately never changes CPO lifecycle status: subscription commercial
// truth and tenant access remain separate policy decisions.
func (service *Service) RunLifecycle(ctx context.Context, interval time.Duration, observer WorkerObserver, instanceKey string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if observer != nil {
			_ = observer.Heartbeat(ctx, lifecycleWorkerName, instanceKey)
		}
		if err := service.processLifecycle(ctx); err != nil && ctx.Err() == nil {
			log.Printf("subscription lifecycle failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type WorkerObserver interface {
	Heartbeat(context.Context, string, string) error
}

func (service *Service) processLifecycle(ctx context.Context) error {
	now := service.now()
	var ids []uuid.UUID
	if err := service.database.WithContext(ctx).Model(&models.CPOSubscription{}).
		Where("status IN ? AND current_period_ends_at <= ?", currentStatuses, now.Add(7*24*time.Hour)).
		Order("current_period_ends_at ASC").Limit(500).Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("list subscription lifecycle candidates: %w", err)
	}
	for _, id := range ids {
		if err := service.processSubscriptionLifecycle(ctx, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) processSubscriptionLifecycle(ctx context.Context, subscriptionID uuid.UUID, now time.Time) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var subscription models.CPOSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&subscription, "id = ?", subscriptionID).Error; err != nil {
			return err
		}
		if !containsStatus(subscription.Status) {
			return nil
		}
		if !now.Before(subscription.CurrentPeriodEndsAt) {
			return service.recordLifecycleEvent(tx, subscription, "EXPIRED", now, func() error {
				previous := subscription.Status
				subscription.Status, subscription.EndedAt, subscription.UpdatedAt = "EXPIRED", &now, now
				if err := tx.Model(&models.CPOSubscription{}).Where("id = ?", subscription.ID).Updates(map[string]any{"status": subscription.Status, "ended_at": now, "updated_at": now}).Error; err != nil {
					return err
				}
				return service.recordTransition(tx, uuid.Nil, subscription, &previous, nil, "subscription period elapsed", "system:expiry:"+subscription.ID.String(), now, "SYSTEM_EXPIRE")
			})
		}
		for _, warning := range []struct {
			kind   string
			before time.Duration
		}{{"EXPIRY_WARNING_1D", 24 * time.Hour}, {"EXPIRY_WARNING_3D", 3 * 24 * time.Hour}, {"EXPIRY_WARNING_7D", 7 * 24 * time.Hour}} {
			if !now.Before(subscription.CurrentPeriodEndsAt.Add(-warning.before)) {
				if err := service.recordLifecycleEvent(tx, subscription, warning.kind, now, nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (service *Service) recordLifecycleEvent(tx *gorm.DB, subscription models.CPOSubscription, kind string, now time.Time, after func() error) error {
	event := models.CPOSubscriptionLifecycleEvent{ID: uuid.New(), SubscriptionID: subscription.ID, Kind: kind, EffectiveAt: now, CreatedAt: now}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subscription_id"}, {Name: "kind"}}, DoNothing: true}).Create(&event)
	if result.Error != nil {
		return fmt.Errorf("record subscription lifecycle event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil
	}
	if after != nil {
		return after()
	}
	return nil
}

func containsStatus(status string) bool {
	for _, candidate := range currentStatuses {
		if status == candidate {
			return true
		}
	}
	return false
}
