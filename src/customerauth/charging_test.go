package customerauth

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

func TestAffordableChargingLimitUsesExactIntegerWh(t *testing.T) {
	t.Parallel()
	tariff := models.Tariff{PricePerKWh: decimal.RequireFromString("10.0000")}
	hold, limit, err := affordableChargingLimit(decimal.RequireFromString("12.34"), tariff)
	if err != nil {
		t.Fatal(err)
	}
	if limit != 1234 || !hold.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("hold=%s limit=%d, want 12.34/1234", hold, limit)
	}
}

func TestChargingCredentialIsShortOpaqueAndStoredAsHash(t *testing.T) {
	t.Parallel()
	credential, hash, err := newChargingCredential()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(credential, "appv1_") || len(credential) > 20 || len(hash) != 64 || strings.Contains(hash, credential) {
		t.Fatalf("invalid credential/hash shape credential=%q hash=%q", credential, hash)
	}
}

func TestCanonicalFactDigestIsStableAndDetectsPayloadChanges(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("c0c7ed9f-5f39-4f7f-9a3d-bd95be4d6eaf")
	makeEnvelope := func(payload string) HALFactEnvelope {
		return HALFactEnvelope{FactID: id, FactType: "transaction.meter", SchemaVersion: 1, OccurredAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC), Producer: "ocpp-hal-go-new", Payload: json.RawMessage(payload)}
	}
	first, err := canonicalFactDigest(makeEnvelope(`{"z":7,"a":{"value_wh":42}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalFactDigest(makeEnvelope(`{"a":{"value_wh":42},"z":7}`))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := canonicalFactDigest(makeEnvelope(`{"a":{"value_wh":43},"z":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == changed {
		t.Fatalf("digest stability/integrity failed first=%s second=%s changed=%s", first, second, changed)
	}
}

func TestChargingSessionHistoryQueryValidation(t *testing.T) {
	t.Parallel()

	query := ChargingSessionHistoryQuery{}
	if err := validateChargingSessionHistoryQuery(&query); err != nil {
		t.Fatalf("default history query rejected: %v", err)
	}
	if query.Limit != chargingSessionHistoryDefaultLimit {
		t.Fatalf("default history limit=%d, want %d", query.Limit, chargingSessionHistoryDefaultLimit)
	}
	query.Limit = chargingSessionHistoryMaxLimit + 1
	if err := validateChargingSessionHistoryQuery(&query); err == nil {
		t.Fatal("overlarge history limit was accepted")
	}
	before := time.Now().UTC()
	query = ChargingSessionHistoryQuery{Before: &before}
	if err := validateChargingSessionHistoryQuery(&query); err == nil {
		t.Fatal("partial history cursor was accepted")
	}
}

func TestChargingSessionProjectionsUsePersistedCompletionAndSnapshots(t *testing.T) {
	t.Parallel()

	cpoID, sessionID, intentID := uuid.New(), uuid.New(), uuid.New()
	meterStop := int64(12_430)
	completedAt := time.Date(2026, 8, 13, 10, 2, 0, 0, time.UTC)
	session := models.ChargingSession{
		ID:               sessionID,
		CPOID:            cpoID,
		StartTime:        completedAt.Add(-48 * time.Minute),
		EndTime:          &completedAt,
		MeterStartWh:     0,
		MeterStopWh:      &meterStop,
		LatestMeterWh:    int64Pointer(1), // A stale pre-completion value must not win.
		TotalKWh:         decimal.RequireFromString("12.430"),
		TotalAmount:      decimal.RequireFromString("186.25"),
		Currency:         "INR",
		Status:           constants.SessionStatusCompleted,
		SettlementStatus: "COMPLETED",
		TariffSnapshot:   models.JSONB{"currency": "INR", "price_per_kwh": "15.00", "idle_fee_per_min": "2.50"},
		TaxSnapshot:      models.JSONB{"cgst_rate": "9.00", "sgst_rate": "9.00"},
		Charger: models.Charger{
			ID: uuid.New(), ChargerID: "a1b2c3", ChargerName: "TransEV Salt Lake",
			Hub: &models.Hub{ID: uuid.New(), Name: "Salt Lake", Address: "1 Test Road"},
		},
		Connector: models.Connector{ID: uuid.New(), ConnectorNumber: 2, ConnectorType: "CCS2"},
	}

	history := customerChargingSessionHistoryView(session)
	if history.ConsumedWh == nil || *history.ConsumedWh != meterStop || history.TotalKWh == nil || *history.TotalKWh != "12.430" || history.TotalAmount == nil || *history.TotalAmount != "186.25" {
		t.Fatalf("unexpected completed history projection: %+v", history)
	}
	if history.Charger.Name != "TransEV Salt Lake" || history.Charger.Hub == nil || history.Connector.Number != 2 || history.Connector.Type != "CCS2" {
		t.Fatalf("history omitted card presentation data: %+v", history)
	}

	ledgerID, paymentID := uuid.New(), uuid.New()
	session.Payment = &models.Payment{
		ID: paymentID, CPOID: cpoID, SessionID: sessionID, WalletTransactionID: ledgerID,
		Amount: decimal.RequireFromString("186.25"), PaymentMethod: "WALLET", Status: constants.FinancialStatusCompleted,
		WalletTransaction: models.WalletTransaction{ID: ledgerID, CPOID: cpoID, SessionID: &sessionID, TransactionType: constants.WalletTransactionTypeDebit},
	}
	detail := customerChargingSessionDetailView(session, models.ChargingStartIntent{ID: intentID, Status: constants.StartIntentStatusActuallyStarted}, liveops.SessionState{State: "COMPLETED", CompletedAt: &completedAt, MeterFreshness: liveops.FreshnessFresh}, liveops.ChargerState{ConnectionState: "UNKNOWN"}, liveops.ConnectorState{Freshness: liveops.FreshnessUnknown})
	if detail.Financial == nil || detail.Financial.PaymentID != paymentID || detail.Financial.WalletTransactionID != ledgerID || detail.Pricing.PricePerKWh == nil || *detail.Pricing.PricePerKWh != "15.00" || detail.Tax.CGSTRate == nil || *detail.Tax.CGSTRate != "9.00" {
		t.Fatalf("detail did not preserve safe settlement and frozen price/tax data: %+v", detail)
	}

	active := session
	active.Status = constants.SessionStatusActive
	active.EndTime, active.MeterStopWh = nil, nil
	active.TotalKWh, active.TotalAmount = decimal.RequireFromString("0"), decimal.RequireFromString("0")
	activeHistory := customerChargingSessionHistoryView(active)
	if activeHistory.TotalKWh != nil || activeHistory.TotalAmount != nil {
		t.Fatalf("active session exposed provisional totals as final: %+v", activeHistory)
	}
}

func TestChargingSessionFinancialProjectionRejectsUnrelatedWalletRecord(t *testing.T) {
	t.Parallel()

	cpoID, sessionID := uuid.New(), uuid.New()
	session := models.ChargingSession{ID: sessionID, CPOID: cpoID, Currency: "INR", Payment: &models.Payment{
		ID: uuid.New(), CPOID: cpoID, SessionID: sessionID, WalletTransactionID: uuid.New(), Amount: decimal.NewFromInt(1),
		WalletTransaction: models.WalletTransaction{CPOID: cpoID, TransactionType: constants.WalletTransactionTypeDebit},
	}}
	if financial := customerChargingSessionFinancialView(session); financial != nil {
		t.Fatalf("unlinked wallet transaction was exposed as settlement: %+v", financial)
	}
}

func TestChargingSessionOperationalEventUsesMaterializedSessionID(t *testing.T) {
	t.Parallel()

	sessionID, intentID := uuid.New(), uuid.New()
	session := models.ChargingSession{ID: sessionID, CPOID: uuid.New(), CustomerID: uuid.New(), MeterSequence: 7}
	event, ok := chargingSessionOperationalEvent("transaction.meter", session, models.JSONB{
		"cms_start_intent_id": intentID.String(),
		"meter_sequence":      float64(7),
	})
	if !ok || event.Type != "charging.meter_changed" || event.ResourceType != "CHARGING_SESSION" || event.ResourceID != sessionID.String() || event.ResourceID == intentID.String() {
		t.Fatalf("transaction event did not name its materialized session: %+v", event)
	}
	if _, ok := chargingSessionOperationalEvent("transaction.meter", session, models.JSONB{"meter_sequence": float64(6)}); ok {
		t.Fatal("stale meter sequence emitted an invalidation")
	}
}

func int64Pointer(value int64) *int64 { return &value }
