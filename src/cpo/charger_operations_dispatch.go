package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/workerobs"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	chargerOperationLease       = 30 * time.Second
	chargerOperationCallTimeout = 15 * time.Second
	chargerOperationRetryDelay  = time.Minute
	chargerOperationPoll        = 5 * time.Second
	chargerOperationBatch       = 25
)

func operationNeedsReconciliation(state string) bool {
	return state == "DELIVERY_ATTEMPTED" || state == "RECONCILIATION_REQUIRED" || state == "HAL_ACCEPTED"
}

// claimChargerOperation exclusively leases a definitely-unattempted operation.
// Token fencing, not process-local locking, prevents an expired owner sending.
func (service *Service) claimChargerOperation(ctx context.Context, id uuid.UUID) (models.ChargerOperation, bool, error) {
	var operation models.ChargerOperation
	claimed := false
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := chargerOperationDatabaseTime(tx)
		if err != nil {
			return err
		}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("id = ? AND delivery_attempted_at IS NULL AND recovery_after <= ? AND (state = 'PERSISTED' OR (state = 'DISPATCH_CLAIMED' AND dispatch_claim_expires_at <= ?))", id, now, now).First(&operation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		token, expiry := uuid.New(), now.Add(chargerOperationLease)
		operation.State, operation.DispatchClaimToken, operation.DispatchClaimExpiresAt = "DISPATCH_CLAIMED", &token, &expiry
		operation.RecoveryAfter, operation.UpdatedAt = expiry, now
		if err := tx.Model(&operation).Updates(map[string]any{"state": operation.State, "dispatch_claim_token": token, "dispatch_claim_expires_at": expiry, "recovery_after": expiry, "updated_at": now}).Error; err != nil {
			return err
		}
		claimed = true
		return service.emitChargerOperationEvent(tx, operation)
	})
	return operation, claimed && err == nil, err
}

// markChargerOperationAttempt must COMMIT successfully before the caller may
// touch HAL. A failed or ambiguous commit never permits external dispatch.
func (service *Service) markChargerOperationAttempt(ctx context.Context, operation models.ChargerOperation) (bool, error) {
	marked := false
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := chargerOperationDatabaseTime(tx)
		if err != nil {
			return err
		}
		result := tx.Model(&models.ChargerOperation{}).
			Where("id = ? AND state = 'DISPATCH_CLAIMED' AND dispatch_claim_token = ? AND dispatch_claim_expires_at > ? AND delivery_attempted_at IS NULL", operation.ID, operation.DispatchClaimToken, now).
			Updates(map[string]any{"state": "DELIVERY_ATTEMPTED", "delivery_attempted_at": now, "dispatch_claim_token": nil, "dispatch_claim_expires_at": nil, "recovery_after": now.Add(chargerOperationRetryDelay), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		operation.State, operation.UpdatedAt, operation.DeliveryAttemptedAt = "DELIVERY_ATTEMPTED", now, &now
		marked = true
		return service.emitChargerOperationEvent(tx, operation)
	})
	return marked && err == nil, err
}

func frozenChargerOperationRequest(operation models.ChargerOperation) (halops.ChargerOperationRequest, error) {
	if operation.DispatchChargerIdentity == nil || *operation.DispatchChargerIdentity == "" || operation.DispatchConnectorNumber == nil || *operation.DispatchConnectorNumber < 0 {
		return halops.ChargerOperationRequest{}, errors.New("invalid dispatch snapshot")
	}
	correlation, err := uuid.Parse(operation.CorrelationID)
	if err != nil || correlation == uuid.Nil || correlation.String() != operation.CorrelationID || operation.ID == uuid.Nil || operation.TraceID == uuid.Nil || operation.CPOID == uuid.Nil || operation.ChargerID == uuid.Nil {
		return halops.ChargerOperationRequest{}, errors.New("invalid dispatch identity")
	}
	for key, value := range operation.Parameters {
		if key != "configuration_keys" {
			if _, ok := value.(string); !ok {
				return halops.ChargerOperationRequest{}, errors.New("invalid dispatch parameter")
			}
		}
	}
	var keys []string
	if value, exists := operation.Parameters["configuration_keys"]; exists {
		raw, err := json.Marshal(value)
		if err != nil {
			return halops.ChargerOperationRequest{}, err
		}
		if err := json.Unmarshal(raw, &keys); err != nil {
			return halops.ChargerOperationRequest{}, err
		}
	}
	return halops.ChargerOperationRequest{CMSOperationID: operation.ID, TraceID: operation.TraceID, CPOID: operation.CPOID, CMSChargerID: operation.ChargerID, CMSConnectorID: operation.ConnectorID, ChargerOCPPIdentity: *operation.DispatchChargerIdentity, OCPPConnectorNumber: *operation.DispatchConnectorNumber, Kind: operation.Kind, Parameters: operationParameters(operation.Parameters), ConfigurationKeys: keys}, nil
}

