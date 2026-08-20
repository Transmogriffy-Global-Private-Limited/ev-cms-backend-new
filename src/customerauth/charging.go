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
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
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

var errWalletMinimumBalance = errors.New("wallet balance is below the CPO minimum")

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
	PricePerUnit      *string `json:"price_per_unit,omitempty"`
	LegacyPricePerKWh *string `json:"legacy_price_per_kwh,omitempty"`
	TariffType        *string `json:"tariff_type,omitempty"`
	PriceType         *string `json:"price_type,omitempty"`
	Units             *string `json:"units,omitempty"`
	IdleFeePerMinute  *string `json:"idle_fee_per_minute,omitempty"`
	Currency          string  `json:"currency"`
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

var chargingStartActiveStatuses = []constants.StartIntentStatus{
	constants.StartIntentStatusRequested,
	constants.StartIntentStatusAcceptedForDelivery,
	constants.StartIntentStatusProtocolAcknowledged,
	constants.StartIntentStatusActuallyStarted,
	constants.StartIntentStatusReconciliation,
}

func connectorNotAvailableForCharging() *APIError {
	return &APIError{http.StatusConflict, "connector_not_available", "The connector is not currently available for charging."}
}

func chargerMappingUnavailable() *APIError {
	return &APIError{http.StatusServiceUnavailable, "charger_mapping_unavailable", "Charging is temporarily unavailable for this charger."}
}

func chargingConnectorAllowsNewStart(state liveops.ConnectorState) bool {
	return state.Availability == "AVAILABLE" && state.Freshness == liveops.FreshnessFresh
}

func activeChargingStartIntent(query *gorm.DB, connectorID uuid.UUID) (models.ChargingStartIntent, bool, error) {
	var intent models.ChargingStartIntent
	err := query.Where("connector_id = ? AND status IN ?", connectorID, chargingStartActiveStatuses).
		Order("created_at DESC").First(&intent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ChargingStartIntent{}, false, nil
	}
	if err != nil {
		return models.ChargingStartIntent{}, false, err
	}
	return intent, true, nil
}

