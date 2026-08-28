package customerauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HALFactEnvelope remains a source-compatible alias while ingress ownership
// lives in halops.
type HALFactEnvelope = halops.FactEnvelope

func canonicalFactDigest(envelope HALFactEnvelope) (string, error) {
	return halops.CanonicalFactDigest(envelope)
}

// ApplyHALFactProjection is the business projection socket consumed by the
// shared HAL fact ingestor. It retains customer/wallet settlement ownership
// while ingress, integrity, receipt and idempotency are integration concerns.
func (service *Service) ApplyHALFactProjection(tx *gorm.DB, envelope halops.FactEnvelope, payload models.JSONB) error {
	var err error
	emit := true
	switch envelope.FactType {
	case "charger.connection.updated":
		err = service.applyConnectionFact(tx, payload)
	case "connector.status.updated":
		err = service.applyConnectorFact(tx, payload)
	case "command.updated":
		err = service.applyCommandFact(tx, payload)
	case "transaction.started":
		emit, err = service.applyStartedFact(tx, payload)
	case "transaction.meter":
		err = service.applyMeterFact(tx, payload)
	case "transaction.soc":
		err = service.applySoCFact(tx, payload)
	case "transaction.completed":
		err = service.applyCompletedFact(tx, payload)
	default:
		return halops.NewFactProjectionError(400, "unsupported_hal_fact", "The HAL fact type is unsupported.", nil)
	}
	if err != nil || !emit || service.operationalEvents == nil {
		return err
	}
	return service.emitOperationalFact(tx, envelope.FactType, payload)
}

func (service *Service) emitOperationalFact(tx *gorm.DB, factType string, payload models.JSONB) error {
	cpoID, hasCPO := factID(payload, "cpo_id")
	input := operationalrealtime.Input{Data: models.JSONB{}}
	switch factType {
	case "command.updated":
		commandID, ok := factID(payload, "cms_command_id")
		if !ok {
			return nil
		}
		var command models.HALCommandRecord
		if err := tx.Select("cpo_id").First(&command, "cms_command_id = ?", commandID).Error; err != nil {
			return err
		}
		input.CPOID = command.CPOID
		input.Type, input.ResourceType, input.ResourceID = "charging.command_changed", "HAL_COMMAND", commandID.String()
	case "charger.connection.updated":
		if !hasCPO {
			return nil
		}
		sequence, ok := factInt(payload, "connection_sequence")
		if !ok {
			return nil
		}
		input.CPOID = cpoID
		chargerID, ok := factID(payload, "cms_charger_id")
		if !ok {
			return nil
		}
		var runtime models.HALChargerRuntime
		if err := tx.First(&runtime, "cms_charger_id = ?", chargerID).Error; err != nil || runtime.ConnectionSequence != sequence {
			return err
		}
		input.Type, input.ResourceType, input.ResourceID = "charger.live_state_changed", "CHARGER", chargerID.String()
	case "connector.status.updated":
		if !hasCPO {
			return nil
		}
		sequence, ok := factInt(payload, "connector_status_sequence")
		if !ok {
			return nil
		}
		input.CPOID = cpoID
		connectorID, ok := factID(payload, "cms_connector_id")
		if !ok {
			return nil
		}
		var runtime models.HALConnectorRuntime
		if err := tx.First(&runtime, "cms_connector_id = ?", connectorID).Error; err != nil || runtime.ConnectorStatusSequence != sequence {
			return err
		}
		input.Type, input.ResourceType, input.ResourceID = "connector.live_state_changed", "CONNECTOR", connectorID.String()
	case "transaction.started", "transaction.meter", "transaction.soc", "transaction.completed":
		intentID, ok := factID(payload, "cms_start_intent_id")
		if !ok {
			return nil
		}
		var session models.ChargingSession
		if err := tx.Select("id", "cpo_id", "customer_id", "meter_sequence", "soc_sequence").First(&session, "start_intent_id = ?", intentID).Error; err != nil {
			return err
		}
		var emit bool
		input, emit = chargingSessionOperationalEvent(factType, session, payload)
		if !emit {
			return nil
		}
	default:
		return nil
	}
	_, err := service.operationalEvents.Emit(tx, input)
	return err
}

