package customerauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const customerChargeabilityMaxChargers = 100

const (
	chargeabilityAvailable                  = "AVAILABLE"
	chargeabilityNoChargeableConnector      = "NO_CHARGEABLE_CONNECTOR"
	chargeabilityCPOInactive                = "CPO_NOT_ACTIVE"
	chargeabilityCommercialBlocked          = "COMMERCIAL_ADMISSION_BLOCKED"
	chargeabilityHALUnavailable             = "HAL_UNAVAILABLE"
	chargeabilityChargerUnavailable         = "CHARGER_NOT_AVAILABLE"
	chargeabilityConnectorUnavailable       = "CONNECTOR_NOT_AVAILABLE"
	chargeabilityChargerOffline             = "CHARGER_OFFLINE"
	chargeabilityChargerStateUnknown        = "CHARGER_STATE_UNKNOWN"
	chargeabilityChargerStale               = "CHARGER_STALE"
	chargeabilityConnectorStateUnknown      = "CONNECTOR_STATE_UNKNOWN"
	chargeabilityConnectorStale             = "CONNECTOR_STALE"
	chargeabilityConnectorFaulted           = "CONNECTOR_FAULTED"
	chargeabilityStartInProgress            = "START_IN_PROGRESS"
	chargeabilityConnectorOccupied          = "CONNECTOR_OCCUPIED"
	chargeabilityMappingUnavailable         = "MAPPING_UNAVAILABLE"
	chargeabilityNoEligibleTariff           = "NO_ELIGIBLE_TARIFF"
	chargeabilityUnsupportedTariffPricing   = "UNSUPPORTED_TARIFF_PRICING"
	chargeabilityHubGSTUnavailable          = "HUB_GST_UNAVAILABLE"
	chargeabilityWalletMinimumBalanceNotMet = "WALLET_MINIMUM_BALANCE_NOT_MET"
	chargeabilityInsufficientWalletBalance  = "INSUFFICIENT_WALLET_BALANCE"
)

// CustomerChargeabilityResponse is the compact customer-facing projection for
// a bounded set of public charger IDs. It deliberately contains no inventory
// metadata: callers that need it use the existing charger projections.
type CustomerChargeabilityResponse struct {
	AsOf     time.Time                      `json:"as_of"`
	Chargers []CustomerChargerChargeability `json:"chargers"`
}

type CustomerChargerChargeability struct {
	ChargerID           string                           `json:"charger_id"`
	CanCharge           bool                             `json:"can_charge"`
	ChargeabilityReason string                           `json:"chargeability_reason"`
	Connectors          []CustomerConnectorChargeability `json:"connectors"`
}

type CustomerConnectorChargeability struct {
	ConnectorID         uuid.UUID `json:"connector_id"`
	ConnectorNumber     int       `json:"connector_number"`
	CanCharge           bool      `json:"can_charge"`
	ChargeabilityReason string    `json:"chargeability_reason"`
}

// normalizeCustomerChargerIDs accepts repeated and comma-separated public
// charger IDs. The output is deterministic and contains every ID once in its
// first-seen order; callers never supply CMS UUIDs to this surface.
func normalizeCustomerChargerIDs(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			id := normalizeCustomerChargerID(part)
			if !customerChargerIDPattern.MatchString(id) {
				return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_charger_id", Message: "Every charger ID must be a six-character public charger ID."}
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
			if len(result) > customerChargeabilityMaxChargers {
				return nil, &APIError{Status: http.StatusBadRequest, Code: "too_many_charger_ids", Message: "At most 100 charger IDs may be requested."}
			}
		}
	}
	if len(result) == 0 {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_charger_id", Message: "At least one public charger ID is required."}
	}
	return result, nil
}

