package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const lifecycleWorkerName = "subscription-lifecycle"

// RunLifecycle records each warning threshold once and expires elapsed periods.
// It deliberately never changes CPO lifecycle status: CPO administrative
// access remains separate from customer-command commercial admission.
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

// lifecycleScanCursor is tenant-agnostic scanner progress only. Actual
// authorization and subscription state remain on each locked domain row.
type lifecycleScanCursor struct {
	WorkerName     string `gorm:"primaryKey;type:varchar(60)"`
	CursorEndAt    *time.Time
	CursorID       *uuid.UUID `gorm:"type:uuid"`
	LastScanAt     *time.Time
	ProcessedTotal int64
	FailuresTotal  int64
	UpdatedAt      time.Time
}

func (lifecycleScanCursor) TableName() string { return "subscription_lifecycle_scan_progress" }

const lifecycleScanBatchSize = 500

// Each tick advances a durable keyset cursor, including across failures.
// Empty tail wraps to the beginning so skipped/failing rows are retried.
// processLifecycle separates durable discovery/lease from each independently
// committed subscription transition. Every unacknowledged job survives cursor
// advancement, process crashes and later arrivals; retries are fenced.
func (service *Service) processLifecycle(ctx context.Context) error {
	now := service.now()
	jobs, err := service.reserveLifecycleWork(ctx, now)
	if err != nil {
		return err
	}
	var failures []error
	var processed, failed int64
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break // Remaining leases expire and retry on a later tick/restart.
		}
		if err := service.processSubscriptionLifecycle(ctx, job.SubscriptionID, now); err != nil {
			failed++
			failures = append(failures, fmt.Errorf("subscription %s: %w", job.SubscriptionID, err))
			if deferralErr := service.deferLifecycleWork(ctx, job, service.now()); deferralErr != nil {
				failures = append(failures, fmt.Errorf("reschedule subscription %s: %w", job.SubscriptionID, deferralErr))
			}
			continue
		}
		if err := service.completeLifecycleWork(ctx, job); err != nil {
			failed++
			failures = append(failures, fmt.Errorf("acknowledge subscription %s: %w", job.SubscriptionID, err))
			continue // Completed transition is idempotent when lease retries.
		}
		processed++
	}
	if err := service.database.WithContext(ctx).Model(&lifecycleScanCursor{}).
		Where("worker_name = ?", lifecycleWorkerName).
		Updates(map[string]any{"processed_total": gorm.Expr("processed_total + ?", processed),
			"failures_total": gorm.Expr("failures_total + ?", failed), "updated_at": service.now()}).Error; err != nil {
		failures = append(failures, fmt.Errorf("update lifecycle scanner counters: %w", err))
	}
	return errors.Join(failures...)
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
				return service.recordTransition(tx, uuid.Nil, subscription, &previous, nil, "subscription period elapsed", fmt.Sprintf("system:expiry:%s:%d", subscription.ID, subscription.CurrentPeriodSequence), now, "SYSTEM_EXPIRE")
			})
		}
		remaining := subscription.CurrentPeriodEndsAt.Sub(now)
		kind := ""
		switch {
		case remaining <= 24*time.Hour:
			kind = "EXPIRY_WARNING_1D"
		case remaining <= 3*24*time.Hour:
			kind = "EXPIRY_WARNING_3D"
		case remaining <= 7*24*time.Hour:
			kind = "EXPIRY_WARNING_7D"
		}
		if kind != "" {
			return service.recordLifecycleEvent(tx, subscription, kind, now, nil)
		}
		return nil
	})
}

// legacyLifecycleWindow derives only a conservative temporal association.
// A legacy event has no period identity, so the timestamp is evidence of
// whether it belongs to the current period, not proof of historical numbering.
// EXPIRED is recorded on/after the period end; warnings are recorded before it.
func legacyLifecycleWindow(subscription models.CPOSubscription, kind string) (time.Time, *time.Time) {
	if kind == "EXPIRED" {
		return subscription.CurrentPeriodEndsAt, nil
	}
	end := subscription.CurrentPeriodEndsAt
	return subscription.CurrentPeriodStartsAt, &end
}