func existingChargingStartResponse(intent models.ChargingStartIntent) ChargingStartResponse {
	return ChargingStartResponse{StartIntentID: intent.ID, Status: intent.Status, SessionID: intent.MaterializedSessionID}
}

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
	if request.ConnectorID == uuid.Nil {
		return ChargingStartResponse{}, &APIError{http.StatusNotFound, "connector_not_found", "The requested connector was not found."}
	}
	var requestedConnector models.Connector
	if err := service.database.WithContext(ctx).First(&requestedConnector, "id = ? AND cpo_id = ? AND charger_id = ?", request.ConnectorID, principal.CPOID, charger.ID).Error; err != nil {
		return ChargingStartResponse{}, &APIError{http.StatusNotFound, "connector_not_found", "The requested connector was not found."}
	}
	existing, found, err := activeChargingStartIntent(service.database.WithContext(ctx), requestedConnector.ID)
	if err != nil {
		return ChargingStartResponse{}, fmt.Errorf("load active charging start intent: %w", err)
	}
	if found {
		if existing.CustomerID == principal.CustomerID {
			return existingChargingStartResponse(existing), nil
		}
		return ChargingStartResponse{}, connectorNotAvailableForCharging()
	}
	liveConnector, err := service.live.GetConnector(ctx, principal.CPOID, requestedConnector.ID)
	if err != nil {
		return ChargingStartResponse{}, fmt.Errorf("load connector live state: %w", err)
	}
	if !chargingConnectorAllowsNewStart(liveConnector) {
		// A same-customer retry may have committed after the preflight read but
		// before the live snapshot. Preserve replay semantics in that narrow race.
		existing, found, err = activeChargingStartIntent(service.database.WithContext(ctx), requestedConnector.ID)
		if err != nil {
			return ChargingStartResponse{}, fmt.Errorf("recheck active charging start intent: %w", err)
		}
		if found && existing.CustomerID == principal.CustomerID {
			return existingChargingStartResponse(existing), nil
		}
		return ChargingStartResponse{}, connectorNotAvailableForCharging()
	}
	// Mapping is a known operational prerequisite, not a command-delivery
	// outcome. Perform it before creating a commercial reservation so an
	// unavailable HAL mapping cannot create a reconciler zombie.
	if err := service.hal.EnsureChargerMapping(ctx, charger.ID, correlationID); err != nil {
		return ChargingStartResponse{}, chargerMappingUnavailable()
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&connector, "id = ? AND cpo_id = ? AND charger_id = ?", request.ConnectorID, principal.CPOID, charger.ID).Error; err != nil {
			return &APIError{http.StatusNotFound, "connector_not_found", "The requested connector was not found."}
		}
		if connector.Status != constants.ChargerStatusActive {
			return connectorNotAvailableForCharging()
		}
		existing, found, err := activeChargingStartIntent(tx, connector.ID)
		if err != nil {
			return err
		}
		if found {
			if existing.CustomerID == principal.CustomerID {
				return &existingStartIntentError{intent: existing}
			}
			return connectorNotAvailableForCharging()
		}
		var connectors []models.Connector
		if err := tx.Where("cpo_id = ? AND charger_id = ?", principal.CPOID, charger.ID).Order("connector_number ASC").Find(&connectors).Error; err != nil {
			return err
		}
		var wallet models.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "id = ? AND cpo_id = ? AND customer_id = ?", principal.Wallet.ID, principal.CPOID, principal.CustomerID).Error; err != nil {
			return err
		}
		settings := models.Settings{CPOID: principal.CPOID}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&settings, "cpo_id = ?", principal.CPOID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load CPO wallet policy: %w", err)
		}
		tariff, tariffOK, err := resolveEffectiveTariff(tx, principal.CPOID, principal.Customer.UserGroupID, &charger.ID, charger.HubID, now)
		if err != nil {
			return startChargingTariffResolutionError(err)
		}
		if !tariffOK {
			return &APIError{http.StatusConflict, "no_eligible_tariff", "No tariff is available for this charger."}
		}
		pricing, err := tariffPricingFromTariff(tariff)
		if err != nil {
			return &APIError{http.StatusConflict, "unsupported_tariff_pricing", "The selected tariff does not have supported pricing semantics."}
		}
		gst, gstOK, err := resolveActiveHubGST(tx, principal.CPOID, *charger.HubID)
		if err != nil {
			return err
		}
		if !gstOK {
			return &APIError{http.StatusConflict, "hub_gst_unavailable", "An active GST profile is required on this charger's hub."}
		}
		usableBalance, err := usableWalletBalance(wallet.Balance, settings)
		if err != nil {
			if errors.Is(err, errWalletMinimumBalance) {
				return &APIError{http.StatusConflict, "wallet_minimum_balance_not_met", "The wallet balance is below this CPO's minimum required to start charging."}
			}
			return &APIError{http.StatusConflict, "insufficient_wallet_balance", "The wallet balance after this CPO's buffer is insufficient for charging."}
		}
		reserved, energyLimit, err := affordableChargingLimit(usableBalance, pricing, gst, connector)
		if err != nil {
			return &APIError{http.StatusConflict, "insufficient_wallet_balance", "The wallet balance is insufficient for charging."}
		}
		intent = models.ChargingStartIntent{ID: intentID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, ChargerID: charger.ID, ConnectorID: connector.ID, WalletID: wallet.ID, TariffID: tariff.ID, Status: constants.StartIntentStatusRequested, CredentialHash: hash, CredentialExpiresAt: now.Add(chargingCredentialLifetime), CommandExpiresAt: now.Add(chargingCommandLifetime), EnergyLimitWh: energyLimit, MaxDurationSeconds: chargingMaxDuration, TariffSnapshot: pricing.snapshot(tariff), TaxSnapshot: taxSnapshot(*charger.HubID, gst), CreatedAt: now, UpdatedAt: now}
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
	// Inventory can change after the preflight. Reconfirm the mapping after the
	// transaction and before RequestStart; unlike delivery uncertainty, this
	// known pre-delivery failure is synchronously terminalized and releases the
	// new hold rather than becoming a reconciliation-required intent.
	if err := service.hal.EnsureChargerMapping(ctx, mapping.CMSChargerID, correlationID); err != nil {
		if terminalizeErr := service.terminalizeUnattemptedStartCommand(ctx, commandID); terminalizeErr != nil {
			return ChargingStartResponse{}, fmt.Errorf("terminalize failed mapping prerequisite: %w", terminalizeErr)
		}
		return ChargingStartResponse{}, chargerMappingUnavailable()
	}
	command, err := service.hal.RequestStart(ctx, halops.StartRequest{CMSCommandID: commandID, CMSStartIntentID: intentID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: request.ConnectorID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: requestConnectorNumber(mapping, request.ConnectorID), Credential: credential, CredentialExpiresAt: intent.CredentialExpiresAt, CommandExpiresAt: intent.CommandExpiresAt, EnergyLimitWh: intent.EnergyLimitWh, MaxDurationSeconds: intent.MaxDurationSeconds}, correlationID)
	if err != nil {
		if recordErr := service.markHALCommandFailure(ctx, commandID, err); recordErr != nil {
			return ChargingStartResponse{}, fmt.Errorf("record uncertain HAL start delivery: %w", recordErr)
		}
		return ChargingStartResponse{StartIntentID: intentID, Status: constants.StartIntentStatusReconciliation}, nil
	}
	response := ChargingStartResponse{StartIntentID: intentID, Status: constants.StartIntentStatusAcceptedForDelivery}
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var persistedCommand models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&persistedCommand, "cms_command_id = ?", commandID).Error; err != nil {
			return err
		}
		var persistedIntent models.ChargingStartIntent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&persistedIntent, "id = ?", intentID).Error; err != nil {
			return err
		}
		// A charger-originated start fact can win the race while the synchronous
		// HAL command response is in flight. Only the fact materializer may set
		// ACTUALLY_STARTED; this response must never move it backwards.
		if persistedIntent.Status == constants.StartIntentStatusActuallyStarted || persistedIntent.MaterializedSessionID != nil || persistedCommand.State == "MATERIALIZED" {
			response = existingChargingStartResponse(persistedIntent)
			return nil
		}
		if persistedIntent.Status == constants.StartIntentStatusExpired || persistedIntent.Status == constants.StartIntentStatusRejected || persistedIntent.Status == constants.StartIntentStatusReconciliation {
			response = existingChargingStartResponse(persistedIntent)
			return nil
		}
		now := service.now()
		if err := tx.Model(&persistedCommand).Updates(map[string]any{"hal_command_id": command.HALCommandID, "state": command.State, "updated_at": now}).Error; err != nil {
			return err
		}
		status := constants.StartIntentStatusAcceptedForDelivery
		if command.State == "OCPP_ACCEPTED" {
			status = constants.StartIntentStatusProtocolAcknowledged
		}
		if err := tx.Model(&persistedIntent).Updates(map[string]any{"status": status, "hal_command_id": command.HALCommandID, "updated_at": now}).Error; err != nil {
			return err
		}
		response.Status = status
		return nil
	}); err != nil {
		return ChargingStartResponse{}, fmt.Errorf("record accepted HAL start command: %w", err)
	}
	return response, nil
}

