package cpo

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm/schema"
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
		ID:            uuid.New(),
		TransactionID: 654321,
		Status:        constants.SessionStatusActive,
		StartTime:     now,
		Customer:      models.Customer{FullName: "Chitradeep Ghosh"},
		Charger: models.Charger{
			ChargerID:   "cp0001",
			ChargerName: "Main forecourt DC charger",
			Hub:         &models.Hub{Name: "Salt Lake Hub"},
		},
		Connector: models.Connector{ID: uuid.New(), ConnectorNumber: 2},
	}
	view := toLiveChargingSessionView(session, liveops.SessionState{
		LatestMeterWh:    &meter,
		ConsumedWh:       &consumed,
		MeterObservedAt:  &now,
		MeterFreshness:   liveops.FreshnessFresh,
		LatestSoCPercent: &soc,
		SoCObservedAt:    &now,
		SoCFreshness:     liveops.FreshnessFresh,
	}, now.Add(92*time.Second))

	if view.OCPPTransactionID != 654321 || view.DurationSeconds != 92 || view.CustomerName != "Chitradeep Ghosh" || view.ChargerID != "cp0001" || view.ChargerName != "Main forecourt DC charger" || view.HubName == nil || *view.HubName != "Salt Lake Hub" || view.ConnectorID != session.Connector.ID || view.ConnectorNumber != 2 || view.LatestMeterWh == nil || *view.LatestMeterWh != meter || view.ConsumedWh == nil || *view.ConsumedWh != consumed || view.SoCPercent == nil || !view.SoCPercent.Equal(soc) {
		t.Fatalf("live session operational projection=%+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal live session view: %v", err)
	}
	for _, forbidden := range []string{"customer_id", "customer_email", "wallet", "tariff", "total_amount", "total_kwh"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("live session projection leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestLiveChargingSessionSnapshotSSEContainsTheFullOperationalProjection(t *testing.T) {
	t.Parallel()

	snapshot := LiveChargingSessionListResponse{
		Sessions: []LiveChargingSessionView{{
			SessionID: uuid.New(), OCPPTransactionID: 654321, CustomerName: "Chitradeep Ghosh", ChargerID: "cp0001", ChargerName: "Main forecourt DC charger",
			ConnectorID: uuid.New(), DurationSeconds: 92, Status: constants.SessionStatusActive,
		}},
		AsOf: time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC),
	}
	var output bytes.Buffer
	if err := writeLiveSessionSnapshot(&output, "live_sessions", 17, snapshot); err != nil {
		t.Fatalf("write live-session snapshot: %v", err)
	}

	frame := output.String()
	for _, expected := range []string{"id: 17\n", "event: live_sessions\n", `"sessions":[`, `"ocpp_transaction_id":654321`, `"duration_seconds":92`, `"customer_name":"Chitradeep Ghosh"`, `"charger_id":"cp0001"`, `"connector_id":`, `"as_of":"2026-08-25T12:00:00Z"`} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("SSE frame %q missing %q", frame, expected)
		}
	}
	for _, forbidden := range []string{"customer_id", "customer_email", "wallet", "tariff", "total_amount", "total_kwh"} {
		if strings.Contains(frame, forbidden) {
			t.Fatalf("SSE frame leaked %q: %s", forbidden, frame)
		}
	}
}

func TestCPOLiveSessionSnapshotFingerprintIgnoresAsOfButRetainsProjectionState(t *testing.T) {
	t.Parallel()
	snapshot := LiveChargingSessionFinancialListResponse{
		Sessions: []LiveChargingSessionFinancialView{{LiveChargingSessionView: LiveChargingSessionView{SessionID: uuid.New(), Status: constants.SessionStatusActive}, ProjectedAmount: decimal.RequireFromString("12.50"), Currency: "INR"}},
		AsOf:     time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC),
	}
	first, err := cpoLiveSessionSnapshotFingerprint(snapshot)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	later := snapshot
	later.AsOf = later.AsOf.Add(time.Minute)
	second, err := cpoLiveSessionSnapshotFingerprint(later)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("as_of-only refresh changed CPO fingerprint: %v", err)
	}
	later.Sessions[0].ProjectedAmount = decimal.RequireFromString("13.00")
	third, err := cpoLiveSessionSnapshotFingerprint(later)
	if err != nil || bytes.Equal(first, third) {
		t.Fatalf("client-visible projected amount did not change CPO fingerprint: %v", err)
	}
}

func TestSessionChargerFallbackUsesTenantResolvedConnectorCharger(t *testing.T) {
	t.Parallel()

	chargerID := uuid.New()
	session := models.ChargingSession{
		Connector: models.Connector{ChargerID: chargerID},
	}
	charger := models.Charger{ID: chargerID, ChargerID: "cp0001", ChargerName: "Main forecourt DC charger", Hub: &models.Hub{Name: "Salt Lake Hub"}}
	assignSessionChargerFallback(&session, map[uuid.UUID]models.Charger{chargerID: charger})

	if session.Charger.ID != chargerID || session.ChargerID != chargerID || session.Charger.ChargerID != "cp0001" || session.Charger.Hub == nil || session.Charger.Hub.Name != "Salt Lake Hub" {
		t.Fatalf("connector charger fallback=%+v", session.Charger)
	}

	missing := models.ChargingSession{Connector: models.Connector{ChargerID: uuid.New()}}
	assignSessionChargerFallback(&missing, map[uuid.UUID]models.Charger{})
	if missing.Charger.ID != uuid.Nil {
		t.Fatalf("missing charger must not be fabricated: %+v", missing.Charger)
	}
}

func TestCustomerUsageAggregateUsesCanonicalKWhAlias(t *testing.T) {
	t.Parallel()

	for _, target := range []any{&CustomerAggregates{}, &customerAggregateResult{}} {
		parsed, err := schema.Parse(target, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse aggregate %T: %v", target, err)
		}
		field := parsed.LookUpField("TotalUsageKWh")
		if field == nil {
			t.Fatalf("aggregate %T has no TotalUsageKWh field", target)
		}
		if field.DBName != "total_usage_kwh" {
			t.Fatalf("aggregate %T maps TotalUsageKWh to %q, want total_usage_kwh", target, field.DBName)
		}
	}
}