// The handler and worker both use this dispatcher; neither has another send path.
func (service *Service) dispatchChargerOperation(ctx context.Context, id uuid.UUID) (ChargerOperationResponse, error) {
	if service.halOperations == nil || !service.halOperations.Available() {
		return service.readChargerOperation(ctx, id)
	}
	operation, claimed, err := service.claimChargerOperation(ctx, id)
	if err != nil {
		return ChargerOperationResponse{}, fmt.Errorf("claim charger operation: %w", err)
	}
	if !claimed {
		return service.readChargerOperation(ctx, id)
	}
	request, err := frozenChargerOperationRequest(operation)
	if err != nil {
		// No external attempt: release with backoff so a poison row cannot hot-loop.
		updateErr := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&models.ChargerOperation{}).Where("id = ? AND state = 'DISPATCH_CLAIMED' AND dispatch_claim_token = ?", id, operation.DispatchClaimToken).
				Updates(map[string]any{"state": "PERSISTED", "dispatch_claim_token": nil, "dispatch_claim_expires_at": nil, "recovery_after": gorm.Expr("clock_timestamp() + interval '1 minute'"), "failure_category": "invalid_dispatch_snapshot", "updated_at": service.now()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			if err := tx.First(&operation, "id = ?", id).Error; err != nil {
				return err
			}
			return service.emitChargerOperationEvent(tx, operation)
		})
		return ChargerOperationResponse{}, errors.Join(err, updateErr)
	}
	// Start the deadline before the marker: a paused owner must not wake up
	// after the reconciliation grace period and begin a fresh network budget.
	callCtx, cancel := context.WithTimeout(ctx, chargerOperationCallTimeout)
	defer cancel()
	marked, err := service.markChargerOperationAttempt(callCtx, operation)
	if err != nil {
		return ChargerOperationResponse{}, fmt.Errorf("mark charger operation attempt: %w", err)
	}
	if !marked {
		return service.readChargerOperation(ctx, id)
	}
	result, callErr := service.halOperations.RequestChargerOperation(callCtx, request, operation.CorrelationID)
	cancel()
	if err := service.recordChargerOperationResult(ctx, operation, result, callErr, nil); err != nil {
		return ChargerOperationResponse{}, err
	}
	response, err := service.readChargerOperation(ctx, id)
	if callErr == nil && response.State == "OCPP_CONFIRMED" {
		response.Configuration = result.Configuration
	}
	return response, err
}

func (service *Service) readChargerOperation(ctx context.Context, id uuid.UUID) (ChargerOperationResponse, error) {
	var operation models.ChargerOperation
	if err := service.database.WithContext(ctx).First(&operation, "id = ?", id).Error; err != nil {
		return ChargerOperationResponse{}, err
	}
	return chargerOperationView(operation), nil
}

// Only HAL-owned outcomes may be copied back; HAL must never put a CMS row in
// a pre-delivery state. Identity mismatches remain uncertain, never dispatchable.
func validChargerOperationResult(operation models.ChargerOperation, result halops.ChargerOperation) bool {
	return result.CMSOperationID == operation.ID && result.HALOperationID != uuid.Nil && result.Kind == operation.Kind &&
		(result.State == "HAL_ACCEPTED" || result.State == "OCPP_CONFIRMED" || result.State == "RECONCILIATION_REQUIRED")
}

func (service *Service) recordChargerOperationResult(ctx context.Context, operation models.ChargerOperation, result halops.ChargerOperation, callErr error, recoveryToken *uuid.UUID) error {
	// HAL's queue/attempt states describe HAL's boundary, not CMS's. Once
	// known there, this CMS operation is never a pre-delivery candidate again.
	if result.State == "PERSISTED" || result.State == "DELIVERY_ATTEMPTED" {
		result.State = "HAL_ACCEPTED"
	}
	if callErr == nil && !validChargerOperationResult(operation, result) {
		callErr = errors.New("invalid HAL operation result")
	}
	now, err := chargerOperationDatabaseTime(service.database.WithContext(ctx))
	if err != nil {
		return err
	}
	updates := map[string]any{"updated_at": now, "recovery_after": now.Add(chargerOperationRetryDelay), "recovery_token": nil, "completed_at": nil}
	if callErr != nil {
		if recoveryToken != nil && errors.Is(callErr, halops.ErrChargerOperationNotFound) {
			updates["state"], updates["failure_category"], updates["completed_at"] = "CONFIRMED_ABSENT", "confirmed_absent", now
		} else {
			updates["state"], updates["failure_category"] = "RECONCILIATION_REQUIRED", chargerOperationFailure(callErr)
		}
	} else {
		updates["hal_operation_id"], updates["state"], updates["ocpp_result"], updates["failure_category"] = result.HALOperationID, result.State, result.OCPPResult, result.ErrorCategory
		if result.State == "OCPP_CONFIRMED" {
			updates["completed_at"] = result.CompletedAt
		}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&models.ChargerOperation{}).Where("id = ?", operation.ID)
		if recoveryToken == nil {
			query = query.Where("state = 'DELIVERY_ATTEMPTED'")
		} else {
			query = query.Where("recovery_token = ? AND state IN ?", *recoveryToken, []string{"DELIVERY_ATTEMPTED", "RECONCILIATION_REQUIRED", "HAL_ACCEPTED"})
		}
		updated := query.Updates(updates)
		if updated.Error != nil {
			return fmt.Errorf("record charger operation result: %w", updated.Error)
		}
		if updated.RowsAffected == 0 {
			return nil
		} // A newer reconciler/result won.
		if err := tx.First(&operation, "id = ?", operation.ID).Error; err != nil {
			return err
		}
		return service.emitChargerOperationEvent(tx, operation)
	})
}

