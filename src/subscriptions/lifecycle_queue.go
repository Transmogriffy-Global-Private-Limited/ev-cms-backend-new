package subscriptions

import (
	"context"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A scan cursor is only a discovery watermark. Durable, separately leased
// pending work prevents a crash or a failing subscription from disappearing
// behind that watermark while new eligible subscriptions keep arriving.
type lifecyclePendingWork struct {
	SubscriptionID uuid.UUID  `gorm:"type:uuid;primaryKey"`
	NextAttemptAt  time.Time  `gorm:"not null"`
	ClaimToken     *uuid.UUID `gorm:"type:uuid"`
	Attempts       int64      `gorm:"not null"`
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
}

func (lifecyclePendingWork) TableName() string { return "subscription_lifecycle_pending_work" }

const lifecycleWorkLease = 2 * time.Minute
const lifecycleRetryCap = 15 * time.Minute

// Failed work backs off, but a process crash needs only the bounded lease to
// expire. The queue preserves the job while the scanner advances independently.
func lifecycleRetryDelay(attempts int64) time.Duration {
	delay := 30 * time.Second
	for attempt := int64(1); attempt < attempts && delay < lifecycleRetryCap; attempt++ {
		delay *= 2
		if delay >= lifecycleRetryCap {
			return lifecycleRetryCap
		}
	}
	return delay
}

// reserveLifecycleWork atomically discovers, persists, and leases work.
// It performs NO mail delivery or subscription transitions while holding the
// scanner cursor lock. Each subscription's transition runs in its own tx.
func (service *Service) reserveLifecycleWork(ctx context.Context, now time.Time) ([]lifecyclePendingWork, error) {
	cutoff := now.Add(7 * 24 * time.Hour)
	var reserved []lifecyclePendingWork
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cursor lifecycleScanCursor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cursor, "worker_name = ?", lifecycleWorkerName).Error; err != nil {
			return fmt.Errorf("lock lifecycle scan cursor: %w", err)
		}
		type candidate struct {
			ID                  uuid.UUID
			CurrentPeriodEndsAt time.Time
		}
		load := func(after bool) ([]candidate, error) {
			var rows []candidate
			query := tx.Model(&models.CPOSubscription{}).
				Select("id, current_period_ends_at").
				Where("status IN ? AND current_period_ends_at <= ?", currentStatuses, cutoff)
			if after && cursor.CursorEndAt != nil && cursor.CursorID != nil {
				query = query.Where("(current_period_ends_at, id) > (?, ?)", *cursor.CursorEndAt, *cursor.CursorID)
			}
			err := query.Order("current_period_ends_at ASC, id ASC").Limit(lifecycleScanBatchSize).Find(&rows).Error
			return rows, err
		}
		rows, err := load(true)
		if err != nil {
			return fmt.Errorf("discover lifecycle work: %w", err)
		}
		if len(rows) == 0 && cursor.CursorEndAt != nil {
			cursor.CursorEndAt, cursor.CursorID = nil, nil
			rows, err = load(false)
			if err != nil {
				return fmt.Errorf("wrap lifecycle scan: %w", err)
			}
		}
		// Insert-before-cursor-advance is the crash-safety invariant. Existing
		// pending rows retain their retry/lease deadline on rediscovery.
		if len(rows) != 0 {
			pending := make([]lifecyclePendingWork, 0, len(rows))
			for _, row := range rows {
				pending = append(pending, lifecyclePendingWork{
					SubscriptionID: row.ID, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
				})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&pending, 100).Error; err != nil {
				return fmt.Errorf("persist lifecycle pending work: %w", err)
			}
			last := rows[len(rows)-1]
			cursor.CursorEndAt, cursor.CursorID = &last.CurrentPeriodEndsAt, &last.ID
		}
		cursor.LastScanAt, cursor.UpdatedAt = &now, now
		if err := tx.Save(&cursor).Error; err != nil {
			return fmt.Errorf("save lifecycle cursor: %w", err)
		}
		// Unlike the discovery cursor, this queue selects overdue retries
		// independently, ordered by deadline even under continuous arrivals.
		var ready []lifecyclePendingWork
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_attempt_at <= ?", now).
			Order("next_attempt_at ASC, subscription_id ASC").
			Limit(lifecycleScanBatchSize).Find(&ready).Error; err != nil {
			return fmt.Errorf("reserve due lifecycle work: %w", err)
		}
		reserved = make([]lifecyclePendingWork, 0, len(ready))
		for _, job := range ready {
			token := uuid.New()
			result := tx.Model(&lifecyclePendingWork{}).Where("subscription_id = ?", job.SubscriptionID).
				Updates(map[string]any{
					"claim_token":     token,
					"next_attempt_at": now.Add(lifecycleWorkLease),
					"attempts":        gorm.Expr("attempts + 1"),
					"updated_at":      now,
				})
			if result.Error != nil {
				return fmt.Errorf("lease lifecycle work: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("lifecycle work disappeared while reserved")
			}
			job.ClaimToken, job.NextAttemptAt, job.Attempts = &token, now.Add(lifecycleWorkLease), job.Attempts+1
			reserved = append(reserved, job)
		}
		return nil
	})
	return reserved, err
}

// A stale owner must never acknowledge or postpone work claimed by a newer
// worker after its lease expired. Claim tokens fence both operations.
func (service *Service) completeLifecycleWork(ctx context.Context, job lifecyclePendingWork) error {
	if job.ClaimToken == nil {
		return fmt.Errorf("lifecycle work missing claim token")
	}
	return service.database.WithContext(ctx).Where("subscription_id = ? AND claim_token = ?", job.SubscriptionID, *job.ClaimToken).
		Delete(&lifecyclePendingWork{}).Error
}

func (service *Service) deferLifecycleWork(ctx context.Context, job lifecyclePendingWork, now time.Time) error {
	if job.ClaimToken == nil {
		return fmt.Errorf("lifecycle work missing claim token")
	}
	return service.database.WithContext(ctx).Model(&lifecyclePendingWork{}).
		Where("subscription_id = ? AND claim_token = ?", job.SubscriptionID, *job.ClaimToken).
		Updates(map[string]any{
			"next_attempt_at": now.Add(lifecycleRetryDelay(job.Attempts)),
			"claim_token":     nil,
			"updated_at":      now,
		}).Error
}
