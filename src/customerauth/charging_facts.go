package customerauth

import (
	"net/http"
	"time"

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
	switch envelope.FactType {
	case "charger.connection.updated":
		err = service.applyConnectionFact(tx, payload)
	case "connector.status.updated":
		err = service.applyConnectorFact(tx, payload)
	case "command.updated":
		err = service.applyCommandFact(tx, payload)
	case "transaction.started":
		err = service.applyStartedFact(tx, payload)
	case "transaction.meter":
		err = service.applyMeterFact(tx, payload)
	case "transaction.completed":
		err = service.applyCompletedFact(tx, payload)
	default:
		return &APIError{http.StatusBadRequest, "unsupported_hal_fact", "The HAL fact type is unsupported."}
	}
	if err != nil || service.operationalEvents == nil {
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
	case "transaction.started", "transaction.meter", "transaction.completed":
		intentID, ok := factID(payload, "cms_start_intent_id")
		if !ok {
			return nil
		}
		var session models.ChargingSession
		if err := tx.Select("id", "cpo_id", "customer_id", "meter_sequence").First(&session, "start_intent_id = ?", intentID).Error; err != nil {
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
	return tx.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{"state": state, "updated_at": service.now()}).Error
}
func (service *Service) applyStartedFact(tx *gorm.DB, p models.JSONB) error {
	intentID, ok := factID(p, "cms_start_intent_id")
	if !ok {
		return invalidFact()
	}
	commandID, ok := factID(p, "cms_command_id")
	if !ok {
		return invalidFact()
	}
	halTx, ok := factID(p, "hal_transaction_id")
	if !ok {
		return invalidFact()
	}
	ocppID, ok := factInt(p, "ocpp_transaction_id")
	if !ok || ocppID < 1 {
		return invalidFact()
	}
	meter, ok := factInt(p, "meter_start_wh")
	if !ok || meter < 0 {
		return invalidFact()
	}
	started, ok := factTime(p, "started_at")
	if !ok {
		return invalidFact()
	}
	var intent models.ChargingStartIntent
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", intentID).Error; err != nil {
		return invalidFact()
	}
	if intent.Status == constants.StartIntentStatusExpired || intent.Status == constants.StartIntentStatusRejected {
		return &APIError{http.StatusConflict, "unsafe_hal_fact_transition", "The start intent cannot accept this fact."}
	}
	var command models.HALCommandRecord
	if err := tx.First(&command, "cms_command_id = ? AND start_intent_id = ?", commandID, intent.ID).Error; err != nil {
		return invalidFact()
	}
	commandHALID, ok := factID(p, "hal_command_id")
	if !ok || (command.HALCommandID != nil && *command.HALCommandID != commandHALID) {
		return invalidFact()
	}
	identity, ok := p["charger_ocpp_identity"].(string)
	connectorNumber, connectorNumberOK := factInt(p, "ocpp_connector_number")
	if !ok || !connectorNumberOK {
		return invalidFact()
	}
	var charger models.Charger
	var connector models.Connector
	if tx.First(&charger, "id = ? AND cpo_id = ? AND ocpp_identity = ?", intent.ChargerID, intent.CPOID, identity).Error != nil ||
		tx.First(&connector, "id = ? AND charger_id = ? AND cpo_id = ? AND connector_number = ?", intent.ConnectorID, intent.ChargerID, intent.CPOID, connectorNumber).Error != nil {
		return invalidFact()
	}
	var session models.ChargingSession
	if err := tx.First(&session, "start_intent_id = ?", intent.ID).Error; err == nil {
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}
	session = models.ChargingSession{ID: uuid.New(), CPOID: intent.CPOID, StartIntentID: &intent.ID, TransactionID: ocppID, CustomerID: intent.CustomerID, ChargerID: intent.ChargerID, ConnectorID: intent.ConnectorID, TariffID: intent.TariffID, StartTime: started, MeterStartWh: meter, TotalKWh: decimal.Zero, TotalAmount: decimal.Zero, Currency: stringValue(intent.TariffSnapshot, "currency", "INR"), TariffSnapshot: intent.TariffSnapshot, TaxSnapshot: intent.TaxSnapshot, Status: constants.SessionStatusActive, CreatedAt: service.now(), UpdatedAt: service.now()}
	session.HALTransactionID = &halTx
	if err := tx.Create(&session).Error; err != nil {
		return err
	}
	return tx.Model(&models.ChargingStartIntent{}).Where("id = ?", intent.ID).Updates(map[string]any{"status": constants.StartIntentStatusActuallyStarted, "materialized_session_id": session.ID, "updated_at": service.now()}).Error
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
	var hold models.WalletHold
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hold, "start_intent_id = ?", session.StartIntentID).Error; err != nil {
		return err
	}
	amount := chargingAmount(session.TariffSnapshot, session.TaxSnapshot, meterStop-session.MeterStartWh)
	if amount.GreaterThan(hold.Amount) {
		return tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusReconciling, "updated_at": service.now()}).Error
	}
	var wallet models.Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "id = ?", hold.WalletID).Error; err != nil {
		return err
	}
	if wallet.Balance.LessThan(amount) {
		return tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusReconciling, "updated_at": service.now()}).Error
	}
	ledger := models.WalletTransaction{ID: uuid.New(), CPOID: session.CPOID, WalletID: wallet.ID, SessionID: &session.ID, Amount: amount, TransactionType: constants.WalletTransactionTypeDebit, Description: "Charging session settlement", IdempotencyKey: chargingStringPtr("charging-settlement-" + session.ID.String()), Status: constants.FinancialStatusCompleted, CreatedAt: service.now(), UpdatedAt: service.now()}
	if err := tx.Create(&ledger).Error; err != nil {
		return err
	}
	if err := tx.Create(&models.Payment{ID: uuid.New(), CPOID: session.CPOID, SessionID: session.ID, WalletTransactionID: ledger.ID, Amount: amount, PaymentMethod: "WALLET", Status: constants.FinancialStatusCompleted, CreatedAt: service.now(), UpdatedAt: service.now()}).Error; err != nil {
		return err
	}
	if err := tx.Model(&wallet).Updates(map[string]any{"balance": wallet.Balance.Sub(amount), "updated_at": service.now()}).Error; err != nil {
		return err
	}
	if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusCaptured, "captured_at": service.now(), "updated_at": service.now()}).Error; err != nil {
		return err
	}
	return tx.Model(&session).Updates(map[string]any{"meter_stop_wh": meterStop, "latest_meter_wh": meterStop, "meter_observed_at": stopped, "end_time": stopped, "total_kwh": decimal.NewFromInt(meterStop - session.MeterStartWh).Div(decimal.NewFromInt(1000)), "total_amount": amount, "status": constants.SessionStatusCompleted, "settlement_status": "SETTLED", "updated_at": service.now()}).Error
}

func chargingAmount(tax, gst models.JSONB, consumed int64) decimal.Decimal {
	price, _ := decimal.NewFromString(stringValue(tax, "price_per_kwh", "0"))
	rate, _ := decimal.NewFromString(stringValue(gst, "igst_rate", "0"))
	return price.Mul(decimal.NewFromInt(consumed)).Div(decimal.NewFromInt(1000)).Mul(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100)))).Round(2)
}
func factID(p models.JSONB, key string) (uuid.UUID, bool) {
	v, ok := p[key].(string)
	if !ok {
		return uuid.Nil, false
	}
	id, e := uuid.Parse(v)
	return id, e == nil
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
func factTime(p models.JSONB, key string) (time.Time, bool) {
	v, ok := p[key].(string)
	if !ok {
		return time.Time{}, false
	}
	t, e := time.Parse(time.RFC3339, v)
	return t, e == nil
}
func invalidFact() error {
	return &APIError{http.StatusUnprocessableEntity, "invalid_hal_fact", "The HAL fact cannot be safely applied."}
}
func stringValue(p models.JSONB, key, def string) string {
	v, ok := p[key].(string)
	if !ok {
		return def
	}
	return v
}
func chargingStringPtr(value string) *string { return &value }