// usableWalletBalance applies the CPO admission policy once, inside the
// wallet-locked start transaction. The buffer limits a new session's total
// hold; it is not a second minimum-balance threshold.
func usableWalletBalance(balance decimal.Decimal, settings models.Settings) (decimal.Decimal, error) {
	if settings.WalletMinBalance < 0 || settings.WalletBufferMinBalance < 0 {
		return decimal.Zero, errors.New("invalid CPO wallet policy")
	}
	minimum := decimal.NewFromInt(int64(settings.WalletMinBalance))
	if balance.LessThan(minimum) {
		return decimal.Zero, errWalletMinimumBalance
	}
	usable := balance.Sub(decimal.NewFromInt(int64(settings.WalletBufferMinBalance)))
	if usable.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("no positive usable wallet balance")
	}
	return usable, nil
}

func requestConnectorNumber(mapping halops.ChargerMapping, connectorID uuid.UUID) int {
	for _, connector := range mapping.Connectors {
		if connector.CMSConnectorID == connectorID {
			return connector.OCPPConnectorNumber
		}
	}
	return 0
}

func startChargingTariffResolutionError(err error) error {
	if isTariffTopologyError(err) {
		return &APIError{http.StatusConflict, "no_eligible_tariff", "No unambiguous tariff is available for this charger."}
	}
	return err
}

