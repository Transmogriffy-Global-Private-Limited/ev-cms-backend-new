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
	// This is the largest whole-minute deadline representable by time.Duration,
	// not a product session ceiling. Wallet affordability determines a default
	// time-billed session's duration.
	maxChargingDurationMinutes = int64((time.Duration(1<<63 - 1)) / time.Minute)
)

var errWalletMinimumBalance = errors.New("wallet balance is below the CPO minimum")

var (
	errChargingLimitInvalid      = errors.New("invalid charging limit")
	errChargingLimitUnsupported  = errors.New("charging limit is incompatible with the tariff")
	errChargingLimitUnaffordable = errors.New("charging limit cannot be covered by the usable wallet balance")
)

type ChargingStartRequest struct {
	ChargerID   string                `json:"charger_id"`
	ConnectorID uuid.UUID             `json:"connector_id"`
	Limit       *ChargingLimitRequest `json:"limit,omitempty"`
}

// ChargingLimitRequest is one customer-selected execution boundary. It does
// not alter the tariff: the tariff still determines how the completed session
// is billed. Exactly the one value belonging to Type must be supplied.
type ChargingLimitRequest struct {
	Type            constants.ChargingLimitType `json:"type"`
	EnergyKWh       *decimal.Decimal            `json:"energy_kwh,omitempty"`
	DurationMinutes *int64                      `json:"duration_minutes,omitempty"`
	Amount          *decimal.Decimal            `json:"amount,omitempty"`
}

type ChargingLimitView struct {
	Type                constants.ChargingLimitType   `json:"type"`
	RequestedValue      *string                       `json:"requested_value,omitempty"`
	RequestedUnit       *string                       `json:"requested_unit,omitempty"`
	EnergyLimitWh       int64                         `json:"energy_limit_wh"`
	EnergyLimitSource   constants.ChargingLimitSource `json:"energy_limit_source"`
	MaxDurationSeconds  int64                         `json:"max_duration_seconds"`
	DurationLimitSource constants.ChargingLimitSource `json:"duration_limit_source"`
}
type ChargingStopRequest struct {
	Reason string `json:"reason"`
}
type ChargingStartResponse struct {
	TraceID       uuid.UUID                   `json:"trace_id"`
	StartIntentID uuid.UUID                   `json:"start_intent_id"`
	Status        constants.StartIntentStatus `json:"status"`
	SessionID     *uuid.UUID                  `json:"session_id,omitempty"`
	Limit         ChargingLimitView           `json:"limit"`
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
	SoCPercent           *string                       `json:"soc_percent,omitempty"`
	SoCObservedAt        *time.Time                    `json:"soc_observed_at,omitempty"`
	SoCFreshness         string                        `json:"soc_freshness"`
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
	Limit                ChargingLimitView             `json:"limit"`
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
	ID                uuid.UUID                    `json:"id"`
	State             string                       `json:"state"`
	StartedAt         time.Time                    `json:"started_at"`
	CompletedAt       *time.Time                   `json:"completed_at,omitempty"`
	ConsumedWh        *int64                       `json:"consumed_wh,omitempty"`
	TotalKWh          *string                      `json:"total_kwh,omitempty"`
	TotalAmount       *string                      `json:"total_amount,omitempty"`
	Currency          string                       `json:"currency"`
	SettlementStatus  string                       `json:"settlement_status"`
	InitialSoCPercent *string                      `json:"initial_soc_percent,omitempty"`
	FinalSoCPercent   *string                      `json:"final_soc_percent,omitempty"`
	SoCObservedAt     *time.Time                   `json:"soc_observed_at,omitempty"`
	Charger           ChargingSessionChargerView   `json:"charger"`
	Connector         ChargingSessionConnectorView `json:"connector"`
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
	constants.StartIntentStatusReconciliation,
}

var chargingSessionOccupancyStatuses = []constants.SessionStatus{
	constants.SessionStatusActive,
	constants.SessionStatusStopPending,
	constants.SessionStatusReconciliationRequired,
}

func connectorNotAvailableForCharging() *APIError {
	return &APIError{http.StatusConflict, "connector_not_available", "The connector is not currently available for charging."}
}

func chargerMappingUnavailable() *APIError {
	return &APIError{http.StatusServiceUnavailable, "charger_mapping_unavailable", "Charging is temporarily unavailable for this charger."}
}

func chargingConnectorAllowsNewStart(state liveops.ConnectorState) bool {
	return state.AllowsCMSControlledStart()
}

func activeChargingStartIntent(query *gorm.DB, connectorID uuid.UUID) (models.ChargingStartIntent, bool, error) {
	var intent models.ChargingStartIntent
	err := query.Where("connector_id = ? AND materialized_session_id IS NULL AND status IN ?", connectorID, chargingStartActiveStatuses).
		Order("created_at DESC").First(&intent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ChargingStartIntent{}, false, nil
	}
	if err != nil {
		return models.ChargingStartIntent{}, false, err
	}
	return intent, true, nil
}

