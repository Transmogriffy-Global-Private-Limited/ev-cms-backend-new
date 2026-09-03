package customerauth

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// chargingTraceRoot carries only authoritative identifiers already known to a
// CMS transaction. It is metadata for navigating diagnostic evidence, never a
// source from which charging or commercial state may be reconstructed.
type chargingTraceRoot struct {
	StartIntentID *uuid.UUID
	SessionID     *uuid.UUID
	CommandID     *uuid.UUID
}

// recordChargingTrace is an append-only diagnostic helper. Callers remain
// responsible for their authoritative transaction/session mutation; failure is
// deliberately not interpreted as charging, billing, or connector truth.
func (service *Service) recordChargingTrace(tx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, sessionID *uuid.UUID, source, target, category, protocol, phase, summary, correlation string, data models.JSONB) error {
	return service.recordChargingTraceWithRoot(tx, traceID, cpoID, chargingTraceRoot{SessionID: sessionID}, source, target, category, protocol, phase, summary, correlation, data)
}

func (service *Service) recordChargingTraceWithRoot(tx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, linkage chargingTraceRoot, source, target, category, protocol, phase, summary, correlation string, data models.JSONB) error {
	data = sanitizedChargingTraceData(data)
	// Charging trace rows are diagnostic only. A failed INSERT inside a PostgreSQL
	// transaction would otherwise mark the whole transaction aborted even when a
	// caller deliberately ignores this error. GORM uses a savepoint for nested
	// transactions, so returning the insert error rolls back only the trace write.
	return tx.Transaction(func(traceTx *gorm.DB) error {
		now := service.now()
		root := models.ChargingTrace{TraceID: traceID, CPOID: cpoID, CMSStartIntentID: linkage.StartIntentID, CMSChargingSessionID: linkage.SessionID, CMSCommandID: linkage.CommandID, CreatedAt: now, UpdatedAt: now}
		if err := traceTx.Where("trace_id = ?", traceID).FirstOrCreate(&root).Error; err != nil {
			return err
		}
		if root.CPOID != cpoID {
			return gorm.ErrRecordNotFound
		}
		updates := chargingTraceRootUpdates(root, linkage, now)
		if err := traceTx.Model(&root).Updates(updates).Error; err != nil {
			return err
		}
		return traceTx.Create(&models.ChargingTraceEvent{ID: uuid.New(), TraceID: traceID, CPOID: cpoID, SessionID: linkage.SessionID, Source: source, Target: target, Category: category, Protocol: protocol, Phase: phase, Summary: summary, OccurredAt: now, RecordedAt: now, CorrelationID: correlation, Data: data}).Error
	})
}

func chargingTraceRootUpdates(root models.ChargingTrace, linkage chargingTraceRoot, now time.Time) map[string]any {
	updates := map[string]any{"updated_at": now}
	if root.CMSStartIntentID == nil && linkage.StartIntentID != nil {
		updates["cms_start_intent_id"] = *linkage.StartIntentID
	}
	if root.CMSChargingSessionID == nil && linkage.SessionID != nil {
		updates["cms_charging_session_id"] = *linkage.SessionID
	}
	if root.CMSCommandID == nil && linkage.CommandID != nil {
		updates["cms_command_id"] = *linkage.CommandID
	}
	return updates
}

func sanitizedChargingTraceData(input models.JSONB) models.JSONB {
	output := models.JSONB{}
	for _, key := range []string{
		"start_intent_id", "cms_command_id", "hal_command_id", "connector_id", "hal_transaction_id", "ocpp_transaction_id",
		"state", "error_class", "meter_wh", "reason", "amount", "currency", "status", "settlement_status",
		"wallet_hold_id", "wallet_transaction_id", "payment_id", "limit_type", "energy_limit_wh", "energy_limit_source",
		"max_duration_seconds", "duration_limit_source", "stop_reason",
	} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}
