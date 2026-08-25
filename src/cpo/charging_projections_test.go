package cpo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
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