// chargingSessionOperationalEvent keeps transaction invalidation correlation
// testable: CHARGING_SESSION always names the materialized CMS session, never
// the start intent that caused it to exist.
func chargingSessionOperationalEvent(factType string, session models.ChargingSession, payload models.JSONB) (operationalrealtime.Input, bool) {
	input := operationalrealtime.Input{
		CPOID:        session.CPOID,
		CustomerID:   &session.CustomerID,
		ResourceType: "CHARGING_SESSION",
		ResourceID:   session.ID.String(),
		Data:         models.JSONB{},
	}
	switch factType {
	case "transaction.started", "transaction.completed":
		input.Type = "charging.session_changed"
	case "transaction.meter":
		sequence, ok := factInt(payload, "meter_sequence")
		if !ok || session.MeterSequence != sequence {
			return operationalrealtime.Input{}, false
		}
		input.Type = "charging.meter_changed"
	case "transaction.soc":
		sequence, ok := factInt(payload, "soc_sequence")
		if !ok || session.SoCSequence != sequence {
			return operationalrealtime.Input{}, false
		}
		input.Type = "charging.telemetry_changed"
	default:
		return operationalrealtime.Input{}, false
	}
	return input, true
}

func (service *Service) applyConnectionFact(tx *gorm.DB, p models.JSONB) error {
	cpo, charger, err := factIDs(p, "cpo_id", "cms_charger_id")
	if err != nil {
		return err
	}
	state, ok := p["connection_state"].(string)
	if !ok || (state != "ONLINE" && state != "OFFLINE" && state != "UNKNOWN") {
		return invalidFact()
	}
	generation, ok := factInt(p, "connection_generation")
	if !ok {
		return invalidFact()
	}
	sequence, ok := factInt(p, "connection_sequence")
	if !ok {
		return invalidFact()
	}
	observed, ok := factTime(p, "observed_at")
	if !ok {
		return invalidFact()
	}
	identity, ok := p["charger_ocpp_identity"].(string)
	if !ok || identity == "" {
		return invalidFact()
	}
	var chargerRecord models.Charger
	if tx.First(&chargerRecord, "id = ? AND cpo_id = ? AND ocpp_identity = ?", charger, cpo, identity).Error != nil {
		return invalidFact()
	}
	var row models.HALChargerRuntime
	err = tx.First(&row, "cms_charger_id = ?", charger).Error
	if err == gorm.ErrRecordNotFound {
		return tx.Create(&models.HALChargerRuntime{CMSChargerID: charger, CPOID: cpo, ConnectionState: state, ConnectionGeneration: generation, ConnectionSequence: sequence, ObservedAt: observed, UpdatedAt: service.now()}).Error
	}
	if err != nil {
		return err
	}
	if row.CPOID != cpo {
		return invalidFact()
	}
	if sequence <= row.ConnectionSequence {
		return nil
	}
	return tx.Model(&row).Updates(map[string]any{"connection_state": state, "connection_generation": generation, "connection_sequence": sequence, "observed_at": observed, "updated_at": service.now()}).Error
}
func (service *Service) applyConnectorFact(tx *gorm.DB, p models.JSONB) error {
	cpo, charger, err := factIDs(p, "cpo_id", "cms_charger_id")
	if err != nil {
		return err
	}
	_, connector, err := factIDs(p, "cpo_id", "cms_connector_id")
	if err != nil {
		return err
	}
	status, ok := p["ocpp_connector_status"].(string)
	if !ok || status == "" {
		return invalidFact()
	}
	sequence, ok := factInt(p, "connector_status_sequence")
	if !ok {
		return invalidFact()
	}
	observed, ok := factTime(p, "observed_at")
	if !ok {
		return invalidFact()
	}
	identity, ok := p["charger_ocpp_identity"].(string)
	connectorNumber, connectorNumberOK := factInt(p, "ocpp_connector_number")
	if !ok || !connectorNumberOK {
		return invalidFact()
	}
	var chargerRecord models.Charger
	var connectorRecord models.Connector
	if tx.First(&chargerRecord, "id = ? AND cpo_id = ? AND ocpp_identity = ?", charger, cpo, identity).Error != nil ||
		tx.First(&connectorRecord, "id = ? AND charger_id = ? AND cpo_id = ? AND connector_number = ?", connector, charger, cpo, connectorNumber).Error != nil {
		return invalidFact()
	}
	var row models.HALConnectorRuntime
	err = tx.First(&row, "cms_connector_id = ?", connector).Error
	if err == gorm.ErrRecordNotFound {
		return tx.Create(&models.HALConnectorRuntime{CMSConnectorID: connector, CMSChargerID: charger, CPOID: cpo, OCPPConnectorStatus: status, ConnectorStatusSequence: sequence, ObservedAt: observed, UpdatedAt: service.now()}).Error
	}
	if err != nil {
		return err
	}
	if row.CPOID != cpo || row.CMSChargerID != charger {
		return invalidFact()
	}
	if sequence <= row.ConnectorStatusSequence {
		return nil
	}
	return tx.Model(&row).Updates(map[string]any{"ocpp_connector_status": status, "connector_status_sequence": sequence, "observed_at": observed, "updated_at": service.now()}).Error
}
func (service *Service) applyCommandFact(tx *gorm.DB, p models.JSONB) error {
	commandID, ok := factID(p, "cms_command_id")
	if !ok {
		return invalidFact()
	}
	state, ok := p["state"].(string)
	if !ok {
		return invalidFact()
	}
	var command models.HALCommandRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ?", commandID).Error; err != nil {
		return invalidFact()
	}
	// A later command fact may be delivered after the authoritative start fact.
	// MATERIALIZED is terminal command evidence for this purpose and must never
	// regress to an earlier delivery acknowledgement.
	if command.State == "MATERIALIZED" && state != "MATERIALIZED" {
		return nil
	}
	return tx.Model(&command).Updates(map[string]any{"state": state, "updated_at": service.now()}).Error
}
func (service *Service) applyStartedFact(tx *gorm.DB, p models.JSONB) (bool, error) {
	evidence, err := startEvidenceFromFact(p)
	if err != nil {
		return false, err
	}
	_, materialized, err := service.materializeAuthoritativeStart(tx, evidence)
	return materialized, err
}