func activeChargingSession(query *gorm.DB, connectorID uuid.UUID) (models.ChargingSession, bool, error) {
	var session models.ChargingSession
	err := query.Where("connector_id = ? AND status IN ?", connectorID, chargingSessionOccupancyStatuses).
		Order("start_time DESC").First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ChargingSession{}, false, nil
	}
	if err != nil {
		return models.ChargingSession{}, false, err
	}
	return session, true, nil
}

func existingChargingStartResponse(intent models.ChargingStartIntent) ChargingStartResponse {
	return chargingStartResponse(intent)
}

func chargingStartResponse(intent models.ChargingStartIntent) ChargingStartResponse {
	response := ChargingStartResponse{StartIntentID: intent.ID, Status: intent.Status, SessionID: intent.MaterializedSessionID, Limit: chargingLimitView(intent)}
	if intent.TraceID != nil {
		response.TraceID = *intent.TraceID
	}
	return response
}

func chargingLimitView(intent models.ChargingStartIntent) ChargingLimitView {
	view := ChargingLimitView{Type: intent.LimitType, EnergyLimitWh: intent.EnergyLimitWh, EnergyLimitSource: compatibleEnergyLimitSource(intent), MaxDurationSeconds: intent.MaxDurationSeconds, DurationLimitSource: compatibleDurationLimitSource(intent)}
	if !view.Type.Valid() {
		view.Type = constants.ChargingLimitTypeAuto
	}
	if intent.RequestedLimitValue == nil {
		return view
	}
	value := intent.RequestedLimitValue.String()
	view.RequestedValue = &value
	switch view.Type {
	case constants.ChargingLimitTypeEnergy:
		unit := "kwh"
		view.RequestedUnit = &unit
	case constants.ChargingLimitTypeTime:
		unit := "minutes"
		view.RequestedUnit = &unit
	case constants.ChargingLimitTypeMoney:
		unit := "currency"
		view.RequestedUnit = &unit
	}
	return view
}

// Compatibility is intentionally narrow: the first charging-limit release
// used AUTO only for wallet-derived bounds and matching explicit dimensions
// only for customer bounds. New rows persist the source directly.
func compatibleEnergyLimitSource(intent models.ChargingStartIntent) constants.ChargingLimitSource {
	if intent.EnergyLimitWh == 0 {
		return constants.ChargingLimitSourceNone
	}
	if intent.EnergyLimitSource.Valid() && intent.EnergyLimitSource != constants.ChargingLimitSourceNone {
		return intent.EnergyLimitSource
	}
	switch intent.LimitType {
	case constants.ChargingLimitTypeEnergy:
		return constants.ChargingLimitSourceCustomerEnergy
	case constants.ChargingLimitTypeMoney:
		return constants.ChargingLimitSourceCustomerMoney
	case constants.ChargingLimitTypeAuto:
		return constants.ChargingLimitSourceWallet
	default:
		return constants.ChargingLimitSourceNone
	}
}

func compatibleDurationLimitSource(intent models.ChargingStartIntent) constants.ChargingLimitSource {
	if intent.MaxDurationSeconds == 0 {
		return constants.ChargingLimitSourceNone
	}
	if intent.DurationLimitSource.Valid() && intent.DurationLimitSource != constants.ChargingLimitSourceNone {
		return intent.DurationLimitSource
	}
	switch intent.LimitType {
	case constants.ChargingLimitTypeTime:
		return constants.ChargingLimitSourceCustomerTime
	case constants.ChargingLimitTypeMoney:
		return constants.ChargingLimitSourceCustomerMoney
	case constants.ChargingLimitTypeAuto:
		return constants.ChargingLimitSourceWallet
	default:
		return constants.ChargingLimitSourceNone
	}
}

