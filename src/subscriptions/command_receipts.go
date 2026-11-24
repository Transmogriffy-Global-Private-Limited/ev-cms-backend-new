package subscriptions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// canonicalSubscriptionMutationRequest mirrors the fields the API actually
// executes. A caller's harmless surrounding whitespace must not split one
// idempotency key into two different request fingerprints.
func canonicalSubscriptionMutationRequest(request any, reason, key string) (any, error) {
	switch value := request.(type) {
	case RenewRequest:
		value.Reason, value.IdempotencyKey = reason, key
		return value, nil
	case ChangePlanRequest:
		value.Reason, value.IdempotencyKey = reason, key
		return value, nil
	case TransitionRequest:
		value.Reason, value.IdempotencyKey = reason, key
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported subscription mutation request %T", request)
	}
}

// validSubscriptionCommandReceipt rejects an inconsistent stored response.
// The receipt is a frozen audit result, not a license to return data from a
// different subscription or plan to the authenticated caller.
func validSubscriptionCommandReceipt(history models.CPOSubscriptionHistory, view SubscriptionView) bool {
	return history.SubscriptionID != uuid.Nil &&
		history.CPOID != uuid.Nil &&
		view.Subscription.ID == history.SubscriptionID &&
		view.Subscription.CPOID == history.CPOID &&
		view.Subscription.Status == history.NextStatus &&
		view.Subscription.PlanVersionID == history.NextPlanVersionID &&
		view.Version.ID == history.NextPlanVersionID &&
		view.Version.PlanID == view.Plan.ID &&
		view.Plan.ID != uuid.Nil
}

// subscriptionCommandDigest identifies the caller's command, not the mutable
// subscription state. Time omitted by the user stays omitted in the digest.
func subscriptionCommandDigest(operation string, cpoID uuid.UUID, request any) (string, error) {
	encoded, err := json.Marshal(struct {
		Operation string    `json:"operation"`
		CPOID     uuid.UUID `json:"cpo_id"`
		Request   any       `json:"request"`
	}{operation, cpoID, request})
	if err != nil {
		return "", fmt.Errorf("encode subscription request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// The preexisting history uniqueness (actor_user_id,idempotency_key) is the
// authoritative durable receipt key. The advisory transaction lock only
// serializes simultaneous attempts BEFORE the history insert.
func (service *Service) replaySubscriptionCommand(tx *gorm.DB, actor uuid.UUID, key, operation string, cpoID uuid.UUID, request any) (SubscriptionView, bool, error) {
	lockHash := fnv.New64a()
	_, _ = lockHash.Write(actor[:])
	_, _ = lockHash.Write([]byte{0})
	_, _ = lockHash.Write([]byte(key))
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(lockHash.Sum64())).Error; err != nil {
		return SubscriptionView{}, false, fmt.Errorf("lock subscription command key: %w", err)
	}
	digest, err := subscriptionCommandDigest(operation, cpoID, request)
	if err != nil {
		return SubscriptionView{}, false, err
	}
	var history models.CPOSubscriptionHistory
	result := tx.Where("actor_user_id = ? AND idempotency_key = ?", actor, key).First(&history)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return SubscriptionView{}, false, nil
	}
	if result.Error != nil {
		return SubscriptionView{}, false, result.Error
	}
	if history.CPOID != cpoID || fmt.Sprint(history.Metadata["operation"]) != operation {
		return SubscriptionView{}, false, conflict("idempotency_conflict", "This key was previously used for a different subscription command.")
	}
	if history.Metadata["request_hash"] == nil {
		return SubscriptionView{}, false, conflict("legacy_idempotency_result_unavailable", "Old command result is not replayable; refresh state before issuing a new action.")
	}
	if history.Metadata["request_hash"] != digest {
		return SubscriptionView{}, false, conflict("idempotency_conflict", "Idempotency key belongs to a different payload.")
	}
	snapshot, ok := history.Metadata["response_snapshot"]
	if !ok {
		// Historical receipts had no immutable request/result snapshot. Do
		// not invent the original outcome by reading today's mutable row.
		return SubscriptionView{}, false, conflict("legacy_idempotency_result_unavailable", "The old command was recorded without a replayable response. Refresh the subscription before taking a new action.")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return SubscriptionView{}, false, err
	}
	var view SubscriptionView
	if err := json.Unmarshal(raw, &view); err != nil {
		return SubscriptionView{}, false, conflict("idempotency_receipt_corrupt", "The previously recorded subscription command result is unreadable.")
	}
	if !validSubscriptionCommandReceipt(history, view) {
		return SubscriptionView{}, false, conflict("idempotency_receipt_corrupt", "The previously recorded subscription command result is inconsistent.")
	}
	return view, true, nil
}

// Called inside the SAME transaction as the business state, history, audit,
// event, and therefore rolled back with them on any error.
func (service *Service) recordSubscriptionCommandSnapshot(tx *gorm.DB, actor uuid.UUID, key, operation string, cpoID uuid.UUID, request any, view SubscriptionView) error {
	digest, err := subscriptionCommandDigest(operation, cpoID, request)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return err
	}
	var snapshot models.JSONB
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return err
	}
	metadata := models.JSONB{"operation": operation, "request_hash": digest,
		"response_snapshot": snapshot, "period_sequence": view.Subscription.CurrentPeriodSequence,
		"system_actor": false}
	result := tx.Model(&models.CPOSubscriptionHistory{}).
		Where("actor_user_id = ? AND idempotency_key = ?", actor, key).
		Update("metadata", metadata)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("subscription history snapshot missing")
	}
	return nil
}
