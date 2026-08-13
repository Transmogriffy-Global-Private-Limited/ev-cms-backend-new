package customerauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	chargingCredentialLifetime = 5 * time.Minute
	chargingCommandLifetime    = 6 * time.Minute
	chargingMaxDuration        = int64(60 * 60)
)

type ChargingStartRequest struct {
	ChargerID   string    `json:"charger_id"`
	ConnectorID uuid.UUID `json:"connector_id"`
}
type ChargingStopRequest struct {
	Reason string `json:"reason"`
}
type ChargingStartResponse struct {
	StartIntentID uuid.UUID                   `json:"start_intent_id"`
	Status        constants.StartIntentStatus `json:"status"`
	SessionID     *uuid.UUID                  `json:"session_id,omitempty"`
}
type ChargingSessionView struct {
	ID                   uuid.UUID                     `json:"id"`
	StartIntentID        uuid.UUID                     `json:"start_intent_id"`
	State                string                        `json:"state"`
	StartProgress        constants.StartIntentStatus   `json:"start_progress"`
	StopProgress         *string                       `json:"stop_progress,omitempty"`
	LatestMeterWh        *int64                        `json:"latest_meter_wh,omitempty"`
	ConsumedWh           *int64                        `json:"consumed_wh,omitempty"`
	MeterObservedAt      *time.Time                    `json:"meter_observed_at,omitempty"`
	MeterFreshness       string                        `json:"meter_freshness"`
	ConnectionState      string                        `json:"connection_state"`
	ConnectionObservedAt *time.Time                    `json:"connection_observed_at,omitempty"`
	ConnectorOCPPStatus  *string                       `json:"connector_ocpp_status,omitempty"`
	ConnectorObservedAt  *time.Time                    `json:"connector_observed_at,omitempty"`
	ConnectorFreshness   string                        `json:"connector_freshness"`
	CompletedAt          *time.Time                    `json:"completed_at,omitempty"`
	StartedAt            time.Time                     `json:"started_at"`
	MeterStartWh         int64                         `json:"meter_start_wh"`
	MeterStopWh          *int64                        `json:"meter_stop_wh,omitempty"`
	TotalKWh             *string                       `json:"total_kwh,omitempty"`
	TotalAmount          *string                       `json:"total_amount,omitempty"`
	Currency             string                        `json:"currency"`
	SettlementStatus     string                        `json:"settlement_status"`
	StopReason           *string                       `json:"stop_reason,omitempty"`
	Charger              ChargingSessionChargerView    `json:"charger"`
	Connector            ChargingSessionConnectorView  `json:"connector"`
	Pricing              ChargingSessionPricingView    `json:"pricing"`
	Tax                  ChargingSessionTaxView        `json:"tax"`
	Financial            *ChargingSessionFinancialView `json:"financial,omitempty"`
}