func (service *Service) StartCharging(ctx context.Context, principal Principal, request ChargingStartRequest, correlationID string) (ChargingStartResponse, error) {
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
	if err := service.requireCustomerCommercialAdmission(ctx, principal.CPOID, service.now()); err != nil {
		return ChargingStartResponse{}, err
	}
	if service.hal == nil || !service.hal.Available() {
		return ChargingStartResponse{}, &APIError{http.StatusServiceUnavailable, "hal_unavailable", "Charging is temporarily unavailable."}
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
	traceID, intentID, commandID := uuid.New(), uuid.New(), uuid.New()
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
		if err := requireCustomerCommercialAdmission(tx, principal.CPOID, service.now()); err != nil {
			return err
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
		if _, occupied, err := activeChargingSession(tx, connector.ID); err != nil {
			return err
		} else if occupied {
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
		reservedBalance, err := outstandingWalletHolds(tx, wallet.ID)
		if err != nil {
			return fmt.Errorf("sum outstanding wallet holds: %w", err)
		}
		usableBalance, err := usableWalletBalance(wallet.Balance.Sub(reservedBalance), settings)
		if err != nil {
			if errors.Is(err, errWalletMinimumBalance) {
				return &APIError{http.StatusConflict, "wallet_minimum_balance_not_met", "The wallet balance is below this CPO's minimum required to start charging."}
			}
			return &APIError{http.StatusConflict, "insufficient_wallet_balance", "The wallet balance after this CPO's buffer is insufficient for charging."}
		}
		selection, err := parseChargingLimit(request.Limit)
		if err != nil {
			return &APIError{http.StatusBadRequest, "invalid_charging_limit", "Provide exactly one valid value for the selected charging limit."}
		}
		effectiveLimit, err := deriveChargingLimit(usableBalance, pricing, gst, connector, settings, selection)
		if err != nil {
			if errors.Is(err, errChargingLimitUnsupported) {
				return &APIError{http.StatusConflict, "unsupported_charging_limit", "This charging limit is not supported by the selected tariff."}
			}
			return &APIError{http.StatusConflict, "insufficient_wallet_balance", "The wallet balance is insufficient for charging."}
		}
		intent = models.ChargingStartIntent{ID: intentID, TraceID: &traceID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, ChargerID: charger.ID, ConnectorID: connector.ID, WalletID: wallet.ID, TariffID: tariff.ID, Status: constants.StartIntentStatusRequested, CredentialHash: hash, CredentialExpiresAt: now.Add(chargingCredentialLifetime), CommandExpiresAt: now.Add(chargingCommandLifetime), LimitType: effectiveLimit.Type, RequestedLimitValue: effectiveLimit.RequestedValue, EnergyLimitWh: effectiveLimit.EnergyLimitWh, EnergyLimitSource: effectiveLimit.EnergyLimitSource, MaxDurationSeconds: effectiveLimit.MaxDurationSeconds, DurationLimitSource: effectiveLimit.DurationLimitSource, TariffSnapshot: pricing.snapshot(tariff), TaxSnapshot: taxSnapshot(*charger.HubID, gst), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&intent).Error; err != nil {
			return err
		}
		// Trace rows are diagnostic evidence only. Their availability must not
		// decide whether an otherwise valid charging command is accepted.
		linkage := chargingTraceRoot{StartIntentID: &intentID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: connector.ConnectorNumber}
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "APP", "CMS", "REQUEST", "HTTP", "PRE_START", "Charging start request accepted", correlationID, models.JSONB{"start_intent_id": intentID.String(), "connector_id": request.ConnectorID.String()})
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "CMS", "CMS", "LIFECYCLE", "POSTGRES", "STARTING", "Charging admission and limits persisted", correlationID, models.JSONB{"limit_type": string(effectiveLimit.Type), "energy_limit_wh": effectiveLimit.EnergyLimitWh, "energy_limit_source": string(effectiveLimit.EnergyLimitSource), "max_duration_seconds": effectiveLimit.MaxDurationSeconds, "duration_limit_source": string(effectiveLimit.DurationLimitSource)})
		hold := models.WalletHold{ID: uuid.New(), CPOID: principal.CPOID, WalletID: wallet.ID, StartIntentID: intent.ID, Amount: effectiveLimit.HoldAmount, Currency: tariff.Currency, Status: constants.WalletHoldStatusHeld, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&hold).Error; err != nil {
			return err
		}
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "CMS", "CMS", "COMMERCIAL", "POSTGRES", "STARTING", "Wallet reservation held", correlationID, models.JSONB{"wallet_hold_id": hold.ID.String(), "amount": hold.Amount.String(), "currency": hold.Currency, "status": string(hold.Status)})
		command := models.HALCommandRecord{CMSCommandID: commandID, TraceID: &traceID, CPOID: principal.CPOID, Kind: "START", StartIntentID: &intentID, State: "PERSISTED", CommandExpiresAt: intent.CommandExpiresAt, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&command).Error; err != nil {
			return err
		}
		linkage.CommandID = &commandID
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "CMS", "CMS", "COMMAND", "POSTGRES", "STARTING", "Start command persisted", correlationID, models.JSONB{"cms_command_id": commandID.String(), "status": command.State})
		mapping = halops.ChargerMapping{CPOID: principal.CPOID, CMSChargerID: charger.ID, ChargerOCPPIdentity: charger.OCPPIdentity, ExpectedSerial: strings.TrimSpace(charger.SerialNumber), Enabled: true, Connectors: make([]halops.ConnectorMapping, 0, len(connectors))}
		for _, mappedConnector := range connectors {
			mapping.Connectors = append(mapping.Connectors, halops.ConnectorMapping{CMSConnectorID: mappedConnector.ID, OCPPConnectorNumber: mappedConnector.ConnectorNumber})
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cms_charger_id"}}, DoUpdates: clause.AssignmentColumns([]string{"cpo_id", "charger_ocpp_identity", "sync_state", "updated_at"})}).Create(&models.HALChargerMapping{CMSChargerID: charger.ID, CPOID: principal.CPOID, ChargerOCPPIdentity: charger.OCPPIdentity, SyncState: "PENDING", CreatedAt: now, UpdatedAt: now}).Error
	})
	if err != nil {
		var existing *existingStartIntentError
		if errors.As(err, &existing) {
			return existingChargingStartResponse(existing.intent), nil
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
	command, err := service.hal.RequestStart(ctx, halops.StartRequest{TraceID: traceID, CMSCommandID: commandID, CMSStartIntentID: intentID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: request.ConnectorID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: requestConnectorNumber(mapping, request.ConnectorID), Credential: credential, CredentialExpiresAt: intent.CredentialExpiresAt, CommandExpiresAt: intent.CommandExpiresAt, LimitType: string(intent.LimitType), EnergyLimitWh: intent.EnergyLimitWh, EnergyLimitSource: string(intent.EnergyLimitSource), MaxDurationSeconds: intent.MaxDurationSeconds, DurationLimitSource: string(intent.DurationLimitSource)}, correlationID)
	if err != nil {
		if recordErr := service.markHALStartCommandFailure(ctx, commandID, err); recordErr != nil {
			return ChargingStartResponse{}, fmt.Errorf("record uncertain HAL start delivery: %w", recordErr)
		}
		response := chargingStartResponse(intent)
		response.Status = constants.StartIntentStatusReconciliation
		return response, nil
	}
	response := chargingStartResponse(intent)
	response.Status = constants.StartIntentStatusAcceptedForDelivery
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
		if persistedIntent.TraceID != nil {
			_ = service.recordChargingTraceWithRoot(tx, *persistedIntent.TraceID, persistedIntent.CPOID, chargingTraceRoot{StartIntentID: &persistedIntent.ID, SessionID: persistedIntent.MaterializedSessionID, CommandID: &commandID}, "HAL", "CMS", "COMMAND", "HTTP", "STARTING", "HAL recorded start command outcome", correlationID, models.JSONB{"cms_command_id": commandID.String(), "hal_command_id": command.HALCommandID.String(), "state": command.State})
		}
		response.Status = status
		return nil
	}); err != nil {
		return ChargingStartResponse{}, fmt.Errorf("record accepted HAL start command: %w", err)
	}
	return response, nil
}

