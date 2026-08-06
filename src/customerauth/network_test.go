package customerauth

import (
	"testing"
	"time"

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
	charger := models.Charger{
		ID: chargerID, HubID: &hubID, ChargerID: "abc123", Vendor: &vendor,
		Model: &model, MaxPowerKW: 7.4, OCPPVersion: "1.6J",
		Status: "AVAILABLE", Connectors: []models.Connector{{
			ID: uuid.New(), ConnectorNumber: 1, ConnectorType: "TYPE2",
			Status: "AVAILABLE",
		}},
	}
	projection := customerChargerView(charger, false)
	if projection.Availability != customerAvailabilityUnknown {
		t.Fatalf("charger availability=%q, want UNKNOWN", projection.Availability)
	}
	if projection.Connectors[0].Availability != customerAvailabilityUnknown {
		t.Fatalf("connector availability=%q, want UNKNOWN", projection.Connectors[0].Availability)
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