// ListCustomerChargerChargeability reads only already committed CMS and
// HAL-derived projection state. It never synchronizes mappings, reserves
// wallet funds, sends commands, or changes charging/session state.
func (service *Service) ListCustomerChargerChargeability(ctx context.Context, principal Principal, publicIDs []string) (CustomerChargeabilityResponse, error) {
	ids, err := normalizeCustomerChargerIDs(publicIDs)
	if err != nil {
		return CustomerChargeabilityResponse{}, err
	}

	var records []models.Charger
	if err := service.database.WithContext(ctx).Model(&models.Charger{}).
		Joins("JOIN hubs ON hubs.id = chargers.hub_id AND hubs.cpo_id = chargers.cpo_id").
		Where("chargers.cpo_id = ? AND chargers.charger_id IN ? AND chargers.customer_visibility = ? AND hubs.customer_visible = ?", principal.CPOID, ids, true, true).
		Preload("Hub").
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("cpo_id = ?", principal.CPOID).Order("connector_number ASC")
		}).
		Find(&records).Error; err != nil {
		return CustomerChargeabilityResponse{}, fmt.Errorf("load customer chargeability chargers: %w", err)
	}
	byPublicID := make(map[string]models.Charger, len(records))
	for _, record := range records {
		// This re-check is defensive against any future preload/join change. A
		// non-visible record is omitted rather than becoming inventory evidence.
		if record.Hub != nil && record.Hub.CPOID == principal.CPOID && record.CustomerVisibility && record.Hub.CustomerVisible {
			byPublicID[record.ChargerID] = record
		}
	}
	views := make([]CustomerChargerView, 0, len(byPublicID))
	for _, id := range ids {
		if record, ok := byPublicID[id]; ok {
			views = append(views, customerChargerView(record, false))
		}
	}
	if err := service.enrichCustomerChargerChargeability(ctx, principal, views); err != nil {
		return CustomerChargeabilityResponse{}, err
	}
	return customerChargeabilityResponse(service.now(), views), nil
}

func customerChargeabilityResponse(asOf time.Time, views []CustomerChargerView) CustomerChargeabilityResponse {
	response := CustomerChargeabilityResponse{AsOf: asOf, Chargers: make([]CustomerChargerChargeability, 0, len(views))}
	for _, view := range views {
		charger := CustomerChargerChargeability{ChargerID: view.ChargerID, CanCharge: view.CanCharge, ChargeabilityReason: view.ChargeabilityReason, Connectors: make([]CustomerConnectorChargeability, 0, len(view.Connectors))}
		for _, connector := range view.Connectors {
			charger.Connectors = append(charger.Connectors, CustomerConnectorChargeability{ConnectorID: connector.ID, ConnectorNumber: connector.ConnectorNumber, CanCharge: connector.CanCharge, ChargeabilityReason: connector.ChargeabilityReason})
		}
		response.Chargers = append(response.Chargers, charger)
	}
	return response
}

// enrichCustomerChargerChargeability overlays committed live evidence and one
// shared, customer-specific admission evaluation across a bounded view set.
// It deliberately has no locks and no mutation side effects; StartCharging
// retains all transactional rechecks and its mapping synchronization command.
func (service *Service) enrichCustomerChargerChargeability(ctx context.Context, principal Principal, views []CustomerChargerView) error {
	if len(views) == 0 {
		return nil
	}
	chargerIDs := make([]uuid.UUID, 0, len(views))
	for _, view := range views {
		chargerIDs = append(chargerIDs, view.ID)
	}
	details := make(map[uuid.UUID]liveops.ChargerDetail, len(views))
	if service.live != nil {
		var err error
		details, err = service.live.GetChargerDetails(ctx, principal.CPOID, chargerIDs)
		if err != nil {
			return fmt.Errorf("load customer chargeability live state: %w", err)
		}
		for index := range views {
			if detail, ok := details[views[index].ID]; ok {
				applyCustomerChargerLiveDetail(&views[index], detail)
			}
		}
	}
	return service.evaluateCustomerChargerChargeability(ctx, principal, views, details)
}

