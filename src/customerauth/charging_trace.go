package customerauth

import (
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// recordChargingTrace is an append-only diagnostic helper. Callers remain
// responsible for their authoritative transaction/session mutation; failure is
// deliberately not interpreted as charging, billing, or connector truth.
func (service *Service) recordChargingTrace(tx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, sessionID *uuid.UUID, source, target, category, protocol, phase, summary, correlation string, data models.JSONB) error {
	data = sanitizedChargingTraceData(data)
	now := service.now()
	// Charging trace rows are diagnostic only. A failed INSERT inside a PostgreSQL
	// transaction would otherwise mark the whole transaction aborted even when a
	// caller deliberately ignores this error. GORM uses a savepoint for nested
	// transactions, so returning the insert error rolls back only the trace write.
	return tx.Transaction(func(traceTx *gorm.DB) error {
		return traceTx.Create(&models.ChargingTraceEvent{ID: uuid.New(), TraceID: traceID, CPOID: cpoID, SessionID: sessionID, Source: source, Target: target, Category: category, Protocol: protocol, Phase: phase, Summary: summary, OccurredAt: now, RecordedAt: now, CorrelationID: correlation, Data: data}).Error
	})
}

func sanitizedChargingTraceData(input models.JSONB) models.JSONB {
	output := models.JSONB{}
	for _, key := range []string{"start_intent_id", "cms_command_id", "hal_command_id", "connector_id", "hal_transaction_id", "ocpp_transaction_id", "state", "error_class", "meter_wh", "reason"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}