// MaterializeAuthoritativeStart is the reconciliation socket used after HAL
// confirms a transaction by the exact CMS start-intent ID. It deliberately
// shares the same transaction-local materializer as immutable fact ingress;
// neither path can invent a session when HAL returns no transaction.
func (service *Service) MaterializeAuthoritativeStart(ctx context.Context, evidence halops.StartEvidence) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, materialized, err := service.materializeAuthoritativeStart(tx, evidence)
		if err != nil || !materialized || service.operationalEvents == nil {
			return err
		}
		return service.emitStartedOperationalEvent(tx, *session)
	})
}

func (service *Service) materializeAuthoritativeStart(tx *gorm.DB, evidence halops.StartEvidence) (*models.ChargingSession, bool, error) {
	if evidence.HALTransactionID == uuid.Nil || evidence.HALCommandID == uuid.Nil || evidence.CMSCommandID == uuid.Nil || evidence.CMSStartIntentID == uuid.Nil || evidence.CPOID == uuid.Nil || evidence.CMSChargerID == uuid.Nil || evidence.CMSConnectorID == uuid.Nil || evidence.OCPPTransactionID < 1 || evidence.MeterStartWh < 0 || evidence.ActualStartedAt.IsZero() || strings.TrimSpace(evidence.ChargerOCPPIdentity) == "" || evidence.OCPPConnectorNumber < 1 {
		return nil, false, invalidFact()
	}
	var intent models.ChargingStartIntent
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", evidence.CMSStartIntentID).Error; err != nil {
		return nil, false, invalidFact()
	}
	var command models.HALCommandRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ? AND start_intent_id = ?", evidence.CMSCommandID, intent.ID).Error; err != nil {
		return nil, false, invalidFact()
	}
	if establishedHALCommandIdentityConflicts(command.HALCommandID, evidence.HALCommandID) {
		return nil, false, halops.NewFactProjectionError(409, "hal_start_evidence_conflict", "The HAL start evidence conflicts with the recorded command.", nil)
	}
	if establishedHALCommandIdentityConflicts(intent.HALCommandID, evidence.HALCommandID) {
		return nil, false, halops.NewFactProjectionError(409, "hal_start_evidence_conflict", "The HAL start evidence conflicts with the recorded intent.", nil)
	}
	if intent.CPOID != evidence.CPOID || intent.ChargerID != evidence.CMSChargerID || intent.ConnectorID != evidence.CMSConnectorID {
		return nil, false, halops.NewFactProjectionError(409, "hal_start_evidence_conflict", "The HAL start evidence conflicts with the recorded intent.", nil)
	}
	var charger models.Charger
	var connector models.Connector
	if tx.First(&charger, "id = ? AND cpo_id = ? AND ocpp_identity = ?", intent.ChargerID, intent.CPOID, evidence.ChargerOCPPIdentity).Error != nil ||
		tx.First(&connector, "id = ? AND charger_id = ? AND cpo_id = ? AND connector_number = ?", intent.ConnectorID, intent.ChargerID, intent.CPOID, evidence.OCPPConnectorNumber).Error != nil {
		return nil, false, invalidFact()
	}
	if intent.Status == constants.StartIntentStatusExpired || intent.Status == constants.StartIntentStatusRejected {
		now := service.now()
		if err := tx.Model(&command).Updates(map[string]any{"hal_command_id": evidence.HALCommandID, "state": "RECONCILIATION_REQUIRED", "last_error_category": "late_authoritative_start", "last_error_detail": "HAL confirmed a start after CMS terminalized the intent", "updated_at": now}).Error; err != nil {
			return nil, false, err
		}
		if err := tx.Model(&intent).Updates(map[string]any{"status": constants.StartIntentStatusReconciliation, "hal_command_id": evidence.HALCommandID, "updated_at": now}).Error; err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	var session models.ChargingSession
	if err := tx.First(&session, "start_intent_id = ?", intent.ID).Error; err == nil {
		if session.CPOID != intent.CPOID || session.CustomerID != intent.CustomerID || session.ChargerID != intent.ChargerID || session.ConnectorID != intent.ConnectorID || session.HALTransactionID == nil || *session.HALTransactionID != evidence.HALTransactionID || session.TransactionID != evidence.OCPPTransactionID || session.MeterStartWh != evidence.MeterStartWh || !session.StartTime.Equal(evidence.ActualStartedAt) {
			return nil, false, halops.NewFactProjectionError(409, "hal_start_evidence_conflict", "The HAL start evidence conflicts with the materialized session.", nil)
		}
		return &session, false, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	if intent.Status == constants.StartIntentStatusActuallyStarted || intent.MaterializedSessionID != nil {
		return nil, false, halops.NewFactProjectionError(409, "hal_start_evidence_conflict", "The recorded start intent is incomplete or contradictory.", nil)
	}
	session = materializedChargingSession(intent, evidence.OCPPTransactionID, evidence.HALTransactionID, evidence.ActualStartedAt, evidence.MeterStartWh, service.now())
	if err := tx.Create(&session).Error; err != nil {
		return nil, false, err
	}
	now := service.now()
	if err := tx.Model(&command).Updates(map[string]any{"hal_command_id": evidence.HALCommandID, "state": "MATERIALIZED", "last_error_category": "", "last_error_detail": "", "updated_at": now}).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Model(&intent).Updates(map[string]any{"status": constants.StartIntentStatusActuallyStarted, "materialized_session_id": session.ID, "hal_command_id": evidence.HALCommandID, "updated_at": now}).Error; err != nil {
		return nil, false, err
	}
	return &session, true, nil
}

// A nil identifier is unknown and a historical zero UUID is invalid external
// input, not an established HAL identity. Only two different nonzero values
// are a true identity conflict.
func establishedHALCommandIdentityConflicts(recorded *uuid.UUID, authoritative uuid.UUID) bool {
	return recorded != nil && *recorded != uuid.Nil && *recorded != authoritative
}

func startEvidenceFromFact(p models.JSONB) (halops.StartEvidence, error) {
	intentID, intentOK := factID(p, "cms_start_intent_id")
	commandID, commandOK := factID(p, "cms_command_id")
	halCommandID, halCommandOK := factID(p, "hal_command_id")
	halTransactionID, halTransactionOK := factID(p, "hal_transaction_id")
	cpoID, cpoOK := factID(p, "cpo_id")
	chargerID, chargerOK := factID(p, "cms_charger_id")
	connectorID, connectorOK := factID(p, "cms_connector_id")
	ocppID, ocppOK := factInt(p, "ocpp_transaction_id")
	meter, meterOK := factInt(p, "meter_start_wh")
	startedAt, startedOK := factTime(p, "started_at")
	identity, identityOK := p["charger_ocpp_identity"].(string)
	connectorNumber, connectorNumberOK := factInt(p, "ocpp_connector_number")
	if !intentOK || !commandOK || !halCommandOK || !halTransactionOK || !cpoOK || !chargerOK || !connectorOK || !ocppOK || !meterOK || !startedOK || !identityOK || !connectorNumberOK {
		return halops.StartEvidence{}, invalidFact()
	}
	return halops.StartEvidence{HALTransactionID: halTransactionID, HALCommandID: halCommandID, CMSCommandID: commandID, CMSStartIntentID: intentID, CPOID: cpoID, CMSChargerID: chargerID, CMSConnectorID: connectorID, ChargerOCPPIdentity: identity, OCPPConnectorNumber: int(connectorNumber), OCPPTransactionID: ocppID, MeterStartWh: meter, ActualStartedAt: startedAt}, nil
}

func (service *Service) emitStartedOperationalEvent(tx *gorm.DB, session models.ChargingSession) error {
	_, err := service.operationalEvents.Emit(tx, operationalrealtime.Input{CPOID: session.CPOID, CustomerID: &session.CustomerID, Type: "charging.session_changed", ResourceType: "CHARGING_SESSION", ResourceID: session.ID.String(), Data: models.JSONB{}})
	return err
}

// materializedChargingSession copies the commercial decision already frozen on
// the start intent. It never re-reads the mutable live tariff when HAL later
// reports that physical charging started.
func materializedChargingSession(intent models.ChargingStartIntent, ocppID int64, halTransactionID uuid.UUID, startedAt time.Time, meterStartWh int64, now time.Time) models.ChargingSession {
	return models.ChargingSession{
		ID:               uuid.New(),
		CPOID:            intent.CPOID,
		StartIntentID:    &intent.ID,
		HALTransactionID: &halTransactionID,
		TransactionID:    ocppID,
		CustomerID:       intent.CustomerID,
		ChargerID:        intent.ChargerID,
		ConnectorID:      intent.ConnectorID,
		TariffID:         intent.TariffID,
		StartTime:        startedAt,
		MeterStartWh:     meterStartWh,
		TotalKWh:         decimal.Zero,
		TotalAmount:      decimal.Zero,
		Currency:         stringValue(intent.TariffSnapshot, "currency", "INR"),
		TariffSnapshot:   intent.TariffSnapshot,
		TaxSnapshot:      intent.TaxSnapshot,
		Status:           constants.SessionStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}
func (service *Service) applyMeterFact(tx *gorm.DB, p models.JSONB) error {
	halTx, ok := factID(p, "hal_transaction_id")
	if !ok {
		return invalidFact()
	}
	sequence, ok := factInt(p, "meter_sequence")
	if !ok {
		return invalidFact()
	}
	meter, ok := factInt(p, "meter_value_wh")
	if !ok {
		return invalidFact()
	}
	observed, ok := factTime(p, "meter_observed_at")
	if !ok {
		return invalidFact()
	}
	var session models.ChargingSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "hal_transaction_id = ?", halTx).Error; err != nil {
		return invalidFact()
	}
	if session.Status != constants.SessionStatusActive && session.Status != constants.SessionStatusStopPending {
		return nil
	}
	if meter < session.MeterStartWh || sequence <= session.MeterSequence {
		return nil
	}
	return tx.Model(&session).Updates(map[string]any{"latest_meter_wh": meter, "meter_observed_at": observed, "meter_sequence": sequence, "updated_at": service.now()}).Error
}

// applySoCFact deliberately has no energy requirements. The new immutable
// fact represents only a valid charger-observed SoC transition, with its own
// ordering stream, so missing SoC never clears state and a stale SoC cannot
// suppress an independently valid meter update.
func (service *Service) applySoCFact(tx *gorm.DB, p models.JSONB) error {
	halTx, ok := factID(p, "hal_transaction_id")
	if !ok {
		return invalidFact()
	}
	sequence, ok := factInt(p, "soc_sequence")
	if !ok || sequence < 1 {
		return invalidFact()
	}
	soc, ok := factSoC(p, "soc_percent")
	if !ok {
		return invalidFact()
	}
	observed, ok := factTime(p, "soc_observed_at")
	if !ok {
		return invalidFact()
	}
	var session models.ChargingSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "hal_transaction_id = ?", halTx).Error; err != nil {
		return invalidFact()
	}
	// A SoC fact accepted by HAL before completion can be delivered after the
	// completion fact. Preserve that real historical observation; HAL refuses
	// fresh SoC after completion, so this does not reopen a transaction.
	if sequence <= session.SoCSequence || (session.SoCObservedAt != nil && !observed.After(*session.SoCObservedAt)) {
		return nil
	}
	updates := map[string]any{"latest_soc_percent": soc, "soc_observed_at": observed, "soc_sequence": sequence, "updated_at": service.now()}
	if session.InitialSoCPercent == nil {
		updates["initial_soc_percent"] = soc
	}
	return tx.Model(&session).Updates(updates).Error
}
func (service *Service) applyCompletedFact(tx *gorm.DB, p models.JSONB) error {
	halTx, ok := factID(p, "hal_transaction_id")
	if !ok {
		return invalidFact()
	}
	meterStop, ok := factInt(p, "meter_stop_wh")
	if !ok {
		return invalidFact()
	}
	stopped, ok := factTime(p, "stopped_at")
	if !ok {
		return invalidFact()
	}
	var session models.ChargingSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "hal_transaction_id = ?", halTx).Error; err != nil {
		return invalidFact()
	}
	if session.Status == constants.SessionStatusCompleted {
		return nil
	}
	if meterStop < session.MeterStartWh {
		return invalidFact()
	}
	ocppID, ok := factInt(p, "ocpp_transaction_id")
	if !ok || ocppID != session.TransactionID {
		return invalidFact()
	}
	amount, err := chargingAmount(session.TariffSnapshot, session.TaxSnapshot, meterStop-session.MeterStartWh, session.StartTime, stopped)
	if err != nil {
		return err
	}
	updates := map[string]any{"meter_stop_wh": meterStop, "latest_meter_wh": meterStop, "meter_observed_at": stopped, "end_time": stopped, "total_kwh": decimal.NewFromInt(meterStop - session.MeterStartWh).Div(decimal.NewFromInt(1000)), "total_amount": amount, "status": constants.SessionStatusReconciliationRequired, "settlement_status": "RECONCILIATION_REQUIRED", "updated_at": service.now()}
	if reason, ok := factString(p, "stop_reason"); ok {
		updates["stop_reason"] = reason
	}
	if err := tx.Model(&session).Updates(updates).Error; err != nil {
		return err
	}
	session.MeterStopWh, session.LatestMeterWh, session.MeterObservedAt, session.EndTime, session.TotalAmount = &meterStop, &meterStop, &stopped, &stopped, amount
	session.TotalKWh, session.Status, session.SettlementStatus = decimal.NewFromInt(meterStop-session.MeterStartWh).Div(decimal.NewFromInt(1000)), constants.SessionStatusReconciliationRequired, "RECONCILIATION_REQUIRED"
	return service.settleCompletedSession(tx, &session)
}