func (service *Service) effectiveChargingCommercial(tx *gorm.DB, principal Principal, hubID, chargerID uuid.UUID, at time.Time) (models.Tariff, models.GST, bool, error) {
	tariff, ok, err := resolveEffectiveTariff(tx, principal.CPOID, principal.Customer.UserGroupID, &chargerID, &hubID, at)
	if err != nil {
		return models.Tariff{}, models.GST{}, false, err
	}
	if !ok {
		return models.Tariff{}, models.GST{}, false, nil
	}
	gst, ok, err := resolveActiveHubGST(tx, principal.CPOID, hubID)
	if err != nil {
		return models.Tariff{}, models.GST{}, false, err
	}
	return tariff, gst, ok, nil
}

func (service *Service) effectiveChargingTariff(tx *gorm.DB, principal Principal, hubID, chargerID uuid.UUID, at time.Time) (models.Tariff, bool, error) {
	tariff, _, ok, err := service.effectiveChargingCommercial(tx, principal, hubID, chargerID, at)
	return tariff, ok, err
}

func affordableChargingLimit(balance decimal.Decimal, pricing tariffPricing, gst models.GST, connector models.Connector) (decimal.Decimal, int64, error) {
	if balance.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, 0, errors.New("no positive affordable charge")
	}
	energyLimit, err := physicalChargingEnergyLimit(connector)
	if err != nil {
		return decimal.Zero, 0, err
	}
	switch pricing.priceType {
	case constants.PriceTypeEnergy:
		if pricing.pricePerUnit.IsZero() {
			return decimal.Zero, energyLimit, nil
		}
		multiplier, err := gstMultiplier(gst)
		if err != nil {
			return decimal.Zero, 0, err
		}
		unitPricePerKWh := pricing.pricePerUnit.Mul(multiplier)
		affordableWh := balance.Div(unitPricePerKWh).Mul(decimal.NewFromInt(1000)).Floor().IntPart()
		if affordableWh < 1 {
			return decimal.Zero, 0, errors.New("no positive affordable energy")
		}
		if affordableWh < energyLimit {
			energyLimit = affordableWh
		}
		reserved := unitPricePerKWh.Mul(decimal.NewFromInt(energyLimit)).Div(decimal.NewFromInt(1000)).RoundCeil(2)
		if reserved.GreaterThan(balance) {
			energyLimit--
			reserved = unitPricePerKWh.Mul(decimal.NewFromInt(energyLimit)).Div(decimal.NewFromInt(1000)).RoundCeil(2)
		}
		if energyLimit < 1 {
			return decimal.Zero, 0, errors.New("no positive affordable energy")
		}
		return reserved, energyLimit, nil
	case constants.PriceTypeTime:
		reserved, err := pricing.amountWithGST(0, time.Time{}, time.Time{}.Add(time.Duration(chargingMaxDuration)*time.Second), gst)
		if err != nil || reserved.GreaterThan(balance) {
			return decimal.Zero, 0, errors.New("no affordable session duration")
		}
		return reserved, energyLimit, nil
	case constants.PriceTypeSession:
		reserved, err := pricing.amountWithGST(0, time.Time{}, time.Time{}, gst)
		if err != nil || reserved.GreaterThan(balance) {
			return decimal.Zero, 0, errors.New("no affordable session")
		}
		return reserved, energyLimit, nil
	default:
		return decimal.Zero, 0, errUnsupportedTariffSemantics
	}
}