// outstandingWalletHolds is called while the wallet is row-locked. HELD and
// reconciliation holds are money already committed to an earlier start; only
// captured/released holds are no longer an admission reservation.
func outstandingWalletHolds(tx *gorm.DB, walletID uuid.UUID) (decimal.Decimal, error) {
	var reserved decimal.Decimal
	err := tx.Model(&models.WalletHold{}).Where("wallet_id = ? AND status IN ?", walletID, []constants.WalletHoldStatus{constants.WalletHoldStatusHeld, constants.WalletHoldStatusReconciling}).Select("COALESCE(SUM(amount), 0)").Scan(&reserved).Error
	return reserved, err
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

type chargingLimitSelection struct {
	Type           constants.ChargingLimitType
	RequestedValue *decimal.Decimal
}

type effectiveChargingLimit struct {
	Type                constants.ChargingLimitType
	RequestedValue      *decimal.Decimal
	EnergyLimitWh       int64
	EnergyLimitSource   constants.ChargingLimitSource
	MaxDurationSeconds  int64
	DurationLimitSource constants.ChargingLimitSource
	HoldAmount          decimal.Decimal
}

// parseChargingLimit keeps the public request deliberately small: a caller may
// choose one execution limit, while the tariff remains the sole billing rule.
func parseChargingLimit(request *ChargingLimitRequest) (chargingLimitSelection, error) {
	if request == nil {
		return chargingLimitSelection{Type: constants.ChargingLimitTypeAuto}, nil
	}
	if !request.Type.Valid() || request.Type == constants.ChargingLimitTypeAuto {
		return chargingLimitSelection{}, errChargingLimitInvalid
	}
	values := 0
	if request.EnergyKWh != nil {
		values++
	}
	if request.DurationMinutes != nil {
		values++
	}
	if request.Amount != nil {
		values++
	}
	if values != 1 {
		return chargingLimitSelection{}, errChargingLimitInvalid
	}

	switch request.Type {
	case constants.ChargingLimitTypeEnergy:
		if request.EnergyKWh == nil || request.EnergyKWh.LessThanOrEqual(decimal.Zero) || request.EnergyKWh.Exponent() < -3 {
			return chargingLimitSelection{}, errChargingLimitInvalid
		}
		wh := request.EnergyKWh.Mul(decimal.NewFromInt(1000))
		if !wh.Equal(decimal.NewFromInt(wh.IntPart())) {
			return chargingLimitSelection{}, errChargingLimitInvalid
		}
		value := *request.EnergyKWh
		return chargingLimitSelection{Type: request.Type, RequestedValue: &value}, nil
	case constants.ChargingLimitTypeTime:
		if request.DurationMinutes == nil || *request.DurationMinutes < 1 || *request.DurationMinutes > maxChargingDurationMinutes {
			return chargingLimitSelection{}, errChargingLimitInvalid
		}
		value := decimal.NewFromInt(*request.DurationMinutes)
		return chargingLimitSelection{Type: request.Type, RequestedValue: &value}, nil
	case constants.ChargingLimitTypeMoney:
		if request.Amount == nil || request.Amount.LessThanOrEqual(decimal.Zero) || request.Amount.Exponent() < -2 {
			return chargingLimitSelection{}, errChargingLimitInvalid
		}
		value := *request.Amount
		return chargingLimitSelection{Type: request.Type, RequestedValue: &value}, nil
	default:
		return chargingLimitSelection{}, errChargingLimitInvalid
	}
}

// deriveChargingLimit combines two independent constraint systems. Customer
// intent contributes a physical boundary in its own requested dimension;
// wallet safety contributes a tariff-billed-dimension boundary. No path
// estimates energy from time or time from energy.
func deriveChargingLimit(balance decimal.Decimal, pricing tariffPricing, gst models.GST, _ models.Connector, settings models.Settings, selection chargingLimitSelection) (effectiveChargingLimit, error) {
	if balance.LessThanOrEqual(decimal.Zero) {
		return effectiveChargingLimit{}, errChargingLimitUnaffordable
	}
	walletBound, err := pricing.AffordableBound(balance, gst)
	if err != nil {
		return effectiveChargingLimit{}, err
	}
	result := effectiveChargingLimit{Type: selection.Type, RequestedValue: selection.RequestedValue, EnergyLimitSource: constants.ChargingLimitSourceNone, DurationLimitSource: constants.ChargingLimitSourceNone}
	if walletBound.Kind == tariffAffordableEnergy {
		result.EnergyLimitWh, result.EnergyLimitSource = walletBound.EnergyLimitWh, constants.ChargingLimitSourceWallet
	}
	if walletBound.Kind == tariffAffordableTime {
		result.MaxDurationSeconds, result.DurationLimitSource = walletBound.DurationSeconds, constants.ChargingLimitSourceWallet
	}

	if selection.Type != constants.ChargingLimitTypeAuto && selection.RequestedValue == nil {
		return effectiveChargingLimit{}, errChargingLimitInvalid
	}
	switch selection.Type {
	case constants.ChargingLimitTypeAuto:
	case constants.ChargingLimitTypeEnergy:
		applyEnergyConstraint(&result, selection.RequestedValue.Mul(decimal.NewFromInt(1000)).IntPart(), constants.ChargingLimitSourceCustomerEnergy)
	case constants.ChargingLimitTypeTime:
		applyDurationConstraint(&result, selection.RequestedValue.IntPart()*60, constants.ChargingLimitSourceCustomerTime)
	case constants.ChargingLimitTypeMoney:
		if selection.RequestedValue.GreaterThan(balance) {
			return effectiveChargingLimit{}, errChargingLimitUnaffordable
		}
		customerBound, err := pricing.AffordableBound(*selection.RequestedValue, gst)
		if err != nil {
			return effectiveChargingLimit{}, err
		}
		switch customerBound.Kind {
		case tariffAffordableEnergy:
			applyEnergyConstraint(&result, customerBound.EnergyLimitWh, constants.ChargingLimitSourceCustomerMoney)
		case tariffAffordableTime:
			applyDurationConstraint(&result, customerBound.DurationSeconds, constants.ChargingLimitSourceCustomerMoney)
		case tariffAffordableFixed:
			// Fixed session price is discrete: customer money is an admission
			// ceiling, not a continuously enforceable physical threshold.
		case tariffAffordableNone:
			return effectiveChargingLimit{}, errChargingLimitUnsupported
		}
	default:
		return effectiveChargingLimit{}, errChargingLimitInvalid
	}

	// The hold protects the maximum tariff-billed cost that is known without
	// cross-dimensional prediction. It may shrink only when a customer boundary
	// is tighter in that exact same billed dimension.
	nominal := walletBound.Amount
	switch pricing.priceType {
	case constants.PriceTypeEnergy:
		if result.EnergyLimitSource != constants.ChargingLimitSourceWallet && result.EnergyLimitWh > 0 {
			nominal, err = pricing.amountWithGST(result.EnergyLimitWh, time.Time{}, time.Time{}, gst)
		}
	case constants.PriceTypeTime:
		if result.DurationLimitSource != constants.ChargingLimitSourceWallet && result.MaxDurationSeconds > 0 {
			nominal, err = pricing.amountWithGST(0, time.Time{}, time.Time{}.Add(time.Duration(result.MaxDurationSeconds)*time.Second), gst)
		}
	}
	if err != nil || nominal.GreaterThan(balance) {
		return effectiveChargingLimit{}, errChargingLimitUnaffordable
	}
	result.HoldAmount = chargingHoldAmount(nominal, pricing, settings)
	return result, nil
}

func applyEnergyConstraint(result *effectiveChargingLimit, value int64, source constants.ChargingLimitSource) {
	if value <= 0 || (result.EnergyLimitWh > 0 && result.EnergyLimitWh < value) {
		return
	}
	if result.EnergyLimitWh == 0 || value < result.EnergyLimitWh || result.EnergyLimitSource == constants.ChargingLimitSourceWallet {
		result.EnergyLimitWh, result.EnergyLimitSource = value, source
	}
}

func applyDurationConstraint(result *effectiveChargingLimit, value int64, source constants.ChargingLimitSource) {
	if value <= 0 || (result.MaxDurationSeconds > 0 && result.MaxDurationSeconds < value) {
		return
	}
	if result.MaxDurationSeconds == 0 || value < result.MaxDurationSeconds || result.DurationLimitSource == constants.ChargingLimitSourceWallet {
		result.MaxDurationSeconds, result.DurationLimitSource = value, source
	}
}

// chargingHoldAmount turns the established CPO wallet buffer into a real
// reservation for metered tariff modes. Meter/stop intervals can overshoot a
// limit; settlement remains conservatively blocked if that finite buffer is
// still exceeded.
func chargingHoldAmount(nominal decimal.Decimal, pricing tariffPricing, settings models.Settings) decimal.Decimal {
	if nominal.IsZero() || pricing.priceType == constants.PriceTypeSession {
		return nominal
	}
	return nominal.Add(decimal.NewFromInt(int64(settings.WalletBufferMinBalance)))
}

func budgetedEnergyLimit(budget decimal.Decimal, pricing tariffPricing, gst models.GST) (int64, decimal.Decimal, error) {
	bound, err := pricing.AffordableBound(budget, gst)
	if err != nil {
		return 0, decimal.Zero, err
	}
	if bound.Kind != tariffAffordableEnergy {
		return 0, decimal.Zero, errChargingLimitUnsupported
	}
	return bound.EnergyLimitWh, bound.Amount, nil
}

func budgetedDuration(budget decimal.Decimal, pricing tariffPricing, gst models.GST) (int64, decimal.Decimal, error) {
	bound, err := pricing.AffordableBound(budget, gst)
	if err != nil {
		return 0, decimal.Zero, err
	}
	if bound.Kind != tariffAffordableTime {
		return 0, decimal.Zero, errChargingLimitUnsupported
	}
	return bound.DurationSeconds, bound.Amount, nil
}

// affordableChargingLimit remains the shared affordability helper for tests
// and legacy callers. Production admission uses deriveChargingLimit so its
// persisted type and optional deadline travel to HAL in the same command.
func affordableChargingLimit(balance decimal.Decimal, pricing tariffPricing, gst models.GST, _ models.Connector) (decimal.Decimal, int64, error) {
	if balance.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, 0, errChargingLimitUnaffordable
	}
	bound, err := pricing.AffordableBound(balance, gst)
	if err != nil {
		return decimal.Zero, 0, err
	}
	if bound.Kind == tariffAffordableNone {
		return decimal.Zero, 0, errChargingLimitUnsupported
	}
	return bound.Amount, bound.EnergyLimitWh, nil
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

// markHALStartCommandFailure records uncertainty after an invoked start.
func (service *Service) markHALStartCommandFailure(ctx context.Context, commandID uuid.UUID, cause error) error {
	category, detail := commandDeliveryFailureDiagnostic(cause)
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ?", commandID).Error; err != nil {
			return err
		}
		if err := tx.Model(&command).Updates(map[string]any{
			"state":               "RECONCILIATION_REQUIRED",
			"last_error_category": category,
			"last_error_detail":   detail,
			"updated_at":          service.now(),
		}).Error; err != nil {
			return err
		}
		if command.TraceID != nil {
			_ = service.recordChargingTraceWithRoot(tx, *command.TraceID, command.CPOID, chargingTraceRoot{CommandID: &commandID}, "CMS", "CMS", "FAILURE", "POSTGRES", "STARTING", "HAL start command requires reconciliation", "", models.JSONB{"cms_command_id": commandID.String(), "error_class": category, "status": "RECONCILIATION_REQUIRED"})
		}
		return tx.Model(&models.ChargingStartIntent{}).Where("id = (SELECT start_intent_id FROM hal_command_records WHERE cms_command_id = ?)", commandID).Updates(map[string]any{"status": constants.StartIntentStatusReconciliation, "updated_at": service.now()}).Error
	})
}