func (service *Service) evaluateCustomerChargerChargeability(ctx context.Context, principal Principal, views []CustomerChargerView, details map[uuid.UUID]liveops.ChargerDetail) error {
	for index := range views {
		views[index].CanCharge = false
		views[index].ChargeabilityReason = chargeabilityNoChargeableConnector
		for connectorIndex := range views[index].Connectors {
			views[index].Connectors[connectorIndex].CanCharge = false
			views[index].Connectors[connectorIndex].ChargeabilityReason = chargeabilityConnectorUnavailable
		}
	}

	chargerIDs, connectorIDs, hubIDs := chargeabilityIDs(views)
	if len(chargerIDs) == 0 {
		return nil
	}

	var cpo models.CPO
	if err := service.database.WithContext(ctx).First(&cpo, "id = ?", principal.CPOID).Error; err != nil {
		return fmt.Errorf("load chargeability CPO: %w", err)
	}
	if cpo.Status != constants.CPOStatusActive {
		setAllChargeability(views, chargeabilityCPOInactive)
		return nil
	}
	if err := service.requireCustomerCommercialAdmission(ctx, principal.CPOID, service.now()); err != nil {
		setAllChargeability(views, chargeabilityCommercialBlocked)
		return nil
	}
	if service.hal == nil || !service.hal.Available() || service.live == nil {
		setAllChargeability(views, chargeabilityHALUnavailable)
		return nil
	}

	var mappings []models.HALChargerMapping
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND cms_charger_id IN ?", principal.CPOID, chargerIDs).Find(&mappings).Error; err != nil {
		return fmt.Errorf("load chargeability mappings: %w", err)
	}
	mappingByCharger := make(map[uuid.UUID]models.HALChargerMapping, len(mappings))
	for _, mapping := range mappings {
		mappingByCharger[mapping.CMSChargerID] = mapping
	}

	var intents []models.ChargingStartIntent
	if len(connectorIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND connector_id IN ? AND materialized_session_id IS NULL AND status IN ?", principal.CPOID, connectorIDs, chargingStartActiveStatuses).Find(&intents).Error; err != nil {
			return fmt.Errorf("load chargeability start intents: %w", err)
		}
	}
	intentByConnector := make(map[uuid.UUID]struct{}, len(intents))
	for _, intent := range intents {
		intentByConnector[intent.ConnectorID] = struct{}{}
	}
	var sessions []models.ChargingSession
	if len(connectorIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND connector_id IN ? AND status IN ?", principal.CPOID, connectorIDs, chargingSessionOccupancyStatuses).Find(&sessions).Error; err != nil {
			return fmt.Errorf("load chargeability sessions: %w", err)
		}
	}
	sessionByConnector := make(map[uuid.UUID]struct{}, len(sessions))
	for _, session := range sessions {
		sessionByConnector[session.ConnectorID] = struct{}{}
	}

	var wallet models.Wallet
	if err := service.database.WithContext(ctx).First(&wallet, "id = ? AND cpo_id = ? AND customer_id = ?", principal.Wallet.ID, principal.CPOID, principal.CustomerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			setAllChargeability(views, chargeabilityInsufficientWalletBalance)
			return nil
		}
		return fmt.Errorf("load chargeability wallet: %w", err)
	}
	settings := models.Settings{CPOID: principal.CPOID}
	if err := service.database.WithContext(ctx).First(&settings, "cpo_id = ?", principal.CPOID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load chargeability wallet policy: %w", err)
	}
	reservedBalance, err := outstandingWalletHolds(service.database.WithContext(ctx), wallet.ID)
	if err != nil {
		return fmt.Errorf("sum chargeability wallet holds: %w", err)
	}
	usableBalance, walletErr := usableWalletBalance(wallet.Balance.Sub(reservedBalance), settings)

	var tariffs []models.Tariff
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND is_active = ?", principal.CPOID, true).Find(&tariffs).Error; err != nil {
		return fmt.Errorf("load chargeability tariffs: %w", err)
	}
	var hubs []models.Hub
	if len(hubIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND id IN ?", principal.CPOID, hubIDs).Find(&hubs).Error; err != nil {
			return fmt.Errorf("load chargeability hubs: %w", err)
		}
	}
	hubByID := make(map[uuid.UUID]models.Hub, len(hubs))
	gstIDs := make([]uuid.UUID, 0, len(hubs))
	for _, hub := range hubs {
		hubByID[hub.ID] = hub
		if hub.GSTID != nil {
			gstIDs = append(gstIDs, *hub.GSTID)
		}
	}
	var gsts []models.GST
	if len(gstIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND is_active = ? AND id IN ?", principal.CPOID, true, uniqueUUIDs(gstIDs)).Find(&gsts).Error; err != nil {
			return fmt.Errorf("load chargeability GST profiles: %w", err)
		}
	}
	gstByID := make(map[uuid.UUID]models.GST, len(gsts))
	for _, gst := range gsts {
		gstByID[gst.ID] = gst
	}

	now := service.now()
	for viewIndex := range views {
		view := &views[viewIndex]
		if view.Status != constants.ChargerStatusActive {
			setChargerChargeability(view, chargeabilityChargerUnavailable)
			continue
		}
		mapping, mappingOK := mappingByCharger[view.ID]
		if !mappingOK || mapping.SyncState != "SYNCHRONIZED" {
			setChargerChargeability(view, chargeabilityMappingUnavailable)
			continue
		}
		detail, detailOK := details[view.ID]
		if reason := chargeabilityChargerLiveBlocker(detail, detailOK); reason != "" {
			setChargerChargeability(view, reason)
			continue
		}
		hub, hubOK := hubByID[view.HubID]
		if !hubOK || !hub.CustomerVisible {
			setChargerChargeability(view, chargeabilityChargerUnavailable)
			continue
		}
		tariff, tariffOK, tariffErr := resolveEffectiveTariffFromRecords(tariffs, principal.CPOID, principal.Customer.UserGroupID, view.ID, view.HubID, now)
		if tariffErr != nil || !tariffOK {
			setChargerChargeability(view, chargeabilityNoEligibleTariff)
			continue
		}
		pricing, pricingErr := tariffPricingFromTariff(tariff)
		if pricingErr != nil {
			setChargerChargeability(view, chargeabilityUnsupportedTariffPricing)
			continue
		}
		gst, gstOK := chargeabilityHubGST(hub, gstByID)
		if !gstOK {
			setChargerChargeability(view, chargeabilityHubGSTUnavailable)
			continue
		}
		if walletErr != nil {
			reason := chargeabilityInsufficientWalletBalance
			if errors.Is(walletErr, errWalletMinimumBalance) {
				reason = chargeabilityWalletMinimumBalanceNotMet
			}
			setChargerChargeability(view, reason)
			continue
		}

		liveByConnector := make(map[uuid.UUID]liveops.ConnectorState, len(detail.Connectors))
		for _, live := range detail.Connectors {
			liveByConnector[live.ConnectorID] = live
		}
		for connectorIndex := range view.Connectors {
			connector := &view.Connectors[connectorIndex]
			if connector.Status != constants.ChargerStatusActive {
				setConnectorChargeability(connector, chargeabilityConnectorUnavailable)
				continue
			}
			if _, found := intentByConnector[connector.ID]; found {
				setConnectorChargeability(connector, chargeabilityStartInProgress)
				continue
			}
			if _, found := sessionByConnector[connector.ID]; found {
				setConnectorChargeability(connector, chargeabilityConnectorOccupied)
				continue
			}
			live, found := liveByConnector[connector.ID]
			if !found || !chargingConnectorAllowsNewStart(live) {
				setConnectorChargeability(connector, chargeabilityLiveBlocker(live, found))
				continue
			}
			if _, err := deriveChargingLimit(usableBalance, pricing, gst, models.Connector{ID: connector.ID, ConnectorNumber: connector.ConnectorNumber, ConnectorTotalCapacity: connector.ConnectorTotalCapacity}, settings, chargingLimitSelection{Type: constants.ChargingLimitTypeAuto}); err != nil {
				setConnectorChargeability(connector, chargeabilityInsufficientWalletBalance)
				continue
			}
			setConnectorChargeability(connector, chargeabilityAvailable)
		}
		aggregateChargerChargeability(view)
	}
	return nil
}