// physicalChargingEnergyLimit preserves HAL's positive energy bound for every
// tariff mode. ConnectorTotalCapacity is registered kW; over the existing
// safety duration it yields a bounded Wh limit without pricing time as energy.
func physicalChargingEnergyLimit(connector models.Connector) (int64, error) {
	if connector.ConnectorTotalCapacity <= 0 {
		return 0, errors.New("free charging requires positive connector capacity")
	}
	capacity, err := decimal.NewFromString(strconv.FormatFloat(connector.ConnectorTotalCapacity, 'f', -1, 64))
	if err != nil || capacity.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("invalid connector capacity")
	}
	limit := capacity.Mul(decimal.NewFromInt(1000)).Mul(decimal.NewFromInt(chargingMaxDuration)).Div(decimal.NewFromInt(3600)).Floor().IntPart()
	if limit < 1 {
		return 0, errors.New("no positive physical energy limit")
	}
	return limit, nil
}

func taxSnapshot(hubID uuid.UUID, gst models.GST) models.JSONB {
	return models.JSONB{"gst_id": gst.ID.String(), "hub_id": hubID.String(), "igst_rate": gst.IGSTRate.String(), "cgst_rate": gst.CGSTRate.String(), "sgst_rate": gst.SGSTRate.String()}
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
func commandDeliveryFailureDiagnostic(cause error) (string, string) {
	var httpError *halclient.HTTPError
	if errors.As(cause, &httpError) {
		return "provider_http", fmt.Sprintf("HAL command delivery returned HTTP %d; exact reconciliation required", httpError.Status)
	}
	if errors.Is(cause, halclient.ErrUnavailable) {
		return "hal_unavailable", "HAL command delivery became unavailable; exact reconciliation required"
	}
	return "transport", "HAL command delivery transport outcome is unknown; exact reconciliation required"
}

// markHALCommandFailure is used only after RequestStart or RequestStop has
// been invoked. Its reconciliation state therefore represents genuine
// uncertainty, never a known mapping prerequisite failure.
func (service *Service) markHALCommandFailure(ctx context.Context, commandID uuid.UUID, cause error) error {
	category, detail := commandDeliveryFailureDiagnostic(cause)
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{
			"state":               "RECONCILIATION_REQUIRED",
			"last_error_category": category,
			"last_error_detail":   detail,
			"updated_at":          service.now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingStartIntent{}).Where("id = (SELECT start_intent_id FROM hal_command_records WHERE cms_command_id = ?)", commandID).Updates(map[string]any{"status": constants.StartIntentStatusReconciliation, "updated_at": service.now()}).Error
	})
}

// ReconcileConfirmedAbsentStartCommand is the business-side consequence of a
// typed exact HAL 404. It is registered with halops at composition time. A
// command or session fact that wins the intent row lock leaves all financial
// state untouched.
func (service *Service) ReconcileConfirmedAbsentStartCommand(ctx context.Context, commandID uuid.UUID) error {
	return service.terminalizeStartCommand(ctx, commandID, "RECONCILIATION_REQUIRED", constants.StartIntentStatusReconciliation, "CONFIRMED_ABSENT", "confirmed_absent", "HAL exact command lookup confirmed no durable command")
}

// terminalizeUnattemptedStartCommand handles the narrow post-transaction
// mapping race: RequestStart has not been called, so the command is known not
// to exist at HAL. The normal preflight prevents this path in ordinary cases.
func (service *Service) terminalizeUnattemptedStartCommand(ctx context.Context, commandID uuid.UUID) error {
	return service.terminalizeStartCommand(ctx, commandID, "PERSISTED", constants.StartIntentStatusRequested, "NOT_ATTEMPTED", "mapping_prerequisite_failed", "HAL mapping prerequisite failed before start command delivery")
}

