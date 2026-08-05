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

func TestCustomerNetworkProjectionDoesNotClaimLiveAvailability(t *testing.T) {
	t.Parallel()

	hubID := uuid.New()
	chargerID := uuid.New()
	charger := models.Charger{
		ID: chargerID, HubID: &hubID, ChargerID: "abc123", Vendor: "Delta",
		Model: "Wallbox", MaxPowerKW: 7.4, OCPPVersion: "1.6J",
		Status: "AVAILABLE", Connectors: []models.Connector{{
			ID: uuid.New(), ConnectorNumber: 1, ConnectorType: "TYPE2",
			MaxCurrent: 32, MaxVoltage: 230, Status: "AVAILABLE",
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
