package customerauth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestCustomerHubListQueryValidation(t *testing.T) {
	t.Parallel()

	query := CustomerHubListQuery{}
	if err := validateCustomerHubListQuery(&query); err != nil {
		t.Fatalf("default query rejected: %v", err)
	}
	if query.Limit != customerNetworkDefaultLimit {
		t.Fatalf("default limit=%d, want %d", query.Limit, customerNetworkDefaultLimit)
	}

	invalid := CustomerHubListQuery{Limit: customerNetworkMaxLimit + 1}
	if err := validateCustomerHubListQuery(&invalid); err == nil {
		t.Fatal("overlarge query limit was accepted")
	}
	invalid = CustomerHubListQuery{BeforeID: func() *uuid.UUID { value := uuid.New(); return &value }()}
	if err := validateCustomerHubListQuery(&invalid); err == nil {
		t.Fatal("incomplete cursor was accepted")
	}
}

func TestCustomerChargerListQueryValidation(t *testing.T) {
	t.Parallel()

	query := CustomerChargerListQuery{}
	if err := validateCustomerChargerListQuery(&query); err != nil {
		t.Fatalf("default charger query rejected: %v", err)
	}
	if query.Limit != customerNetworkDefaultLimit {
		t.Fatalf("default charger limit=%d, want %d", query.Limit, customerNetworkDefaultLimit)
	}
	latitude, longitude := 22.5, 88.3
	query = CustomerChargerListQuery{Latitude: &latitude, Longitude: &longitude}
	if err := validateCustomerChargerListQuery(&query); err != nil {
		t.Fatalf("location charger query rejected: %v", err)
	}
	if query.RadiusKM == nil || *query.RadiusKM != customerChargerSearchRadiusKM {
		t.Fatalf("default radius=%v, want %v", query.RadiusKM, customerChargerSearchRadiusKM)
	}
	query = CustomerChargerListQuery{Latitude: &latitude}
	if err := validateCustomerChargerListQuery(&query); err == nil {
		t.Fatal("incomplete charger location was accepted")
	}
	query = CustomerChargerListQuery{MinPowerKW: func() *float64 { value := 20.0; return &value }(), MaxPowerKW: func() *float64 { value := 10.0; return &value }()}
	if err := validateCustomerChargerListQuery(&query); err == nil {
		t.Fatal("reversed charger power range was accepted")
	}
}

func TestCustomerChargerDistance(t *testing.T) {
	t.Parallel()

	if distance := haversineDistanceKM(22.5726, 88.3639, 22.5726, 88.3639); distance != 0 {
		t.Fatalf("same-point distance=%v, want 0", distance)
	}
	if distance := haversineDistanceKM(0, 0, 0, 1); distance < 111 || distance > 112 {
		t.Fatalf("one-degree distance=%v, want approximately 111 km", distance)
	}
}