// A short durable reservation bounds concurrent reads and fences stale results.
// This method ONLY queries HAL, even when HAL reports confirmed absence.
func (service *Service) reconcileChargerOperation(ctx context.Context, id uuid.UUID) error {
	if service.halOperations == nil || !service.halOperations.Available() {
		return nil
	}
	var operation models.ChargerOperation
	token := uuid.New()
	reserved := false
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := chargerOperationDatabaseTime(tx)
		if err != nil {
			return err
		}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("id = ? AND state IN ? AND recovery_after <= ?", id, []string{"DELIVERY_ATTEMPTED", "RECONCILIATION_REQUIRED", "HAL_ACCEPTED"}, now).First(&operation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&operation).UpdateColumns(map[string]any{"recovery_token": token, "recovery_after": now.Add(chargerOperationRetryDelay)}).Error; err != nil {
			return err
		}
		reserved = true
		return nil
	})
	if err != nil || !reserved {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, chargerOperationCallTimeout)
	result, callErr := service.halOperations.ReconcileChargerOperation(callCtx, id)
	cancel()
	return service.recordChargerOperationResult(ctx, operation, result, callErr, &token)
}

// Each iteration selects a bounded, oldest-due batch; each failure is isolated.
// Claim/reservation writes move failed work behind other due rows. No network
// call holds a DB transaction, and concurrency is deliberately one per process.
func (service *Service) recoverChargerOperations(ctx context.Context) error {
	if service.halOperations == nil || !service.halOperations.Available() {
		return nil
	}
	var operations []models.ChargerOperation
	if err := service.database.WithContext(ctx).Where("state IN ? AND recovery_after <= clock_timestamp()", []string{"PERSISTED", "DISPATCH_CLAIMED", "DELIVERY_ATTEMPTED", "RECONCILIATION_REQUIRED", "HAL_ACCEPTED"}).Order("recovery_after,id").Limit(chargerOperationBatch).Find(&operations).Error; err != nil {
		return err
	}
	var failures []error
	for _, operation := range operations {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		if operationNeedsReconciliation(operation.State) {
			err = service.reconcileChargerOperation(ctx, operation.ID)
		} else {
			_, err = service.dispatchChargerOperation(ctx, operation.ID)
		}
		if err != nil {
			// If even the claim/event transaction failed, move an unclaimed
			// poison row behind other due work without stealing a live lease.
			deferErr := service.database.WithContext(ctx).Model(&models.ChargerOperation{}).
				Where("id = ? AND delivery_attempted_at IS NULL AND (state = 'PERSISTED' OR (state = 'DISPATCH_CLAIMED' AND dispatch_claim_expires_at <= clock_timestamp()))", operation.ID).
				UpdateColumn("recovery_after", gorm.Expr("clock_timestamp() + interval '1 minute'")).Error
			err = errors.Join(err, deferErr)
			log.Printf("charger operation recovery failed operation_id=%s category=recovery_failed", operation.ID)
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (service *Service) RunChargerOperationRecovery(ctx context.Context, observer workerobs.Observer, instanceKey string) {
	ticker := time.NewTicker(chargerOperationPoll)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if observer != nil {
			_ = observer.Heartbeat(ctx, "charger-operation-recovery", instanceKey)
		}
		cycleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := service.recoverChargerOperations(cycleCtx)
		cancel()
		if observer != nil {
			if err != nil {
				_ = observer.MarkUnhealthy(ctx, "charger-operation-recovery", instanceKey)
			} else {
				_ = observer.JobCompleted(ctx, "charger-operation-recovery", instanceKey)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Leases and scheduling share the database clock across CMS instances.
func chargerOperationDatabaseTime(database *gorm.DB) (time.Time, error) {
	var now time.Time
	err := database.Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now, err
}
