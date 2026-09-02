package customerauth

import (
	"context"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// CustomerLiveChargingSessionListResponse is a complete, replaceable view of
// one customer's currently operational materialized sessions. AsOf is shared
// by every projected amount in a frame, so time-priced sessions are coherent.
type CustomerLiveChargingSessionListResponse struct {
	Sessions []ChargingSessionFinancialProjectionView `json:"sessions"`
	AsOf     time.Time                                `json:"as_of"`
}

// ListCustomerLiveChargingSessionsWithFinancialProjection builds the User App
// live collection through bounded CMS reads. It intentionally does not call
// HAL and does not use the single-session detail path, avoiding N+1 queries.
func (service *Service) ListCustomerLiveChargingSessionsWithFinancialProjection(ctx context.Context, principal Principal) (CustomerLiveChargingSessionListResponse, error) {
	if service.live == nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live operations capability is unavailable")
	}
	asOf := service.now()
	records := make([]models.ChargingSession, 0)
	if err := service.database.WithContext(ctx).
		Preload("Charger", "cpo_id = ?", principal.CPOID).
		Preload("Charger.Hub", "cpo_id = ?", principal.CPOID).
		Preload("Connector", "cpo_id = ?", principal.CPOID).
		Preload("Payment.WalletTransaction").
		Where("charging_sessions.cpo_id = ? AND charging_sessions.customer_id = ? AND charging_sessions.status IN ? AND charging_sessions.end_time IS NULL", principal.CPOID, principal.CustomerID, chargingSessionOccupancyStatuses).
		Order("charging_sessions.start_time DESC, charging_sessions.id DESC").
		Find(&records).Error; err != nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("list customer live charging sessions: %w", err)
	}
	if err := service.hydrateCustomerSessionChargers(ctx, principal.CPOID, records); err != nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("hydrate customer live-session chargers: %w", err)
	}
	response := CustomerLiveChargingSessionListResponse{Sessions: make([]ChargingSessionFinancialProjectionView, 0, len(records)), AsOf: asOf}
	if len(records) == 0 {
		return response, nil
	}

	sessionIDs := make([]uuid.UUID, 0, len(records))
	chargerIDs := make([]uuid.UUID, 0, len(records))
	intentIDs := make([]uuid.UUID, 0, len(records))
	for _, session := range records {
		sessionIDs = append(sessionIDs, session.ID)
		chargerIDs = append(chargerIDs, session.ChargerID)
		if session.StartIntentID == nil {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live charging session is missing its start intent")
		}
		intentIDs = append(intentIDs, *session.StartIntentID)
	}

	intents := make([]models.ChargingStartIntent, 0, len(intentIDs))
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ? AND customer_id = ? AND id IN ?", principal.CPOID, principal.CustomerID, intentIDs).
		Find(&intents).Error; err != nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("load customer live-session start intents: %w", err)
	}
	intentByID := make(map[uuid.UUID]models.ChargingStartIntent, len(intents))
	for _, intent := range intents {
		intentByID[intent.ID] = intent
	}

	liveBySessionID, err := service.live.GetSessions(ctx, principal.CPOID, sessionIDs)
	if err != nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("load customer live-session telemetry: %w", err)
	}
	chargerDetails, err := service.live.GetChargerDetails(ctx, principal.CPOID, chargerIDs)
	if err != nil {
		return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("load customer live-session charger state: %w", err)
	}

	for _, session := range records {
		intent, ok := intentByID[*session.StartIntentID]
		if !ok {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live-session start intent disappeared during read")
		}
		liveSession, ok := liveBySessionID[session.ID]
		if !ok {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live-session telemetry disappeared during read")
		}
		detail, ok := chargerDetails[session.ChargerID]
		if !ok {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live-session charger state disappeared during read")
		}
		connector, ok := customerLiveConnectorState(detail, session.ConnectorID)
		if !ok {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("live-session connector state disappeared during read")
		}

		view := customerChargingSessionDetailView(session, intent, liveSession, detail.Charger, connector)
		if session.Status == constants.SessionStatusStopPending || session.Status == constants.SessionStatusReconciliationRequired {
			progress := "REQUESTED"
			view.StopProgress = &progress
		}
		projected, err := customerChargingSessionProjectedAmount(view, session, asOf)
		if err != nil {
			return CustomerLiveChargingSessionListResponse{}, fmt.Errorf("project customer live-session amount: %w", err)
		}
		response.Sessions = append(response.Sessions, ChargingSessionFinancialProjectionView{ChargingSessionView: view, ProjectedAmount: &projected})
	}
	return response, nil
}

func customerLiveConnectorState(detail liveops.ChargerDetail, connectorID uuid.UUID) (liveops.ConnectorState, bool) {
	for _, connector := range detail.Connectors {
		if connector.ConnectorID == connectorID {
			return connector, true
		}
	}
	return liveops.ConnectorState{}, false
}

func customerChargingSessionProjectedAmount(view ChargingSessionView, source models.ChargingSession, asOf time.Time) (string, error) {
	if asOf.Before(source.StartTime) {
		asOf = source.StartTime
	}
	consumedWh := int64(0)
	if view.ConsumedWh != nil {
		consumedWh = *view.ConsumedWh
	} else if source.LatestMeterWh != nil && *source.LatestMeterWh >= source.MeterStartWh {
		consumedWh = *source.LatestMeterWh - source.MeterStartWh
	}
	amount, err := commercial.SessionAmountFromSnapshots(source.TariffSnapshot, source.TaxSnapshot, consumedWh, source.StartTime, asOf)
	if err != nil {
		return "", err
	}
	return amount.StringFixed(2), nil
}

// customerLiveProjectionResourceIDs produces the current owned operational
// resources used to narrow shared charger/connector wake-up events.
func customerLiveProjectionResourceIDs(snapshot CustomerLiveChargingSessionListResponse) ([]uuid.UUID, []uuid.UUID) {
	chargerIDs := make([]uuid.UUID, 0, len(snapshot.Sessions))
	connectorIDs := make([]uuid.UUID, 0, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		chargerIDs = append(chargerIDs, session.Charger.ID)
		connectorIDs = append(connectorIDs, session.Connector.ID)
	}
	return chargerIDs, connectorIDs
}