// ChargingSessionHistoryQuery uses the same descending timestamp/UUID cursor
// convention as the customer wallet and network collections.
type ChargingSessionHistoryQuery struct {
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

type ChargingSessionHistoryResponse struct {
	Sessions     []ChargingSessionHistoryView `json:"sessions"`
	NextBefore   *time.Time                   `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID                   `json:"next_before_id,omitempty"`
	HasMore      bool                         `json:"has_more"`
}

type ChargingSessionHistoryView struct {
	ID               uuid.UUID                    `json:"id"`
	State            string                       `json:"state"`
	StartedAt        time.Time                    `json:"started_at"`
	CompletedAt      *time.Time                   `json:"completed_at,omitempty"`
	ConsumedWh       *int64                       `json:"consumed_wh,omitempty"`
	TotalKWh         *string                      `json:"total_kwh,omitempty"`
	TotalAmount      *string                      `json:"total_amount,omitempty"`
	Currency         string                       `json:"currency"`
	SettlementStatus string                       `json:"settlement_status"`
	Charger          ChargingSessionChargerView   `json:"charger"`
	Connector        ChargingSessionConnectorView `json:"connector"`
}

type ChargingSessionChargerView struct {
	ID        uuid.UUID               `json:"id"`
	ChargerID string                  `json:"charger_id"`
	Name      string                  `json:"name"`
	Hub       *ChargingSessionHubView `json:"hub,omitempty"`
}

type ChargingSessionHubView struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}

type ChargingSessionConnectorView struct {
	ID     uuid.UUID `json:"id"`
	Number int       `json:"number"`
	Type   string    `json:"type"`
}

type ChargingSessionPricingView struct {
	PricePerKWh      *string `json:"price_per_kwh,omitempty"`
	IdleFeePerMinute *string `json:"idle_fee_per_minute,omitempty"`
	Currency         string  `json:"currency"`
}

type ChargingSessionTaxView struct {
	SGSTRate *string `json:"sgst_rate,omitempty"`
	CGSTRate *string `json:"cgst_rate,omitempty"`
	IGSTRate *string `json:"igst_rate,omitempty"`
}

type ChargingSessionFinancialView struct {
	WalletTransactionID uuid.UUID `json:"wallet_transaction_id"`
	PaymentID           uuid.UUID `json:"payment_id"`
	Amount              string    `json:"amount"`
	Currency            string    `json:"currency"`
	PaymentMethod       string    `json:"payment_method"`
	PaymentStatus       string    `json:"payment_status"`
}

const (
	chargingSessionHistoryDefaultLimit = 25
	chargingSessionHistoryMaxLimit     = 100
)

type existingStartIntentError struct{ intent models.ChargingStartIntent }

func (err *existingStartIntentError) Error() string { return "existing charging start intent" }

func (service *Service) StartCharging(ctx context.Context, principal Principal, request ChargingStartRequest, correlationID string) (ChargingStartResponse, error) {
	if service.hal == nil || !service.hal.Available() {
		return ChargingStartResponse{}, &APIError{http.StatusServiceUnavailable, "hal_unavailable", "Charging is temporarily unavailable."}
	}
	charger, err := service.loadPublishedCustomerCharger(ctx, principal, request.ChargerID)
	if err != nil {
		return ChargingStartResponse{}, err
	}
	if charger.HubID == nil {
		return ChargingStartResponse{}, &APIError{http.StatusConflict, "charger_not_available", "The charger is not available for a new session."}
	}
	if charger.Status != constants.ChargerStatusActive {
		return ChargingStartResponse{}, &APIError{http.StatusConflict, "charger_not_available", "The charger is not available for a new session."}
	}
	now := service.now()
	credential, hash, err := newChargingCredential()
	if err != nil {
		return ChargingStartResponse{}, fmt.Errorf("create charging credential: %w", err)
	}
	intentID, commandID := uuid.New(), uuid.New()
	var intent models.ChargingStartIntent
	var mapping halops.ChargerMapping
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpo models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cpo, "id = ?", principal.CPOID).Error; err != nil {
			return err
		}
		if cpo.Status != constants.CPOStatusActive {
			return &APIError{http.StatusForbidden, "cpo_not_active", "Charging is not available for this provider."}
		}
		var connector models.Connector
		if request.ConnectorID == uuid.Nil || tx.First(&connector, "id = ? AND cpo_id = ? AND charger_id = ?", request.ConnectorID, principal.CPOID, charger.ID).Error != nil {
			return &APIError{http.StatusNotFound, "connector_not_found", "The requested connector was not found."}
		}
		if connector.Status != constants.ChargerStatusActive {
			return &APIError{http.StatusConflict, "connector_not_available", "The connector is not available for a new session."}
		}
		var existing models.ChargingStartIntent
		if err := tx.Where("connector_id = ? AND status IN ?", connector.ID, []constants.StartIntentStatus{constants.StartIntentStatusRequested, constants.StartIntentStatusAcceptedForDelivery, constants.StartIntentStatusProtocolAcknowledged, constants.StartIntentStatusActuallyStarted, constants.StartIntentStatusReconciliation}).Order("created_at DESC").First(&existing).Error; err == nil {
			if existing.CustomerID == principal.CustomerID {
				return &existingStartIntentError{intent: existing}
			}
			return &APIError{http.StatusConflict, "connector_not_available", "The connector is not available for a new session."}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		var connectors []models.Connector
		if err := tx.Where("cpo_id = ? AND charger_id = ?", principal.CPOID, charger.ID).Order("connector_number ASC").Find(&connectors).Error; err != nil {
			return err
		}
		var wallet models.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "id = ? AND cpo_id = ? AND customer_id = ?", principal.Wallet.ID, principal.CPOID, principal.CustomerID).Error; err != nil {
			return err
		}
		tariff, ok := service.effectiveChargingTariff(tx, principal, *charger.HubID, charger.ID, now)
		if !ok {
			return &APIError{http.StatusConflict, "no_eligible_tariff", "No tariff is available for this charger."}
		}
		reserved, energyLimit, err := affordableChargingLimit(wallet.Balance, tariff)
		if err != nil {
			return &APIError{http.StatusConflict, "insufficient_wallet_balance", "The wallet balance is insufficient for charging."}
		}
		intent = models.ChargingStartIntent{ID: intentID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, ChargerID: charger.ID, ConnectorID: connector.ID, WalletID: wallet.ID, TariffID: tariff.ID, Status: constants.StartIntentStatusRequested, CredentialHash: hash, CredentialExpiresAt: now.Add(chargingCredentialLifetime), CommandExpiresAt: now.Add(chargingCommandLifetime), EnergyLimitWh: energyLimit, MaxDurationSeconds: chargingMaxDuration, TariffSnapshot: tariffSnapshot(tariff), TaxSnapshot: taxSnapshot(tariff), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&intent).Error; err != nil {
			return err
		}
		hold := models.WalletHold{ID: uuid.New(), CPOID: principal.CPOID, WalletID: wallet.ID, StartIntentID: intent.ID, Amount: reserved, Currency: tariff.Currency, Status: constants.WalletHoldStatusHeld, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&hold).Error; err != nil {
			return err
		}
		command := models.HALCommandRecord{CMSCommandID: commandID, CPOID: principal.CPOID, Kind: "START", StartIntentID: &intentID, State: "PERSISTED", CommandExpiresAt: intent.CommandExpiresAt, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&command).Error; err != nil {
			return err
		}
		mapping = halops.ChargerMapping{CPOID: principal.CPOID, CMSChargerID: charger.ID, ChargerOCPPIdentity: charger.OCPPIdentity, Enabled: true, Connectors: make([]halops.ConnectorMapping, 0, len(connectors))}
		for _, mappedConnector := range connectors {
			mapping.Connectors = append(mapping.Connectors, halops.ConnectorMapping{CMSConnectorID: mappedConnector.ID, OCPPConnectorNumber: mappedConnector.ConnectorNumber})
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cms_charger_id"}}, DoUpdates: clause.AssignmentColumns([]string{"cpo_id", "charger_ocpp_identity", "sync_state", "updated_at"})}).Create(&models.HALChargerMapping{CMSChargerID: charger.ID, CPOID: principal.CPOID, ChargerOCPPIdentity: charger.OCPPIdentity, SyncState: "PENDING", CreatedAt: now, UpdatedAt: now}).Error
	})
	if err != nil {
		var existing *existingStartIntentError
		if errors.As(err, &existing) {
			return ChargingStartResponse{StartIntentID: existing.intent.ID, Status: existing.intent.Status, SessionID: existing.intent.MaterializedSessionID}, nil
		}
		return ChargingStartResponse{}, err
	}
	if err := service.hal.EnsureChargerMapping(ctx, mapping.CMSChargerID, correlationID); err != nil {
		service.markHALCommandFailure(ctx, commandID, err)
		return ChargingStartResponse{StartIntentID: intentID, Status: constants.StartIntentStatusReconciliation}, nil
	}
	command, err := service.hal.RequestStart(ctx, halops.StartRequest{CMSCommandID: commandID, CMSStartIntentID: intentID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: request.ConnectorID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: requestConnectorNumber(mapping, request.ConnectorID), Credential: credential, CredentialExpiresAt: intent.CredentialExpiresAt, CommandExpiresAt: intent.CommandExpiresAt, EnergyLimitWh: intent.EnergyLimitWh, MaxDurationSeconds: intent.MaxDurationSeconds}, correlationID)
	if err != nil {
		service.markHALCommandFailure(ctx, commandID, err)
		return ChargingStartResponse{StartIntentID: intentID, Status: constants.StartIntentStatusReconciliation}, nil
	}
	_ = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{"hal_command_id": command.HALCommandID, "state": command.State, "updated_at": service.now()}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingStartIntent{}).Where("id = ?", intentID).Updates(map[string]any{"status": constants.StartIntentStatusAcceptedForDelivery, "hal_command_id": command.HALCommandID, "updated_at": service.now()}).Error
	})
	return ChargingStartResponse{StartIntentID: intentID, Status: constants.StartIntentStatusAcceptedForDelivery}, nil
}

func requestConnectorNumber(mapping halops.ChargerMapping, connectorID uuid.UUID) int {
	for _, connector := range mapping.Connectors {
		if connector.CMSConnectorID == connectorID {
			return connector.OCPPConnectorNumber
		}
	}
	return 0
}

func (service *Service) effectiveChargingTariff(tx *gorm.DB, principal Principal, hubID, chargerID uuid.UUID, at time.Time) (models.Tariff, bool) {
	var tariffs []models.Tariff
	if tx.Preload("GST").Where("cpo_id = ? AND hub_id = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date > ?) AND (charger_id IS NULL OR charger_id = ?)", principal.CPOID, hubID, true, at, at, chargerID).Find(&tariffs).Error != nil {
		return models.Tariff{}, false
	}
	return selectCustomerTariff(tariffs, &chargerID, principal.Customer.UserGroupID)
}

func affordableChargingLimit(balance decimal.Decimal, tariff models.Tariff) (decimal.Decimal, int64, error) {
	if balance.LessThanOrEqual(decimal.Zero) || tariff.PricePerKWh.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, 0, errors.New("no positive affordable energy")
	}
	rate := decimal.Zero
	if tariff.GST != nil {
		rate = tariff.GST.IGSTRate.Div(decimal.NewFromInt(100))
	}
	perWh := tariff.PricePerKWh.Div(decimal.NewFromInt(1000)).Mul(decimal.NewFromInt(1).Add(rate))
	limit := balance.Div(perWh).Floor().IntPart()
	if limit < 1 {
		return decimal.Zero, 0, errors.New("no positive affordable energy")
	}
	reserved := perWh.Mul(decimal.NewFromInt(limit)).RoundCeil(2)
	if reserved.GreaterThan(balance) {
		limit--
		reserved = perWh.Mul(decimal.NewFromInt(limit)).RoundCeil(2)
	}
	if limit < 1 {
		return decimal.Zero, 0, errors.New("no positive affordable energy")
	}
	return reserved, limit, nil
}

func tariffSnapshot(t models.Tariff) models.JSONB {
	return models.JSONB{"tariff_id": t.ID.String(), "currency": t.Currency, "price_per_kwh": t.PricePerKWh.String(), "idle_fee_per_min": t.IdleFeePerMin.String()}
}
func taxSnapshot(t models.Tariff) models.JSONB {
	if t.GST == nil {
		return models.JSONB{}
	}
	return models.JSONB{"gst_id": t.GST.ID.String(), "igst_rate": t.GST.IGSTRate.String(), "cgst_rate": t.GST.CGSTRate.String(), "sgst_rate": t.GST.SGSTRate.String()}
}
func newChargingCredential() (string, string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	value := "appv1_" + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(value))
	return value, hex.EncodeToString(sum[:]), nil
}
func (service *Service) markHALCommandFailure(ctx context.Context, commandID uuid.UUID, cause error) {
	_ = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		state := "RECONCILIATION_REQUIRED"
		_ = tx.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{"state": state, "last_error_category": "transport", "last_error_detail": "HAL delivery requires reconciliation", "updated_at": service.now()}).Error
		return tx.Model(&models.ChargingStartIntent{}).Where("id = (SELECT start_intent_id FROM hal_command_records WHERE cms_command_id = ?)", commandID).Updates(map[string]any{"status": constants.StartIntentStatusReconciliation, "updated_at": service.now()}).Error
	})
}

func (service *Service) StopCharging(ctx context.Context, principal Principal, sessionID uuid.UUID, request ChargingStopRequest, correlation string) error {
	if service.hal == nil || !service.hal.Available() {
		return &APIError{http.StatusServiceUnavailable, "hal_unavailable", "Charging is temporarily unavailable."}
	}
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).First(&session, "id = ? AND cpo_id = ? AND customer_id = ?", sessionID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return customerNetworkNotFound(err, "charging session")
	}
	if session.HALTransactionID == nil || session.Status != constants.SessionStatusActive {
		return &APIError{http.StatusConflict, "session_not_stoppable", "The charging session is not active."}
	}
	var connector models.Connector
	if err := service.database.WithContext(ctx).First(&connector, "id = ? AND cpo_id = ?", session.ConnectorID, principal.CPOID).Error; err != nil {
		return err
	}
	var charger models.Charger
	if err := service.database.WithContext(ctx).First(&charger, "id = ? AND cpo_id = ?", session.ChargerID, principal.CPOID).Error; err != nil {
		return err
	}
	commandID := uuid.New()
	expires := service.now().Add(chargingCommandLifetime)
	if err := service.database.WithContext(ctx).Create(&models.HALCommandRecord{CMSCommandID: commandID, CPOID: principal.CPOID, Kind: "STOP", ChargingSessionID: &sessionID, State: "PERSISTED", CommandExpiresAt: expires, CreatedAt: service.now(), UpdatedAt: service.now()}).Error; err != nil {
		return err
	}
	if err := service.database.WithContext(ctx).Model(&models.ChargingSession{}).Where("id = ?", sessionID).Updates(map[string]any{"status": constants.SessionStatusStopPending, "updated_at": service.now()}).Error; err != nil {
		return err
	}
	_, err := service.hal.RequestStop(ctx, halops.StopRequest{CMSCommandID: commandID, CMSChargingSessionID: sessionID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: connector.ID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: connector.ConnectorNumber, HALTransactionID: *session.HALTransactionID, OCPPTransactionID: session.TransactionID, RequestedStopInitiator: "CUSTOMER", RequestedStopReason: strings.TrimSpace(request.Reason), CommandExpiresAt: expires}, correlation)
	if err != nil {
		service.markHALCommandFailure(ctx, commandID, err)
	}
	return nil
}

func (service *Service) GetChargingStartIntent(ctx context.Context, principal Principal, intentID uuid.UUID) (ChargingStartResponse, error) {
	var intent models.ChargingStartIntent
	if err := service.database.WithContext(ctx).First(&intent, "id = ? AND cpo_id = ? AND customer_id = ?", intentID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return ChargingStartResponse{}, customerNetworkNotFound(err, "charging start intent")
	}
	return ChargingStartResponse{StartIntentID: intent.ID, Status: intent.Status, SessionID: intent.MaterializedSessionID}, nil
}

// ListCustomerChargingSessions returns only materialized, customer-owned CMS
// sessions. It deliberately does not include start intents or HAL records.
func (service *Service) ListCustomerChargingSessions(ctx context.Context, principal Principal, query ChargingSessionHistoryQuery) (ChargingSessionHistoryResponse, error) {
	if err := validateChargingSessionHistoryQuery(&query); err != nil {
		return ChargingSessionHistoryResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Preload("Charger.Hub").
		Preload("Connector").
		Where("charging_sessions.cpo_id = ? AND charging_sessions.customer_id = ?", principal.CPOID, principal.CustomerID).
		Order("charging_sessions.start_time DESC, charging_sessions.id DESC")
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(charging_sessions.start_time, charging_sessions.id) < (?, ?)", *query.Before, *query.BeforeID)
	}

	records := make([]models.ChargingSession, 0, query.Limit+1)
	if err := databaseQuery.Limit(query.Limit + 1).Find(&records).Error; err != nil {
		return ChargingSessionHistoryResponse{}, fmt.Errorf("list customer charging sessions: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	response := ChargingSessionHistoryResponse{
		Sessions: make([]ChargingSessionHistoryView, 0, len(records)),
		HasMore:  hasMore,
	}
	for _, record := range records {
		response.Sessions = append(response.Sessions, customerChargingSessionHistoryView(record))
	}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		response.NextBefore = &last.StartTime
		response.NextBeforeID = &last.ID
	}
	return response, nil
}

func validateChargingSessionHistoryQuery(query *ChargingSessionHistoryQuery) error {
	if query.Limit == 0 {
		query.Limit = chargingSessionHistoryDefaultLimit
	}
	if query.Limit < 1 || query.Limit > chargingSessionHistoryMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Both before and before_id are required together."}
	}
	return nil
}

func (service *Service) GetChargingSession(ctx context.Context, principal Principal, sessionID uuid.UUID) (ChargingSessionView, error) {
	if service.live == nil {
		return ChargingSessionView{}, fmt.Errorf("live operations capability is unavailable")
	}
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).
		Preload("Charger.Hub").
		Preload("Connector").
		Preload("Payment.WalletTransaction").
		First(&session, "id = ? AND cpo_id = ? AND customer_id = ?", sessionID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return ChargingSessionView{}, customerNetworkNotFound(err, "charging session")
	}
	var intent models.ChargingStartIntent
	if session.StartIntentID == nil || service.database.WithContext(ctx).First(&intent, "id = ? AND cpo_id = ? AND customer_id = ?", *session.StartIntentID, principal.CPOID, principal.CustomerID).Error != nil {
		return ChargingSessionView{}, fmt.Errorf("load charging start intent: %w", gorm.ErrRecordNotFound)
	}
	live, err := service.live.GetSession(ctx, principal.CPOID, session.ID)
	if err != nil {
		return ChargingSessionView{}, fmt.Errorf("load session live state: %w", err)
	}
	charger, err := service.live.GetCharger(ctx, principal.CPOID, session.ChargerID)
	if err != nil {
		return ChargingSessionView{}, fmt.Errorf("load charger live state: %w", err)
	}
	connector, err := service.live.GetConnector(ctx, principal.CPOID, session.ConnectorID)
	if err != nil {
		return ChargingSessionView{}, fmt.Errorf("load connector live state: %w", err)
	}
	view := customerChargingSessionDetailView(session, intent, live, charger, connector)
	if session.Status == constants.SessionStatusStopPending {
		value := "REQUESTED"
		view.StopProgress = &value
	}
	return view, nil
}

func customerChargingSessionHistoryView(session models.ChargingSession) ChargingSessionHistoryView {
	view := ChargingSessionHistoryView{
		ID:               session.ID,
		State:            string(session.Status),
		StartedAt:        session.StartTime,
		CompletedAt:      session.EndTime,
		ConsumedWh:       customerChargingSessionConsumedWh(session),
		Currency:         session.Currency,
		SettlementStatus: session.SettlementStatus,
		Charger:          customerChargingSessionChargerView(session.Charger),
		Connector:        customerChargingSessionConnectorView(session.Connector),
	}
	if session.Status == constants.SessionStatusCompleted {
		totalKWh := session.TotalKWh.StringFixed(3)
		totalAmount := session.TotalAmount.StringFixed(2)
		view.TotalKWh, view.TotalAmount = &totalKWh, &totalAmount
	}
	return view
}

func customerChargingSessionDetailView(session models.ChargingSession, intent models.ChargingStartIntent, liveState liveops.SessionState, chargerState liveops.ChargerState, connectorState liveops.ConnectorState) ChargingSessionView {
	view := ChargingSessionView{
		ID:                   session.ID,
		StartIntentID:        intent.ID,
		State:                liveState.State,
		StartProgress:        intent.Status,
		LatestMeterWh:        liveState.LatestMeterWh,
		ConsumedWh:           liveState.ConsumedWh,
		MeterObservedAt:      liveState.MeterObservedAt,
		MeterFreshness:       liveState.MeterFreshness,
		ConnectionState:      chargerState.ConnectionState,
		ConnectionObservedAt: chargerState.ConnectionObservedAt,
		ConnectorOCPPStatus:  connectorState.LastOCPPStatus,
		ConnectorObservedAt:  connectorState.ObservedAt,
		ConnectorFreshness:   connectorState.Freshness,
		CompletedAt:          liveState.CompletedAt,
		StartedAt:            session.StartTime,
		MeterStartWh:         session.MeterStartWh,
		MeterStopWh:          session.MeterStopWh,
		Currency:             session.Currency,
		SettlementStatus:     session.SettlementStatus,
		StopReason:           session.StopReason,
		Charger:              customerChargingSessionChargerView(session.Charger),
		Connector:            customerChargingSessionConnectorView(session.Connector),
		Pricing:              customerChargingSessionPricingView(session),
		Tax:                  customerChargingSessionTaxView(session),
		Financial:            customerChargingSessionFinancialView(session),
	}
	if session.Status == constants.SessionStatusCompleted {
		totalKWh := session.TotalKWh.StringFixed(3)
		totalAmount := session.TotalAmount.StringFixed(2)
		view.TotalKWh, view.TotalAmount = &totalKWh, &totalAmount
	}
	return view
}

func customerChargingSessionConsumedWh(session models.ChargingSession) *int64 {
	meter := session.LatestMeterWh
	if session.Status == constants.SessionStatusCompleted && session.MeterStopWh != nil {
		meter = session.MeterStopWh
	}
	if meter == nil || *meter < session.MeterStartWh {
		return nil
	}
	consumed := *meter - session.MeterStartWh
	return &consumed
}

func customerChargingSessionChargerView(charger models.Charger) ChargingSessionChargerView {
	view := ChargingSessionChargerView{ID: charger.ID, ChargerID: charger.ChargerID, Name: charger.ChargerName}
	if charger.Hub != nil {
		view.Hub = &ChargingSessionHubView{ID: charger.Hub.ID, Name: charger.Hub.Name, Address: charger.Hub.Address}
	}
	return view
}

func customerChargingSessionConnectorView(connector models.Connector) ChargingSessionConnectorView {
	return ChargingSessionConnectorView{ID: connector.ID, Number: connector.ConnectorNumber, Type: connector.ConnectorType}
}

func customerChargingSessionPricingView(session models.ChargingSession) ChargingSessionPricingView {
	return ChargingSessionPricingView{
		PricePerKWh:      snapshotDecimal(session.TariffSnapshot, "price_per_kwh"),
		IdleFeePerMinute: snapshotDecimal(session.TariffSnapshot, "idle_fee_per_min"),
		Currency:         snapshotString(session.TariffSnapshot, "currency", session.Currency),
	}
}

func customerChargingSessionTaxView(session models.ChargingSession) ChargingSessionTaxView {
	return ChargingSessionTaxView{
		SGSTRate: snapshotDecimal(session.TaxSnapshot, "sgst_rate"),
		CGSTRate: snapshotDecimal(session.TaxSnapshot, "cgst_rate"),
		IGSTRate: snapshotDecimal(session.TaxSnapshot, "igst_rate"),
	}
}

func customerChargingSessionFinancialView(session models.ChargingSession) *ChargingSessionFinancialView {
	payment := session.Payment
	if payment == nil || payment.CPOID != session.CPOID || payment.SessionID != session.ID || payment.WalletTransaction.CPOID != session.CPOID || payment.WalletTransaction.SessionID == nil || *payment.WalletTransaction.SessionID != session.ID || payment.WalletTransaction.TransactionType != constants.WalletTransactionTypeDebit {
		return nil
	}
	return &ChargingSessionFinancialView{
		WalletTransactionID: payment.WalletTransactionID,
		PaymentID:           payment.ID,
		Amount:              payment.Amount.StringFixed(2),
		Currency:            session.Currency,
		PaymentMethod:       payment.PaymentMethod,
		PaymentStatus:       string(payment.Status),
	}
}

func snapshotDecimal(snapshot models.JSONB, key string) *string {
	value, ok := snapshot[key].(string)
	if !ok || value == "" {
		return nil
	}
	if _, err := decimal.NewFromString(value); err != nil {
		return nil
	}
	return &value
}

func snapshotString(snapshot models.JSONB, key, fallback string) string {
	if value, ok := snapshot[key].(string); ok && value != "" {
		return value
	}
	return fallback
}
