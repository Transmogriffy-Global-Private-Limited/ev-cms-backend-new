package customerauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HALFactEnvelope struct {
	FactID                 uuid.UUID       `json:"fact_id"`
	FactType               string          `json:"fact_type"`
	SchemaVersion          int             `json:"schema_version"`
	OccurredAt             time.Time       `json:"occurred_at"`
	Producer               string          `json:"producer"`
	ImmutableContentSHA256 string          `json:"immutable_content_sha256"`
	Payload                json.RawMessage `json:"payload"`
}

func (service *Service) AcceptHALFact(ctx context.Context, bearer string, envelope HALFactEnvelope) error {
	if service.halFactBearer == "" || bearer == "" || bearer != service.halFactBearer {
		return &APIError{http.StatusUnauthorized, "hal_fact_authentication_required", "Service authentication is required."}
	}
	if envelope.FactID == uuid.Nil || envelope.SchemaVersion != 1 || envelope.Producer != "ocpp-hal-go-new" || strings.TrimSpace(envelope.FactType) == "" || len(envelope.Payload) == 0 || !validFactDigest(envelope.ImmutableContentSHA256) || envelope.OccurredAt.IsZero() {
		return &APIError{http.StatusBadRequest, "invalid_hal_fact", "The HAL fact envelope is invalid."}
	}
	var payload models.JSONB
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return &APIError{http.StatusBadRequest, "invalid_hal_fact", "The HAL fact payload is invalid."}
	}
	digest, err := canonicalFactDigest(envelope)
	if err != nil {
		return fmt.Errorf("canonicalize HAL fact: %w", err)
	}
	if digest != envelope.ImmutableContentSHA256 {
		return &APIError{http.StatusConflict, "fact_integrity_violation", "The HAL fact digest does not match its immutable content."}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior models.HALFactReceipt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "fact_id = ?", envelope.FactID).Error; err == nil {
			if prior.Digest == digest {
				return nil
			}
			return &APIError{http.StatusConflict, "fact_integrity_violation", "The HAL fact ID was reused with altered content."}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		if err := service.applyHALFact(tx, envelope, payload); err != nil {
			return err
		}
		return tx.Create(&models.HALFactReceipt{FactID: envelope.FactID, FactType: envelope.FactType, Digest: digest, OccurredAt: envelope.OccurredAt, Payload: payload, ProcessedAt: service.now()}).Error
	})
}

func canonicalFactDigest(envelope HALFactEnvelope) (string, error) {
	var payload any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return "", err
	}
	raw, err := json.Marshal(map[string]any{"fact_id": envelope.FactID.String(), "fact_type": envelope.FactType, "schema_version": envelope.SchemaVersion, "occurred_at": envelope.OccurredAt.UTC(), "producer": envelope.Producer, "payload": payload})
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
func validFactDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func (service *Service) applyHALFact(tx *gorm.DB, envelope HALFactEnvelope, payload models.JSONB) error {
	switch envelope.FactType {
	case "charger.connection.updated":
		return service.applyConnectionFact(tx, payload)
	case "connector.status.updated":
		return service.applyConnectorFact(tx, payload)
	case "command.updated":
		return service.applyCommandFact(tx, payload)
	case "transaction.started":
		return service.applyStartedFact(tx, payload)
	case "transaction.meter":
		return service.applyMeterFact(tx, payload)
	case "transaction.completed":
		return service.applyCompletedFact(tx, payload)
	default:
		return &APIError{http.StatusBadRequest, "unsupported_hal_fact", "The HAL fact type is unsupported."}
	}
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