func TestCustomerNetworkProjectionDoesNotClaimLiveAvailability(t *testing.T) {
	t.Parallel()

	hubID := uuid.New()
	chargerID := uuid.New()
	vendor := "Delta"
	model := "Wallbox"
	hub := &models.Hub{Name: "Salt Lake Hub", Address: "Sector V", Latitude: 22.5726, Longitude: 88.3639, Open24Hours: true}
	charger := models.Charger{
		ID: chargerID, HubID: &hubID, ChargerID: "abc123", Vendor: &vendor,
		Hub: hub, Model: &model, MaxPowerKW: 7.4, OCPPVersion: "1.6J",
		ChargerName: "Lakefront Fast Charger", ChargerType: "DC", Segment: "Public",
		SubSegment: "Highway", ChargerUseType: "Commercial", Parking: "Covered",
		TwentyFourSevenOpen: false,
		Status:              constants.ChargerStatusActive, Connectors: []models.Connector{{
			ID: uuid.New(), ConnectorNumber: 1, ConnectorType: "TYPE2",
			ConnectorTotalCapacity: 7.4, Status: constants.ChargerStatusActive,
		}},
	}
	projection := customerChargerView(charger, false)
	if projection.Availability != customerAvailabilityUnknown {
		t.Fatalf("charger availability=%q, want UNKNOWN", projection.Availability)
	}
	if projection.Connectors[0].Availability != customerAvailabilityUnknown {
		t.Fatalf("connector availability=%q, want UNKNOWN", projection.Connectors[0].Availability)
	}
	if projection.ChargerName != "Lakefront Fast Charger" || projection.ChargerType != "DC" || projection.Segment != "Public" || projection.SubSegment != "Highway" || projection.ChargerUseType != "Commercial" || projection.Parking != "Covered" {
		t.Fatalf("customer-safe charger metadata was not preserved: %#v", projection)
	}
	if projection.HubName != "Salt Lake Hub" || projection.HubAddress != "Sector V" || projection.HubLatitude == nil || *projection.HubLatitude != 22.5726 || projection.HubLongitude == nil || *projection.HubLongitude != 88.3639 || projection.HubOpen24Hours == nil || !*projection.HubOpen24Hours {
		t.Fatalf("customer-safe hub metadata was not preserved: %#v", projection)
	}
	if projection.TwentyFourSevenOpen {
		t.Fatalf("charger opening status=%t, want false", projection.TwentyFourSevenOpen)
	}
	if projection.Connectors[0].ConnectorTotalCapacity != 7.4 {
		t.Fatalf("connector capacity=%v, want 7.4", projection.Connectors[0].ConnectorTotalCapacity)
	}
	encoded, err := json.Marshal(struct {
		Hub     CustomerHubSummary  `json:"hub"`
		Charger CustomerChargerView `json:"charger"`
	}{Hub: customerHubSummary(*hub, false), Charger: projection})
	if err != nil {
		t.Fatalf("marshal customer network projection: %v", err)
	}
	var payload struct {
		Hub     map[string]json.RawMessage `json:"hub"`
		Charger map[string]json.RawMessage `json:"charger"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal customer network projection: %v", err)
	}
	for key, want := range map[string]bool{"open_24_hours": true} {
		value, ok := payload.Hub[key]
		if !ok {
			t.Fatalf("hub response omitted %q", key)
		}
		var got bool
		if err := json.Unmarshal(value, &got); err != nil || got != want {
			t.Fatalf("hub %s=%s, want %t", key, value, want)
		}
	}
	if _, exists := payload.Hub["twenty_four_seven_open_status"]; exists {
		t.Fatal("hub response still exposes the charger opening-status field")
	}
	for key, want := range map[string]bool{
		"twenty_four_seven_open_status": false,
		"hub_open_24_hours":             true,
	} {
		value, ok := payload.Charger[key]
		if !ok {
			t.Fatalf("charger response omitted %q", key)
		}
		var got bool
		if err := json.Unmarshal(value, &got); err != nil || got != want {
			t.Fatalf("charger %s=%s, want %t", key, value, want)
		}
	}
}

func TestCustomerChargerLocationProjectionContainsOnlyMapFields(t *testing.T) {
	t.Parallel()

	charger := models.Charger{
		ChargerName: "Lakefront Fast Charger",
		Hub:         &models.Hub{Latitude: 22.5726, Longitude: 88.3639},
	}
	encoded, err := json.Marshal(customerChargerLocationView(charger))
	if err != nil {
		t.Fatalf("marshal charger location: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal charger location: %v", err)
	}
	if len(payload) != 3 {
		t.Fatalf("location field count=%d, want 3: %s", len(payload), encoded)
	}
	for _, key := range []string{"charger_name", "latitude", "longitude"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("location response omitted %q: %s", key, encoded)
		}
	}
	if got := customerChargerLocationView(charger); got.ChargerName != charger.ChargerName || got.Latitude != charger.Hub.Latitude || got.Longitude != charger.Hub.Longitude {
		t.Fatalf("location projection=%#v, want charger name and hub coordinates", got)
	}
}

func TestApplyCustomerChargerLiveDetailPreservesStaticEligibility(t *testing.T) {
	t.Parallel()

	availableID, disabledID := uuid.New(), uuid.New()
	view := CustomerChargerView{
		Status: constants.ChargerStatusActive,
		Connectors: []CustomerConnectorView{
			{ID: availableID, Status: constants.ChargerStatusActive, Availability: customerAvailabilityUnknown, Freshness: customerAvailabilityUnknown},
			{ID: disabledID, Status: constants.ChargerStatusInactive, Availability: "UNAVAILABLE", Freshness: customerAvailabilityUnknown},
		},
	}
	applyCustomerChargerLiveDetail(&view, liveops.ChargerDetail{
		Charger: liveops.ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: liveops.FreshnessFresh},
		Connectors: []liveops.ConnectorState{
			{ConnectorID: availableID, Availability: "AVAILABLE", Freshness: liveops.FreshnessFresh},
			{ConnectorID: disabledID, Availability: "AVAILABLE", Freshness: liveops.FreshnessFresh},
		},
	})
	if view.Availability != "AVAILABLE" {
		t.Fatalf("charger availability=%q, want AVAILABLE", view.Availability)
	}
	if view.Connectors[0].Availability != "AVAILABLE" || view.Connectors[0].Freshness != liveops.FreshnessFresh {
		t.Fatalf("active connector=%#v, want available fresh", view.Connectors[0])
	}
	if view.Connectors[1].Availability != "UNAVAILABLE" {
		t.Fatalf("inactive connector=%#v, want unavailable despite HAL status", view.Connectors[1])
	}

	view.Status = constants.ChargerStatusSuspended
	applyCustomerChargerLiveDetail(&view, liveops.ChargerDetail{
		Charger: liveops.ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: liveops.FreshnessFresh},
	})
	if view.Availability != "UNAVAILABLE" {
		t.Fatalf("suspended charger availability=%q, want UNAVAILABLE", view.Availability)
	}
}

func TestApplyCustomerChargerLiveDetailTreatsOfflineEvidenceAsUnavailable(t *testing.T) {
	t.Parallel()

	connectorID := uuid.New()
	view := CustomerChargerView{Status: constants.ChargerStatusActive, Connectors: []CustomerConnectorView{{ID: connectorID, Status: constants.ChargerStatusActive, Availability: customerAvailabilityUnknown, Freshness: customerAvailabilityUnknown}}}
	applyCustomerChargerLiveDetail(&view, liveops.ChargerDetail{
		Charger:    liveops.ChargerState{ConnectionState: "OFFLINE", ConnectionFreshness: liveops.FreshnessFresh},
		Connectors: []liveops.ConnectorState{{ConnectorID: connectorID, Availability: "AVAILABLE", Freshness: liveops.FreshnessFresh}},
	})
	if view.Availability != "UNAVAILABLE" {
		t.Fatalf("offline charger availability=%q, want UNAVAILABLE", view.Availability)
	}
}

func TestCustomerFavoritesQueryValidation(t *testing.T) {
	t.Parallel()

	query := CustomerFavoritesQuery{}
	if err := validateCustomerFavoritesQuery(&query); err != nil {
		t.Fatalf("default favorites query rejected: %v", err)
	}
	if query.Limit != customerNetworkDefaultLimit {
		t.Fatalf("default favorite limit=%d, want %d", query.Limit, customerNetworkDefaultLimit)
	}
	query = CustomerFavoritesQuery{HubBefore: func() *time.Time { value := time.Now(); return &value }()}
	if err := validateCustomerFavoritesQuery(&query); err == nil {
		t.Fatal("incomplete hub favorite cursor was accepted")
	}
}