// markHALStopCommandFailure never rewinds an ambiguous stop. The session
// remains STOP_PENDING until exact HAL evidence or transaction completion
// resolves it.
func (service *Service) markHALStopCommandFailure(ctx context.Context, commandID uuid.UUID, cause error) error {
	category, detail := commandDeliveryFailureDiagnostic(cause)
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ? AND kind = ?", commandID, "STOP").Error; err != nil {
			return err
		}
		if err := tx.Model(&command).Updates(map[string]any{"state": "RECONCILIATION_REQUIRED", "last_error_category": category, "last_error_detail": detail, "updated_at": service.now()}).Error; err != nil {
			return err
		}
		if command.TraceID != nil {
			_ = service.recordChargingTraceWithRoot(tx, *command.TraceID, command.CPOID, chargingTraceRoot{SessionID: command.ChargingSessionID, CommandID: &commandID}, "CMS", "CMS", "FAILURE", "POSTGRES", "STOPPING", "HAL stop command requires reconciliation", "", models.JSONB{"cms_command_id": commandID.String(), "error_class": category, "status": "RECONCILIATION_REQUIRED"})
		}
		return tx.Model(&models.ChargingSession{}).Where("id = (SELECT charging_session_id FROM hal_command_records WHERE cms_command_id = ?) AND end_time IS NULL", commandID).Updates(map[string]any{"status": constants.SessionStatusStopPending, "updated_at": service.now()}).Error
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
		if command.TraceID != nil {
			_ = service.recordChargingTraceWithRoot(tx, *command.TraceID, command.CPOID, chargingTraceRoot{StartIntentID: &intent.ID, CommandID: &commandID}, "CMS", "CMS", "COMMERCIAL", "POSTGRES", "STARTING", "Wallet reservation released", "", models.JSONB{"wallet_hold_id": hold.ID.String(), "amount": hold.Amount.String(), "currency": hold.Currency, "status": string(constants.WalletHoldStatusReleased), "reason": errorCategory})
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
	if session.EndTime != nil || session.Status == constants.SessionStatusCompleted {
		return nil
	}
	if session.Status == constants.SessionStatusStopPending || session.Status == constants.SessionStatusReconciliationRequired {
		return service.reconcileExistingStopCommand(ctx, sessionID)
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
	traceID := uuid.New()
	if session.TraceID != nil {
		traceID = *session.TraceID
	}
	expires := service.now().Add(chargingCommandLifetime)
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.HALCommandRecord{CMSCommandID: commandID, TraceID: &traceID, CPOID: principal.CPOID, Kind: "STOP", ChargingSessionID: &sessionID, State: "PERSISTED", CommandExpiresAt: expires, CreatedAt: service.now(), UpdatedAt: service.now()}).Error; err != nil {
			return err
		}
		linkage := chargingTraceRoot{StartIntentID: session.StartIntentID, SessionID: &sessionID, CommandID: &commandID}
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "APP", "CMS", "REQUEST", "HTTP", "STOPPING", "Charging stop request accepted", correlation, models.JSONB{"cms_command_id": commandID.String(), "connector_id": session.ConnectorID.String()})
		_ = service.recordChargingTraceWithRoot(tx, traceID, principal.CPOID, linkage, "CMS", "CMS", "COMMAND", "POSTGRES", "STOPPING", "Stop command persisted", correlation, models.JSONB{"cms_command_id": commandID.String(), "status": "PERSISTED"})
		update := tx.Model(&models.ChargingSession{}).Where("id = ? AND status = ?", sessionID, constants.SessionStatusActive).Updates(map[string]any{"status": constants.SessionStatusStopPending, "updated_at": service.now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return &APIError{http.StatusConflict, "session_not_stoppable", "The charging session is not active."}
		}
		session.Status = constants.SessionStatusStopPending
		if session.TraceID == nil {
			if err := tx.Model(&models.ChargingSession{}).Where("id = ?", sessionID).Update("trace_id", traceID).Error; err != nil {
				return err
			}
			session.TraceID = &traceID
		}
		return service.emitChargingSessionChanged(tx, session)
	}); err != nil {
		return err
	}
	command, err := service.hal.RequestStop(ctx, halops.StopRequest{TraceID: traceID, CMSCommandID: commandID, CMSChargingSessionID: sessionID, CPOID: principal.CPOID, CustomerID: principal.CustomerID, CMSChargerID: charger.ID, CMSConnectorID: connector.ID, ChargerOCPPIdentity: charger.OCPPIdentity, OCPPConnectorNumber: connector.ConnectorNumber, HALTransactionID: *session.HALTransactionID, OCPPTransactionID: session.TransactionID, RequestedStopInitiator: "CUSTOMER", RequestedStopReason: strings.TrimSpace(request.Reason), CommandExpiresAt: expires}, correlation)
	if err != nil {
		return service.markHALStopCommandFailure(ctx, commandID, err)
	}
	if command.HALCommandID == uuid.Nil {
		return service.markHALStopCommandFailure(ctx, commandID, halclient.ErrInvalidCommandResponse)
	}
	if err := service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ? AND (hal_command_id IS NULL OR hal_command_id = ?)", commandID, uuid.Nil).Updates(map[string]any{"hal_command_id": command.HALCommandID, "state": command.State, "updated_at": service.now()}).Error; err != nil {
		return fmt.Errorf("store HAL stop command identity: %w", err)
	}
	return service.ReconcileStopCommand(ctx, commandID, command)
}

