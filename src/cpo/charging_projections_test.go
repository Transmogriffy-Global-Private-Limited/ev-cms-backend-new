package cpo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCPOTransactionProjectionUsesJoinedHumanAndProtocolIdentity(t *testing.T) {
	t.Parallel()

	hubID, sessionID, halTransactionID := uuid.New(), uuid.New(), uuid.New()
	connectorNumber := 2
	transaction := ChargerTransaction{
		ChargingSession: models.ChargingSession{
			ID:               sessionID,
			HALTransactionID: &halTransactionID,
			TransactionID:    654321,
			Status:           constants.SessionStatusReconciliationRequired,
			SettlementStatus: "RECONCILIATION_REQUIRED",
			TotalKWh:         decimal.RequireFromString("12.345"),
			TotalAmount:      decimal.RequireFromString("123.45"),
			CreatedAt:        time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		},
		ChargerCode:         "CP0001",
		ChargerOCPPIdentity: "ocpp-identity-01",
		ChargerName:         "Main forecourt DC charger",
		HubID:               &hubID,
		HubName:             "Salt Lake Hub",
		HubAddress:          "1 Sector V Road, Kolkata",
		ConnectorNumber:     &connectorNumber,
		ConnectorType:       "CCS2",
	}

	view := toChargerTransactionView(transaction)
	if view.SessionID != sessionID || view.TransactionID != sessionID.String() || view.OCPPTransactionID != 654321 || view.HALTransactionID == nil || *view.HALTransactionID != halTransactionID {
		t.Fatalf("transaction identities=%+v", view)
	}
	if view.ChargerID != "CP0001" || view.OCPPIdentity != "ocpp-identity-01" || view.ChargerName != "Main forecourt DC charger" || view.HubID == nil || *view.HubID != hubID || view.HubAddress == "" || view.ConnectorNumber == nil || *view.ConnectorNumber != 2 || view.ConnectorType != "CCS2" {
		t.Fatalf("human-readable charger projection=%+v", view)
	}
	if !view.ReconciliationRequired || view.SessionStatus != constants.SessionStatusReconciliationRequired || view.SettlementStatus != "RECONCILIATION_REQUIRED" {
		t.Fatalf("reconciliation state=%+v", view)
	}
}

func TestCPOCustomerUsageAlwaysSerializes(t *testing.T) {
	t.Parallel()

	view := cpoAdminCustomerView(models.Customer{ID: uuid.New(), CPOID: uuid.New()}, &CustomerAggregates{})
	if !view.TotalUsage.Equal(decimal.Zero) {
		t.Fatalf("total usage=%s, want zero", view.TotalUsage)
	}
	encoded, err := json.Marshal(view)
	if err != nil || !strings.Contains(string(encoded), `"total_usage_kwh":"0"`) {
		t.Fatalf("zero usage JSON=%s err=%v", encoded, err)
	}
}

func TestLiveChargingSessionProjectionContainsOnlyOperationalContext(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	meter, consumed := int64(50120), int64(120)
	soc := decimal.RequireFromString("63.5")
	session := models.ChargingSession{
		ID:        uuid.New(),
		Status:    constants.SessionStatusActive,
		StartTime: now,
		Charger: models.Charger{
			ChargerID:   "cp0001",
			ChargerName: "Main forecourt DC charger",
			Hub:         &models.Hub{Name: "Salt Lake Hub"},
		},
		Connector: models.Connector{ConnectorNumber: 2},
	}
	view := toLiveChargingSessionView(session, liveops.SessionState{
		LatestMeterWh:    &meter,
		ConsumedWh:       &consumed,
		MeterObservedAt:  &now,
		MeterFreshness:   liveops.FreshnessFresh,
		LatestSoCPercent: &soc,
		SoCObservedAt:    &now,
		SoCFreshness:     liveops.FreshnessFresh,
	})

	if view.ChargerID != "cp0001" || view.ChargerName != "Main forecourt DC charger" || view.HubName == nil || *view.HubName != "Salt Lake Hub" || view.ConnectorNumber != 2 || view.LatestMeterWh == nil || *view.LatestMeterWh != meter || view.ConsumedWh == nil || *view.ConsumedWh != consumed || view.SoCPercent == nil || !view.SoCPercent.Equal(soc) {
		t.Fatalf("live session operational projection=%+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal live session view: %v", err)
	}
	for _, forbidden := range []string{"customer", "wallet", "tariff", "total_amount", "total_kwh"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("live session projection leaked %q: %s", forbidden, encoded)
		}
	}
}
