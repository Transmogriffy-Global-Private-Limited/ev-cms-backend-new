// Package liveops provides CMS-projection reads for operational state. It does
// not make synchronous HAL calls, so REST availability does not depend on HAL.
package liveops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	FreshnessFresh   = "FRESH"
	FreshnessStale   = "STALE"
	FreshnessUnknown = "UNKNOWN"
)

type Service struct {
	database   *gorm.DB
	staleAfter time.Duration
	now        func() time.Time
}

func New(database *gorm.DB, cfg config.HAL) *Service {
	return &Service{database: database, staleAfter: cfg.MeterStaleAfter, now: func() time.Time { return time.Now().UTC() }}
}

type ChargerState struct {
	ChargerID            uuid.UUID  `json:"charger_id"`
	CPOID                uuid.UUID  `json:"cpo_id"`
	ConnectionState      string     `json:"connection_state"`
	ConnectionFreshness  string     `json:"connection_freshness"`
	ConnectionObservedAt *time.Time `json:"connection_observed_at,omitempty"`
	ConnectionSequence   *int64     `json:"connection_sequence,omitempty"`
	ConnectionGeneration *int64     `json:"connection_generation,omitempty"`
}

type ConnectorState struct {
	ConnectorID           uuid.UUID  `json:"connector_id"`
	ChargerID             uuid.UUID  `json:"charger_id"`
	CPOID                 uuid.UUID  `json:"cpo_id"`
	LastOCPPStatus        *string    `json:"last_ocpp_status,omitempty"`
	Availability          string     `json:"availability"`
	Freshness             string     `json:"freshness"`
	ObservedAt            *time.Time `json:"observed_at,omitempty"`
	StatusSequence        *int64     `json:"status_sequence,omitempty"`
	ParentConnectionState string     `json:"parent_connection_state"`
}

type ChargerDetail struct {
	Charger    ChargerState     `json:"charger"`
	Connectors []ConnectorState `json:"connectors"`
}

