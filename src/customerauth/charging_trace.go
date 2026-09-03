package customerauth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// chargingTraceRoot carries only authoritative identifiers already known to a
// CMS transaction. It is metadata for navigating diagnostic evidence, never a
// source from which charging or commercial state may be reconstructed.
type chargingTraceRoot struct {
	StartIntentID       *uuid.UUID
	SessionID           *uuid.UUID
	CommandID           *uuid.UUID
	HALTransactionID    *uuid.UUID
	OCPPTransactionID   *int64
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
}

var errChargingTraceRootIdentityConflict = errors.New("charging trace root identity conflict")

// recordChargingTrace is an append-only diagnostic helper. Callers remain
// responsible for their authoritative transaction/session mutation; failure is
// deliberately not interpreted as charging, billing, or connector truth.
func (service *Service) recordChargingTrace(tx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, sessionID *uuid.UUID, source, target, category, protocol, phase, summary, correlation string, data models.JSONB) error {
	return service.recordChargingTraceWithRoot(tx, traceID, cpoID, chargingTraceRoot{SessionID: sessionID}, source, target, category, protocol, phase, summary, correlation, data)
}

func (service *Service) recordChargingTraceWithRoot(tx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, linkage chargingTraceRoot, source, target, category, protocol, phase, summary, correlation string, data models.JSONB) error {
	data = sanitizedChargingTraceData(data)
	// Root enrichment and event append are independent diagnostic writes. Each
	// gets its own savepoint, so an event-insert failure cannot roll back a root
	// identity that was already bound from authoritative CMS state.
	if err := tx.Transaction(func(traceTx *gorm.DB) error {
		return service.enrichChargingTraceRoot(traceTx, traceID, cpoID, linkage)
	}); err != nil {
		return err
	}
	return tx.Transaction(func(traceTx *gorm.DB) error {
		now := service.now()
		return traceTx.Create(&models.ChargingTraceEvent{ID: uuid.New(), TraceID: traceID, CPOID: cpoID, SessionID: linkage.SessionID, Source: source, Target: target, Category: category, Protocol: protocol, Phase: phase, Summary: summary, OccurredAt: now, RecordedAt: now, CorrelationID: correlation, Data: data}).Error
	})
}

func (service *Service) enrichChargingTraceRoot(traceTx *gorm.DB, traceID uuid.UUID, cpoID uuid.UUID, linkage chargingTraceRoot) error {
	now := service.now()
	root := chargingTraceRootModel(traceID, cpoID, linkage, now)
	err := traceTx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&root, "trace_id = ?", traceID).Error
	if err == gorm.ErrRecordNotFound {
		if err := traceTx.Create(&root).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if root.CPOID != cpoID {
		return gorm.ErrRecordNotFound
	}
	updates, err := chargingTraceRootUpdates(root, linkage, now)
	if err != nil {
		return err
	}
	if len(updates) > 0 {
		if err := traceTx.Model(&root).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func chargingTraceRootModel(traceID, cpoID uuid.UUID, linkage chargingTraceRoot, now time.Time) models.ChargingTrace {
	return models.ChargingTrace{
		TraceID: traceID, CPOID: cpoID,
		CMSStartIntentID: linkage.StartIntentID, CMSChargingSessionID: linkage.SessionID, CMSCommandID: linkage.CommandID,
		HALTransactionID: linkage.HALTransactionID, OCPPTransactionID: linkage.OCPPTransactionID,
		ChargerOCPPIdentity: strings.TrimSpace(linkage.ChargerOCPPIdentity), OCPPConnectorNumber: linkage.OCPPConnectorNumber,
		CreatedAt: now, UpdatedAt: now,
	}
}

func chargingTraceRootUpdates(root models.ChargingTrace, linkage chargingTraceRoot, now time.Time) (map[string]any, error) {
	updates := map[string]any{}
	for _, identity := range []struct {
		column   string
		current  *uuid.UUID
		incoming *uuid.UUID
	}{
		{"cms_start_intent_id", root.CMSStartIntentID, linkage.StartIntentID},
		{"cms_charging_session_id", root.CMSChargingSessionID, linkage.SessionID},
		{"cms_command_id", root.CMSCommandID, linkage.CommandID},
		{"hal_transaction_id", root.HALTransactionID, linkage.HALTransactionID},
	} {
		if err := monotonicTraceUUID(identity.column, identity.current, identity.incoming); err != nil {
			return nil, err
		}
		if identity.current == nil && identity.incoming != nil {
			updates[identity.column] = *identity.incoming
		}
	}
	if err := monotonicTraceInt64("ocpp_transaction_id", root.OCPPTransactionID, linkage.OCPPTransactionID); err != nil {
		return nil, err
	}
	if root.OCPPTransactionID == nil && linkage.OCPPTransactionID != nil {
		updates["ocpp_transaction_id"] = *linkage.OCPPTransactionID
	}
	incomingIdentity := strings.TrimSpace(linkage.ChargerOCPPIdentity)
	if root.ChargerOCPPIdentity == "" && incomingIdentity != "" {
		updates["charger_ocpp_identity"] = incomingIdentity
	} else if root.ChargerOCPPIdentity != "" && incomingIdentity != "" && root.ChargerOCPPIdentity != incomingIdentity {
		return nil, traceRootConflict("charger_ocpp_identity")
	}
	if root.OCPPConnectorNumber == 0 && linkage.OCPPConnectorNumber > 0 {
		updates["ocpp_connector_number"] = linkage.OCPPConnectorNumber
	} else if root.OCPPConnectorNumber > 0 && linkage.OCPPConnectorNumber > 0 && root.OCPPConnectorNumber != linkage.OCPPConnectorNumber {
		return nil, traceRootConflict("ocpp_connector_number")
	}
	if len(updates) > 0 {
		updates["updated_at"] = now
	}
	return updates, nil
}

func monotonicTraceUUID(column string, current, incoming *uuid.UUID) error {
	if current != nil && incoming != nil && *current != *incoming {
		return traceRootConflict(column)
	}
	return nil
}

func monotonicTraceInt64(column string, current, incoming *int64) error {
	if current != nil && incoming != nil && *current != *incoming {
		return traceRootConflict(column)
	}
	return nil
}

func traceRootConflict(column string) error {
	return fmt.Errorf("%w: %s", errChargingTraceRootIdentityConflict, column)
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