// settleCompletedSession is called under the session transaction by both the
// immutable completion fact and bounded recovery. Its payment identity is the
// session ID, so retries cannot debit a wallet twice.
func (service *Service) settleCompletedSession(tx *gorm.DB, session *models.ChargingSession) error {
	if session.EndTime == nil || session.MeterStopWh == nil {
		return errors.New("completed settlement lacks terminal session evidence")
	}
	var hold models.WalletHold
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hold, "start_intent_id = ?", session.StartIntentID).Error; err != nil {
		return err
	}
	now := service.now()
	markReconciliation := func() error {
		if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusReconciling, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": constants.SessionStatusReconciliationRequired, "settlement_status": "RECONCILIATION_REQUIRED", "updated_at": now}).Error
	}
	var payment models.Payment
	err := tx.First(&payment, "session_id = ?", session.ID).Error
	if err == nil {
		if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusCaptured, "captured_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": constants.SessionStatusCompleted, "settlement_status": "SETTLED", "updated_at": now}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if session.TotalAmount.GreaterThan(hold.Amount) {
		return markReconciliation()
	}
	if session.TotalAmount.IsZero() {
		if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusCaptured, "captured_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": constants.SessionStatusCompleted, "settlement_status": "SETTLED", "updated_at": now}).Error
	}
	var wallet models.Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "id = ?", hold.WalletID).Error; err != nil {
		return err
	}
	if wallet.Balance.LessThan(session.TotalAmount) {
		return markReconciliation()
	}
	ledger := models.WalletTransaction{ID: uuid.New(), CPOID: session.CPOID, WalletID: wallet.ID, SessionID: &session.ID, Amount: session.TotalAmount, TransactionType: constants.WalletTransactionTypeDebit, Description: "Charging session settlement", IdempotencyKey: chargingStringPtr("charging-settlement-" + session.ID.String()), Status: constants.FinancialStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}
	if err := tx.Create(&models.Payment{ID: uuid.New(), CPOID: session.CPOID, SessionID: session.ID, WalletTransactionID: ledger.ID, Amount: session.TotalAmount, PaymentMethod: "WALLET", Status: constants.FinancialStatusCompleted, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&wallet).Updates(map[string]any{"balance": wallet.Balance.Sub(session.TotalAmount), "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusCaptured, "captured_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	return tx.Model(&models.ChargingSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": constants.SessionStatusCompleted, "settlement_status": "SETTLED", "updated_at": now}).Error
}

// ReconcileCompletedSettlements makes durable completion evidence recoverable
// after a wallet top-up or transient financial failure without replaying HAL.
func (service *Service) ReconcileCompletedSettlements(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 50
	}
	var ids []uuid.UUID
	if err := service.database.WithContext(ctx).Model(&models.ChargingSession{}).Where("status = ? AND settlement_status = ?", constants.SessionStatusReconciliationRequired, "RECONCILIATION_REQUIRED").Order("updated_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var session models.ChargingSession
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ?", id).Error; err != nil {
				return err
			}
			if session.Status != constants.SessionStatusReconciliationRequired || session.SettlementStatus != "RECONCILIATION_REQUIRED" {
				return nil
			}
			return service.settleCompletedSession(tx, &session)
		}); err != nil {
			return fmt.Errorf("reconcile completed session %s: %w", id, err)
		}
	}
	return nil
}