func (service *Service) reconcileExistingStopCommand(ctx context.Context, sessionID uuid.UUID) error {
	var command models.HALCommandRecord
	if err := service.database.WithContext(ctx).Where("charging_session_id = ? AND kind = ?", sessionID, "STOP").Order("created_at DESC").First(&command).Error; err != nil {
		return err
	}
	result, err := service.hal.ReconcileCommand(ctx, command.CMSCommandID)
	if errors.Is(err, halops.ErrCommandNotFound) {
		return service.ReconcileConfirmedAbsentStopCommand(ctx, command.CMSCommandID)
	}
	if err != nil {
		return service.markHALStopCommandFailure(ctx, command.CMSCommandID, err)
	}
	return service.ReconcileStopCommand(ctx, command.CMSCommandID, result)
}

func (service *Service) GetChargingStartIntent(ctx context.Context, principal Principal, intentID uuid.UUID) (ChargingStartResponse, error) {
	var intent models.ChargingStartIntent
	if err := service.database.WithContext(ctx).First(&intent, "id = ? AND cpo_id = ? AND customer_id = ?", intentID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return ChargingStartResponse{}, customerNetworkNotFound(err, "charging start intent")
	}
	return chargingStartResponse(intent), nil
}

// ListCustomerChargingSessions returns only materialized, customer-owned CMS
// sessions. It deliberately does not include start intents or HAL records.
func (service *Service) ListCustomerChargingSessions(ctx context.Context, principal Principal, query ChargingSessionHistoryQuery) (ChargingSessionHistoryResponse, error) {
	if err := validateChargingSessionHistoryQuery(&query); err != nil {
		return ChargingSessionHistoryResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Preload("Charger", "cpo_id = ?", principal.CPOID).
		Preload("Charger.Hub", "cpo_id = ?", principal.CPOID).
		Preload("Connector", "cpo_id = ?", principal.CPOID).
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
	if err := service.hydrateCustomerSessionChargers(ctx, principal.CPOID, records); err != nil {
		return ChargingSessionHistoryResponse{}, fmt.Errorf("hydrate customer session chargers: %w", err)
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
		Preload("Charger", "cpo_id = ?", principal.CPOID).
		Preload("Charger.Hub", "cpo_id = ?", principal.CPOID).
		Preload("Connector", "cpo_id = ?", principal.CPOID).
		Preload("Payment.WalletTransaction").
		First(&session, "id = ? AND cpo_id = ? AND customer_id = ?", sessionID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return ChargingSessionView{}, customerNetworkNotFound(err, "charging session")
	}
	// hydrateCustomerSessionChargers receives a slice to batch history/live
	// reads. Copy the repaired one-row projection back for this detail read.
	sessions := []models.ChargingSession{session}
	if err := service.hydrateCustomerSessionChargers(ctx, principal.CPOID, sessions); err != nil {
		return ChargingSessionView{}, fmt.Errorf("hydrate customer session charger: %w", err)
	}
	session = sessions[0]
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
	if session.Status == constants.SessionStatusStopPending || session.Status == constants.SessionStatusReconciliationRequired {
		value := "REQUESTED"
		view.StopProgress = &value
	}
	return view, nil
}

func customerChargingSessionHistoryView(session models.ChargingSession) ChargingSessionHistoryView {
	view := ChargingSessionHistoryView{
		ID:                session.ID,
		State:             string(session.Status),
		StartedAt:         session.StartTime,
		CompletedAt:       session.EndTime,
		ConsumedWh:        customerChargingSessionConsumedWh(session),
		Currency:          session.Currency,
		SettlementStatus:  session.SettlementStatus,
		InitialSoCPercent: decimalPointerString(session.InitialSoCPercent),
		FinalSoCPercent:   decimalPointerString(session.LatestSoCPercent),
		SoCObservedAt:     session.SoCObservedAt,
		Charger:           customerChargingSessionChargerView(session.Charger),
		Connector:         customerChargingSessionConnectorView(session.Connector),
	}
	if session.EndTime != nil {
		totalKWh := session.TotalKWh.StringFixed(3)
		totalAmount := session.TotalAmount.StringFixed(2)
		view.TotalKWh, view.TotalAmount = &totalKWh, &totalAmount
	}
	return view
}

// hydrateCustomerSessionChargers restores an absent direct preloaded charger
// relation with one CPO-scoped batch lookup. Connector.ChargerID is the first
// fallback because it is the persisted physical relationship; session.ChargerID
// remains the second materialized-start key. No serializer manufactures data.
func (service *Service) hydrateCustomerSessionChargers(ctx context.Context, cpoID uuid.UUID, sessions []models.ChargingSession) error {
	chargerIDs := make([]uuid.UUID, 0, len(sessions)*2)
	for _, session := range sessions {
		if session.Charger.ID != uuid.Nil {
			continue
		}
		chargerIDs = append(chargerIDs, session.Connector.ChargerID, session.ChargerID)
	}
	chargerIDs = uniqueCustomerSessionChargerIDs(chargerIDs)
	if len(chargerIDs) == 0 {
		return nil
	}

	var chargers []models.Charger
	if err := service.database.WithContext(ctx).
		Preload("Hub", "cpo_id = ?", cpoID).
		Where("cpo_id = ? AND id IN ?", cpoID, chargerIDs).
		Find(&chargers).Error; err != nil {
		return err
	}
	chargersByID := make(map[uuid.UUID]models.Charger, len(chargers))
	for _, charger := range chargers {
		chargersByID[charger.ID] = charger
	}
	for index := range sessions {
		assignCustomerSessionChargerFallback(cpoID, &sessions[index], chargersByID)
	}
	return nil
}

func assignCustomerSessionChargerFallback(cpoID uuid.UUID, session *models.ChargingSession, chargersByID map[uuid.UUID]models.Charger) {
	if session.Charger.ID != uuid.Nil || session.CPOID != cpoID || session.Connector.CPOID != cpoID {
		return
	}
	for _, candidateID := range []uuid.UUID{session.Connector.ChargerID, session.ChargerID} {
		candidate, ok := chargersByID[candidateID]
		if !ok || candidate.CPOID != cpoID {
			continue
		}
		session.Charger = candidate
		session.ChargerID = candidate.ID
		return
	}
}

func uniqueCustomerSessionChargerIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
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
		SoCPercent:           decimalPointerString(liveState.LatestSoCPercent),
		SoCObservedAt:        liveState.SoCObservedAt,
		SoCFreshness:         liveState.SoCFreshness,
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
		Limit:                chargingLimitView(intent),
		Tax:                  customerChargingSessionTaxView(session),
		Financial:            customerChargingSessionFinancialView(session),
	}
	if session.EndTime != nil {
		totalKWh := session.TotalKWh.StringFixed(3)
		totalAmount := session.TotalAmount.StringFixed(2)
		view.TotalKWh, view.TotalAmount = &totalKWh, &totalAmount
	}
	return view
}

func decimalPointerString(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
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