type SessionState struct {
	SessionID       uuid.UUID  `json:"session_id"`
	CPOID           uuid.UUID  `json:"cpo_id"`
	CustomerID      uuid.UUID  `json:"-"`
	State           string     `json:"state"`
	StartedAt       time.Time  `json:"started_at"`
	LatestMeterWh   *int64     `json:"latest_meter_wh,omitempty"`
	ConsumedWh      *int64     `json:"consumed_wh,omitempty"`
	MeterObservedAt *time.Time `json:"meter_observed_at,omitempty"`
	MeterFreshness  string     `json:"meter_freshness"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type FleetState struct {
	CPOID               uuid.UUID `json:"cpo_id"`
	TotalChargers       int64     `json:"total_chargers"`
	OnlineChargers      int64     `json:"online_chargers"`
	OfflineChargers     int64     `json:"offline_chargers"`
	UnknownChargers     int64     `json:"unknown_chargers"`
	AvailableConnectors int64     `json:"available_connectors"`
	ChargingConnectors  int64     `json:"charging_connectors"`
	FaultedConnectors   int64     `json:"faulted_connectors"`
	ActiveSessions      int64     `json:"active_sessions"`
}

func (service *Service) GetCharger(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerState, error) {
	var charger models.Charger
	if err := service.database.WithContext(ctx).First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
		return ChargerState{}, err
	}
	state := ChargerState{ChargerID: charger.ID, CPOID: charger.CPOID, ConnectionState: "UNKNOWN", ConnectionFreshness: FreshnessUnknown}
	var runtime models.HALChargerRuntime
	if err := service.database.WithContext(ctx).First(&runtime, "cms_charger_id = ? AND cpo_id = ?", charger.ID, cpoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return state, nil
		}
		return ChargerState{}, fmt.Errorf("load charger runtime: %w", err)
	}
	state.ConnectionState = runtime.ConnectionState
	state.ConnectionObservedAt = &runtime.ObservedAt
	sequence, generation := runtime.ConnectionSequence, runtime.ConnectionGeneration
	state.ConnectionSequence, state.ConnectionGeneration = &sequence, &generation
	state.ConnectionFreshness = service.freshness(runtime.ObservedAt)
	return state, nil
}

func (service *Service) GetConnector(ctx context.Context, cpoID, connectorID uuid.UUID) (ConnectorState, error) {
	var connector models.Connector
	if err := service.database.WithContext(ctx).First(&connector, "id = ? AND cpo_id = ?", connectorID, cpoID).Error; err != nil {
		return ConnectorState{}, err
	}
	charger, err := service.GetCharger(ctx, cpoID, connector.ChargerID)
	if err != nil {
		return ConnectorState{}, err
	}
	state := ConnectorState{ConnectorID: connector.ID, ChargerID: connector.ChargerID, CPOID: cpoID, Availability: "UNKNOWN", Freshness: FreshnessUnknown, ParentConnectionState: charger.ConnectionState}
	var runtime models.HALConnectorRuntime
	if err := service.database.WithContext(ctx).First(&runtime, "cms_connector_id = ? AND cpo_id = ?", connectorID, cpoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return state, nil
		}
		return ConnectorState{}, fmt.Errorf("load connector runtime: %w", err)
	}
	status, sequence := runtime.OCPPConnectorStatus, runtime.ConnectorStatusSequence
	state.LastOCPPStatus, state.ObservedAt, state.StatusSequence = &status, &runtime.ObservedAt, &sequence
	state.Freshness = service.freshness(runtime.ObservedAt)
	if charger.ConnectionState != "ONLINE" || charger.ConnectionFreshness != FreshnessFresh {
		state.Availability = "UNAVAILABLE"
		state.Freshness = FreshnessStale
		return state, nil
	}
	if state.Freshness != FreshnessFresh {
		return state, nil
	}
	switch status {
	case "Available":
		state.Availability = "AVAILABLE"
	case "Charging", "Preparing", "Finishing":
		state.Availability = "CHARGING"
	case "Faulted":
		state.Availability = "FAULTED"
	default:
		state.Availability = "UNAVAILABLE"
	}
	return state, nil
}

// GetChargerDetail reads one committed charger projection and all connector
// projections in a bounded query set. It never fans out to HAL or per-row SQL.
func (service *Service) GetChargerDetail(ctx context.Context, cpoID, chargerID uuid.UUID) (ChargerDetail, error) {
	var charger models.Charger
	if err := service.database.WithContext(ctx).Preload("Connectors", func(tx *gorm.DB) *gorm.DB { return tx.Order("connector_number ASC") }).First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
		return ChargerDetail{}, err
	}
	liveCharger, err := service.GetCharger(ctx, cpoID, chargerID)
	if err != nil {
		return ChargerDetail{}, err
	}
	connectorIDs := make([]uuid.UUID, 0, len(charger.Connectors))
	for _, connector := range charger.Connectors {
		connectorIDs = append(connectorIDs, connector.ID)
	}
	var runtimes []models.HALConnectorRuntime
	if len(connectorIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND cms_connector_id IN ?", cpoID, connectorIDs).Find(&runtimes).Error; err != nil {
			return ChargerDetail{}, fmt.Errorf("load connector runtimes: %w", err)
		}
	}
	runtimeByID := make(map[uuid.UUID]models.HALConnectorRuntime, len(runtimes))
	for _, runtime := range runtimes {
		runtimeByID[runtime.CMSConnectorID] = runtime
	}
	result := ChargerDetail{Charger: liveCharger, Connectors: make([]ConnectorState, 0, len(charger.Connectors))}
	for _, connector := range charger.Connectors {
		result.Connectors = append(result.Connectors, service.connectorState(connector, runtimeByID[connector.ID], liveCharger))
	}
	return result, nil
}

func (service *Service) GetSession(ctx context.Context, cpoID, sessionID uuid.UUID) (SessionState, error) {
	var session models.ChargingSession
	if err := service.database.WithContext(ctx).First(&session, "id = ? AND cpo_id = ?", sessionID, cpoID).Error; err != nil {
		return SessionState{}, err
	}
	state := SessionState{SessionID: session.ID, CPOID: session.CPOID, CustomerID: session.CustomerID, State: string(session.Status), StartedAt: session.StartTime, LatestMeterWh: session.LatestMeterWh, MeterObservedAt: session.MeterObservedAt, MeterFreshness: FreshnessUnknown, CompletedAt: session.EndTime}
	if session.LatestMeterWh != nil && *session.LatestMeterWh >= session.MeterStartWh {
		consumed := *session.LatestMeterWh - session.MeterStartWh
		state.ConsumedWh = &consumed
	}
	if session.MeterObservedAt != nil {
		state.MeterFreshness = service.freshness(*session.MeterObservedAt)
	}
	if session.Status == constants.SessionStatusActive || session.Status == constants.SessionStatusStopPending {
		charger, err := service.GetCharger(ctx, cpoID, session.ChargerID)
		if err != nil {
			return SessionState{}, err
		}
		if charger.ConnectionState != "ONLINE" || charger.ConnectionFreshness != FreshnessFresh {
			state.MeterFreshness = FreshnessStale
		}
	}
	return state, nil
}

func (service *Service) GetFleet(ctx context.Context, cpoID uuid.UUID) (FleetState, error) {
	state := FleetState{CPOID: cpoID}
	if err := service.database.WithContext(ctx).Model(&models.Charger{}).Where("cpo_id = ?", cpoID).Count(&state.TotalChargers).Error; err != nil {
		return FleetState{}, err
	}
	var runtimes []models.HALChargerRuntime
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).Find(&runtimes).Error; err != nil {
		return FleetState{}, err
	}
	seen := map[uuid.UUID]bool{}
	for _, runtime := range runtimes {
		seen[runtime.CMSChargerID] = true
		if service.freshness(runtime.ObservedAt) != FreshnessFresh {
			state.UnknownChargers++
			continue
		}
		switch runtime.ConnectionState {
		case "ONLINE":
			state.OnlineChargers++
		case "OFFLINE":
			state.OfflineChargers++
		default:
			state.UnknownChargers++
		}
	}
	state.UnknownChargers += state.TotalChargers - int64(len(seen))
	var connectors []models.Connector
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).Find(&connectors).Error; err != nil {
		return FleetState{}, err
	}
	connectorIDs := make([]uuid.UUID, 0, len(connectors))
	for _, connector := range connectors {
		connectorIDs = append(connectorIDs, connector.ID)
	}
	var connectorRuntimes []models.HALConnectorRuntime
	if len(connectorIDs) > 0 {
		if err := service.database.WithContext(ctx).Where("cpo_id = ? AND cms_connector_id IN ?", cpoID, connectorIDs).Find(&connectorRuntimes).Error; err != nil {
			return FleetState{}, err
		}
	}
	connectorRuntimeByID := make(map[uuid.UUID]models.HALConnectorRuntime, len(connectorRuntimes))
	for _, runtime := range connectorRuntimes {
		connectorRuntimeByID[runtime.CMSConnectorID] = runtime
	}
	chargerRuntimeByID := make(map[uuid.UUID]models.HALChargerRuntime, len(runtimes))
	for _, runtime := range runtimes {
		chargerRuntimeByID[runtime.CMSChargerID] = runtime
	}
	for _, connector := range connectors {
		charger := ChargerState{ChargerID: connector.ChargerID, CPOID: cpoID, ConnectionState: "UNKNOWN", ConnectionFreshness: FreshnessUnknown}
		if runtime, ok := chargerRuntimeByID[connector.ChargerID]; ok {
			observed, sequence, generation := runtime.ObservedAt, runtime.ConnectionSequence, runtime.ConnectionGeneration
			charger.ConnectionState, charger.ConnectionObservedAt, charger.ConnectionSequence, charger.ConnectionGeneration, charger.ConnectionFreshness = runtime.ConnectionState, &observed, &sequence, &generation, service.freshness(observed)
		}
		live := service.connectorState(connector, connectorRuntimeByID[connector.ID], charger)
		switch live.Availability {
		case "AVAILABLE":
			state.AvailableConnectors++
		case "CHARGING":
			state.ChargingConnectors++
		case "FAULTED":
			state.FaultedConnectors++
		}
	}
	if err := service.database.WithContext(ctx).Model(&models.ChargingSession{}).Where("cpo_id = ? AND status IN ?", cpoID, []constants.SessionStatus{constants.SessionStatusActive, constants.SessionStatusStopPending}).Count(&state.ActiveSessions).Error; err != nil {
		return FleetState{}, err
	}
	return state, nil
}

func (service *Service) connectorState(connector models.Connector, runtime models.HALConnectorRuntime, charger ChargerState) ConnectorState {
	state := ConnectorState{ConnectorID: connector.ID, ChargerID: connector.ChargerID, CPOID: connector.CPOID, Availability: "UNKNOWN", Freshness: FreshnessUnknown, ParentConnectionState: charger.ConnectionState}
	if runtime.CMSConnectorID == uuid.Nil {
		return state
	}
	status, sequence, observed := runtime.OCPPConnectorStatus, runtime.ConnectorStatusSequence, runtime.ObservedAt
	state.LastOCPPStatus, state.ObservedAt, state.StatusSequence, state.Freshness = &status, &observed, &sequence, service.freshness(observed)
	if charger.ConnectionState != "ONLINE" || charger.ConnectionFreshness != FreshnessFresh {
		state.Availability, state.Freshness = "UNAVAILABLE", FreshnessStale
		return state
	}
	if state.Freshness != FreshnessFresh {
		return state
	}
	switch status {
	case "Available":
		state.Availability = "AVAILABLE"
	case "Charging", "Preparing", "Finishing":
		state.Availability = "CHARGING"
	case "Faulted":
		state.Availability = "FAULTED"
	default:
		state.Availability = "UNAVAILABLE"
	}
	return state
}

func (service *Service) freshness(observed time.Time) string {
	if observed.IsZero() {
		return FreshnessUnknown
	}
	if service.staleAfter <= 0 || service.now().Sub(observed) > service.staleAfter {
		return FreshnessStale
	}
	return FreshnessFresh
}