func chargingAmount(tariff, tax models.JSONB, consumedWh int64, startedAt, stoppedAt time.Time) (decimal.Decimal, error) {
	return commercial.SessionAmountFromSnapshots(
		tariff,
		tax,
		consumedWh,
		startedAt,
		stoppedAt,
	)
}
func factID(p models.JSONB, key string) (uuid.UUID, bool) {
	v, ok := p[key].(string)
	if !ok {
		return uuid.Nil, false
	}
	id, e := uuid.Parse(v)
	return id, e == nil && id != uuid.Nil
}
func factString(p models.JSONB, key string) (string, bool) {
	v, ok := p[key].(string)
	return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
}
func factIDs(p models.JSONB, first, second string) (uuid.UUID, uuid.UUID, error) {
	a, ok := factID(p, first)
	if !ok {
		return uuid.Nil, uuid.Nil, invalidFact()
	}
	b, ok := factID(p, second)
	if !ok {
		return uuid.Nil, uuid.Nil, invalidFact()
	}
	return a, b, nil
}
func factInt(p models.JSONB, key string) (int64, bool) {
	v, ok := p[key].(float64)
	if !ok || v != float64(int64(v)) {
		return 0, false
	}
	return int64(v), true
}
func factSoC(p models.JSONB, key string) (decimal.Decimal, bool) {
	raw, ok := p[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return decimal.Zero, false
	}
	value, err := decimal.NewFromString(raw)
	if err != nil || value.IsNegative() || value.GreaterThan(decimal.NewFromInt(100)) || value.Exponent() < -3 {
		return decimal.Zero, false
	}
	return value, true
}
func factTime(p models.JSONB, key string) (time.Time, bool) {
	v, ok := p[key].(string)
	if !ok {
		return time.Time{}, false
	}
	t, e := time.Parse(time.RFC3339, v)
	return t, e == nil
}
func invalidFact() error {
	return halops.NewFactProjectionError(422, "invalid_hal_fact", "The HAL fact cannot be safely applied.", nil)
}
func stringValue(p models.JSONB, key, def string) string {
	v, ok := p[key].(string)
	if !ok {
		return def
	}
	return v
}
func chargingStringPtr(value string) *string { return &value }