// legacyLifecycleEventAlreadyRecorded limits deduplication to the original
// period sequence assigned at upgrade. No old lifecycle rows are rewritten.
// A previous-period legacy event remains eligible for a new periodized event.
func (service *Service) legacyLifecycleEventAlreadyRecorded(tx *gorm.DB, subscription models.CPOSubscription, kind string, now time.Time) (bool, error) {
	if subscription.CurrentPeriodSequence != 1 {
		return false, nil
	}
	start, end := legacyLifecycleWindow(subscription, kind)
	query := tx.Model(&models.CPOSubscriptionLifecycleEvent{}).
		Where("subscription_id = ? AND kind = ? AND period_sequence IS NULL", subscription.ID, kind).
		Where("effective_at >= ? AND effective_at <= ?", start, now)
	if end != nil {
		query = query.Where("effective_at < ?", *end)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("read legacy subscription lifecycle event: %w", err)
	}
	return count != 0, nil
}

func (service *Service) recordLifecycleEvent(tx *gorm.DB, subscription models.CPOSubscription, kind string, now time.Time, after func() error) error {
	legacyRecorded, err := service.legacyLifecycleEventAlreadyRecorded(tx, subscription, kind, now)
	if err != nil {
		return err
	}
	if legacyRecorded {
		// Do not deliver a duplicate legacy warning/expired email. If a
		// still-current subscription has an existing legacy EXPIRED event,
		// its state still needs completing, not silently left ACTIVE.
		if after != nil {
			return after()
		}
		return nil
	}
	period := subscription.CurrentPeriodSequence
	event := models.CPOSubscriptionLifecycleEvent{ID: uuid.New(), SubscriptionID: subscription.ID, Kind: kind, PeriodSequence: &period, EffectiveAt: now, CreatedAt: now}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subscription_id"}, {Name: "period_sequence"}, {Name: "kind"}}, DoNothing: true}).Create(&event)
	if result.Error != nil {
		return fmt.Errorf("record subscription lifecycle event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// The delivery is already recorded, but the expiry transition may
		// still need reconciliation if inconsistent state was encountered.
		if after != nil {
			return after()
		}
		return nil
	}
	if err := service.enqueueLifecycleMail(tx, subscription, kind); err != nil {
		return err
	}
	if after != nil {
		return after()
	}
	return nil
}

func (service *Service) enqueueLifecycleMail(tx *gorm.DB, subscription models.CPOSubscription, kind string) error {
	if service.outbox == nil {
		return nil
	}
	template := "CPO_SUBSCRIPTION_EXPIRY_WARNING"
	if kind == "EXPIRED" {
		template = "CPO_SUBSCRIPTION_EXPIRED"
	}
	var cpo models.CPO
	if err := tx.First(&cpo, "id = ?", subscription.CPOID).Error; err != nil {
		return err
	}
	type recipient struct{ Email, FullName string }
	var recipients []recipient
	if err := tx.Table("cpo_memberships").Select("users.email, users.full_name").Joins("JOIN users ON users.id = cpo_memberships.user_id").Where("cpo_memberships.cpo_id = ? AND cpo_memberships.status = ? AND users.is_active = ? AND (cpo_memberships.is_primary_admin = ? OR cpo_memberships.role IN ?)", subscription.CPOID, constants.MembershipStatusActive, true, true, []constants.CPORole{constants.CPORoleAdmin, constants.CPORoleOwner}).Scan(&recipients).Error; err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, recipient := range recipients {
		if seen[recipient.Email] {
			continue
		}
		seen[recipient.Email] = true
		if err := service.outbox.EnqueueMessageWithContext(tx, recipient.Email, template, cmsmail.MessagePayload{RecipientName: recipient.FullName, CPOName: cpo.BusinessName, ExpiresAt: subscription.CurrentPeriodEndsAt}, cmsmail.MessageContext{CPOID: &subscription.CPOID}); err != nil {
			return err
		}
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