func (service *Service) terminalizeStartCommand(ctx context.Context, commandID uuid.UUID, expectedCommandState string, expectedIntentStatus constants.StartIntentStatus, terminalCommandState, errorCategory, errorDetail string) error {
	now := service.now()
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ? AND kind = ?", commandID, "START").Error; err != nil {
			return err
		}
		if command.State != expectedCommandState || command.StartIntentID == nil {
			return nil
		}

		var intent models.ChargingStartIntent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&intent, "id = ?", *command.StartIntentID).Error; err != nil {
			return err
		}
		if intent.Status != expectedIntentStatus {
			return nil
		}
		if intent.MaterializedSessionID != nil {
			return nil
		}

		var sessionCount int64
		if err := tx.Model(&models.ChargingSession{}).Where("start_intent_id = ?", intent.ID).Count(&sessionCount).Error; err != nil {
			return err
		}
		if sessionCount != 0 {
			return nil
		}

		var hold models.WalletHold
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&hold, "start_intent_id = ?", intent.ID).Error; err != nil {
			return err
		}
		if hold.Status != constants.WalletHoldStatusHeld {
			if hold.Status == constants.WalletHoldStatusReleased {
				return nil
			}
			return fmt.Errorf("start command %s has non-releasable wallet hold state %s", commandID, hold.Status)
		}

		intentStatus := constants.StartIntentStatusRejected
		if !now.Before(command.CommandExpiresAt) {
			intentStatus = constants.StartIntentStatusExpired
		}
		if err := tx.Model(&hold).Updates(map[string]any{"status": constants.WalletHoldStatusReleased, "released_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&command).Updates(map[string]any{
			"state":               terminalCommandState,
			"last_error_category": errorCategory,
			"last_error_detail":   errorDetail,
			"updated_at":          now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&intent).Updates(map[string]any{"status": intentStatus, "updated_at": now}).Error
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
	command, err := service.hal.RequestStop(ctx, halops.StopRequest{CMSCommandID: commandID, CMSChargingSessionID: sessionID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: connector.ID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: connector.ConnectorNumber, HALTransactionID: *session.HALTransactionID, OCPPTransactionID: session.TransactionID, RequestedStopInitiator: "CUSTOMER", RequestedStopReason: strings.TrimSpace(request.Reason), CommandExpiresAt: expires}, correlation)
	if err != nil {
		return service.markHALCommandFailure(ctx, commandID, err)
	}
	if command.HALCommandID == uuid.Nil {
		return service.markHALCommandFailure(ctx, commandID, halclient.ErrInvalidCommandResponse)
	}
	if err := service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ? AND (hal_command_id IS NULL OR hal_command_id = ?)", commandID, uuid.Nil).Updates(map[string]any{"hal_command_id": command.HALCommandID, "state": command.State, "updated_at": service.now()}).Error; err != nil {
		return fmt.Errorf("store HAL stop command identity: %w", err)
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
	view := ChargingSessionPricingView{
		IdleFeePerMinute: snapshotDecimal(session.TariffSnapshot, "idle_fee_per_min"),
		Currency:         snapshotString(session.TariffSnapshot, "currency", session.Currency),
	}
	if price := snapshotDecimal(session.TariffSnapshot, "price_per_unit"); price != nil {
		view.PricePerUnit = price
		view.TariffType = snapshotStringPointer(session.TariffSnapshot, "tariff_type")
		view.PriceType = snapshotStringPointer(session.TariffSnapshot, "price_type")
		view.Units = snapshotStringPointer(session.TariffSnapshot, "units")
		return view
	}
	// The legacy property is exposed only for pre-correction snapshots, where
	// the stored amount was explicitly per kWh and cannot be relabelled safely.
	view.LegacyPricePerKWh = snapshotDecimal(session.TariffSnapshot, "price_per_kwh")
	return view
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

func snapshotStringPointer(snapshot models.JSONB, key string) *string {
	value, ok := snapshot[key].(string)
	if !ok || value == "" {
		return nil
	}
	return &value
}