func chargeabilityIDs(views []CustomerChargerView) ([]uuid.UUID, []uuid.UUID, []uuid.UUID) {
	chargers, connectors, hubs := make([]uuid.UUID, 0, len(views)), make([]uuid.UUID, 0), make([]uuid.UUID, 0, len(views))
	for _, view := range views {
		chargers = append(chargers, view.ID)
		if view.HubID != uuid.Nil {
			hubs = append(hubs, view.HubID)
		}
		for _, connector := range view.Connectors {
			connectors = append(connectors, connector.ID)
		}
	}
	return uniqueUUIDs(chargers), uniqueUUIDs(connectors), uniqueUUIDs(hubs)
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

func resolveEffectiveTariffFromRecords(tariffs []models.Tariff, cpoID uuid.UUID, userGroupID *uuid.UUID, chargerID, hubID uuid.UUID, effectiveAt time.Time) (models.Tariff, bool, error) {
	for _, target := range effectiveTariffTargets(userGroupID, &chargerID, &hubID) {
		matches := make([]models.Tariff, 0)
		projection := make([]commercial.TemporalTariff, 0)
		for _, tariff := range tariffs {
			if tariff.CPOID != cpoID || tariff.AssignedTo != target.assignment || !tariff.IsActive {
				continue
			}
			matchesTarget := (target.column == "user_group_id" && tariff.UserGroupID != nil && *tariff.UserGroupID == target.id) ||
				(target.column == "charger_id" && tariff.ChargerID != nil && *tariff.ChargerID == target.id) ||
				(target.column == "hub_id" && tariff.HubID != nil && *tariff.HubID == target.id)
			if !matchesTarget {
				continue
			}
			matches = append(matches, tariff)
			projection = append(projection, commercial.TemporalTariff{ID: tariff.ID, IsActive: tariff.IsActive, StartDate: tariff.StartDate, EndDate: tariff.EndDate})
		}
		selectedID, ok, err := commercial.ResolveEnabledTariff(projection, effectiveAt)
		if err != nil {
			return models.Tariff{}, false, err
		}
		if !ok {
			continue
		}
		for _, tariff := range matches {
			if tariff.ID == selectedID {
				return tariff, true, nil
			}
		}
		return models.Tariff{}, false, errors.New("selected chargeability tariff missing from loaded records")
	}
	return models.Tariff{}, false, nil
}

func chargeabilityHubGST(hub models.Hub, gstByID map[uuid.UUID]models.GST) (models.GST, bool) {
	if hub.GSTID == nil {
		return models.GST{}, false
	}
	gst, ok := gstByID[*hub.GSTID]
	if !ok || commercial.ValidateHubGST(hub.State, gst.State, gst.SGSTRate, gst.CGSTRate, gst.IGSTRate) != nil {
		return models.GST{}, false
	}
	return gst, true
}

func chargeabilityLiveBlocker(live liveops.ConnectorState, found bool) string {
	if !found {
		return chargeabilityConnectorStateUnknown
	}
	if live.ParentConnectionState == "OFFLINE" {
		return chargeabilityChargerOffline
	}
	if live.ParentConnectionState != "ONLINE" {
		return chargeabilityChargerStateUnknown
	}
	if live.Freshness == liveops.FreshnessStale {
		return chargeabilityConnectorStale
	}
	if live.Freshness != liveops.FreshnessFresh {
		return chargeabilityConnectorStateUnknown
	}
	if live.LastOCPPStatus != nil && *live.LastOCPPStatus == "Faulted" {
		return chargeabilityConnectorFaulted
	}
	return chargeabilityConnectorUnavailable
}

// chargeabilityChargerLiveBlocker classifies already-materialized live state
// without reinterpreting how liveops derived it. In particular, unknown and
// stale evidence must not be presented as an asserted physical disconnection.
func chargeabilityChargerLiveBlocker(detail liveops.ChargerDetail, found bool) string {
	if !found {
		return chargeabilityChargerStateUnknown
	}
	if detail.Charger.ConnectionState == "OFFLINE" {
		return chargeabilityChargerOffline
	}
	if detail.Charger.ConnectionState != "ONLINE" {
		return chargeabilityChargerStateUnknown
	}
	if detail.Charger.ConnectionFreshness == liveops.FreshnessStale {
		return chargeabilityChargerStale
	}
	if detail.Charger.ConnectionFreshness != liveops.FreshnessFresh {
		return chargeabilityChargerStateUnknown
	}
	return ""
}

func setAllChargeability(views []CustomerChargerView, reason string) {
	for index := range views {
		setChargerChargeability(&views[index], reason)
	}
}

func setChargerChargeability(view *CustomerChargerView, reason string) {
	view.CanCharge, view.ChargeabilityReason = false, reason
	for index := range view.Connectors {
		setConnectorChargeability(&view.Connectors[index], reason)
	}
}

func setConnectorChargeability(connector *CustomerConnectorView, reason string) {
	connector.CanCharge = reason == chargeabilityAvailable
	connector.ChargeabilityReason = reason
}

func aggregateChargerChargeability(view *CustomerChargerView) {
	for _, connector := range view.Connectors {
		if connector.CanCharge {
			view.CanCharge, view.ChargeabilityReason = true, chargeabilityAvailable
			return
		}
	}
	view.CanCharge, view.ChargeabilityReason = false, chargeabilityNoChargeableConnector
}
