package customerauth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestAffordableChargingLimitUsesExactIntegerWh(t *testing.T) {
	t.Parallel()
	tariffType, priceType, units := energyTariffMetadata()
	pricing, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: decimal.RequireFromString("10.00"), TariffType: &tariffType, PriceType: &priceType, Units: &units})
	if err != nil {
		t.Fatal(err)
	}
	zero := decimal.Zero
	hold, limit, err := affordableChargingLimit(decimal.RequireFromString("12.34"), pricing, models.GST{SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &zero}, models.Connector{ConnectorTotalCapacity: 7.4})
	if err != nil {
		t.Fatal(err)
	}
	if limit != 1234 || !hold.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("hold=%s limit=%d, want 12.34/1234", hold, limit)
	}
}

func TestFreeChargingStillRequiresPhysicalEnergyBound(t *testing.T) {
	t.Parallel()
	zero := decimal.Zero
	tariffType, priceType, units := energyTariffMetadata()
	pricing, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: zero, TariffType: &tariffType, PriceType: &priceType, Units: &units})
	if err != nil {
		t.Fatal(err)
	}
	_, limit, err := affordableChargingLimit(decimal.NewFromInt(1), pricing, models.GST{SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &zero}, models.Connector{ConnectorTotalCapacity: 7.4})
	if err != nil || limit != 7400 {
		t.Fatalf("free limit=%d err=%v, want 7400 Wh", limit, err)
	}
	if _, _, err := affordableChargingLimit(decimal.NewFromInt(1), pricing, models.GST{SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &zero}, models.Connector{}); err == nil {
		t.Fatal("free charging without a physical capacity must fail")
	}
}

func TestUsableWalletBalanceEnforcesCPOThresholdAndBuffer(t *testing.T) {
	t.Parallel()
	policy := models.Settings{WalletMinBalance: 500, WalletBufferMinBalance: 20}
	usable, err := usableWalletBalance(decimal.NewFromInt(500), policy)
	if err != nil || !usable.Equal(decimal.NewFromInt(480)) {
		t.Fatalf("usable balance=%s err=%v, want 480", usable, err)
	}
	tariffType, priceType, units := energyTariffMetadata()
	pricing, err := tariffPricingFromTariff(models.Tariff{
		PricePerUnit: decimal.NewFromInt(100),
		TariffType:   &tariffType,
		PriceType:    &priceType,
		Units:        &units,
	})
	if err != nil {
		t.Fatal(err)
	}
	zero := decimal.Zero
	_, limitWh, err := affordableChargingLimit(usable, pricing, models.GST{SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &zero}, models.Connector{ConnectorTotalCapacity: 7.4})
	if err != nil || limitWh != 4800 {
		t.Fatalf("usable balance energy limit=%d err=%v, want 4800 Wh", limitWh, err)
	}
	if _, err := usableWalletBalance(decimal.NewFromInt(499), policy); !errors.Is(err, errWalletMinimumBalance) {
		t.Fatalf("below-minimum error=%v, want wallet minimum error", err)
	}
	if _, err := usableWalletBalance(decimal.NewFromInt(20), models.Settings{WalletBufferMinBalance: 20}); err == nil {
		t.Fatal("buffer that leaves no positive usable balance was accepted")
	}
	zeroDefault, err := usableWalletBalance(decimal.NewFromInt(1), models.Settings{})
	if err != nil || !zeroDefault.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("zero-default usable balance=%s err=%v, want 1", zeroDefault, err)
	}
}

func TestChargingOccupancyAndFactIdentityInvariants(t *testing.T) {
	t.Parallel()
	for _, status := range chargingStartActiveStatuses {
		if status == constants.StartIntentStatusActuallyStarted {
			t.Fatal("historical ACTUALLY_STARTED start intent must not own connector occupancy")
		}
	}
	if len(chargingSessionOccupancyStatuses) != 3 || chargingSessionOccupancyStatuses[2] != constants.SessionStatusReconciliationRequired {
		t.Fatalf("session occupancy statuses=%v", chargingSessionOccupancyStatuses)
	}
	if _, ok := factID(models.JSONB{"id": uuid.Nil.String()}, "id"); ok {
		t.Fatal("zero UUID fact identity was accepted")
	}
	if id := uuid.New(); func() bool { got, ok := factID(models.JSONB{"id": id.String()}, "id"); return ok && got == id }() == false {
		t.Fatal("nonzero UUID fact identity was rejected")
	}
}

func TestTariffPricingCalculatesEachSupportedBasisAndLegacySnapshots(t *testing.T) {
	t.Parallel()
	zero := decimal.Zero
	gst := models.GST{SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &zero}
	startedAt := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	stoppedAt := startedAt.Add(90 * time.Second)

	energyType, energyPriceType, energyUnits := energyTariffMetadata()
	energy, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: decimal.RequireFromString("10.00"), TariffType: &energyType, PriceType: &energyPriceType, Units: &energyUnits})
	if err != nil {
		t.Fatal(err)
	}
	if amount, err := energy.amountWithGST(1234, startedAt, stoppedAt, gst); err != nil || !amount.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("energy amount=%s err=%v, want 12.34", amount, err)
	}
	energyAtCommercialRate, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: decimal.RequireFromString("16.91"), TariffType: &energyType, PriceType: &energyPriceType, Units: &energyUnits})
	if err != nil {
		t.Fatal(err)
	}
	if base, err := energyAtCommercialRate.baseAmount(7200, startedAt, stoppedAt); err != nil || !base.Equal(decimal.RequireFromString("121.752")) {
		t.Fatalf("energy base=%s err=%v, want 121.752 for 7.2 kWh at 16.91/kWh", base, err)
	}

	timeType := constants.TariffTypeFixed
	timePriceType := constants.PriceTypeTime
	timeUnits := constants.UnitMinutes
	timePricing, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: decimal.NewFromInt(2), TariffType: &timeType, PriceType: &timePriceType, Units: &timeUnits})
	if err != nil {
		t.Fatal(err)
	}
	if amount, err := timePricing.amountWithGST(0, startedAt, stoppedAt, gst); err != nil || !amount.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("time amount=%s err=%v, want 3.00", amount, err)
	}

	sessionType := constants.TariffTypeFixed
	sessionPriceType := constants.PriceTypeSession
	sessionPricing, err := tariffPricingFromTariff(models.Tariff{PricePerUnit: decimal.RequireFromString("25.50"), TariffType: &sessionType, PriceType: &sessionPriceType})
	if err != nil {
		t.Fatal(err)
	}
	if amount, err := sessionPricing.amountWithGST(0, startedAt, startedAt, gst); err != nil || !amount.Equal(decimal.RequireFromString("25.50")) {
		t.Fatalf("session amount=%s err=%v, want 25.50", amount, err)
	}
	if hold, _, err := affordableChargingLimit(decimal.NewFromInt(200), timePricing, gst, models.Connector{ConnectorTotalCapacity: 7.4}); err != nil || !hold.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("time hold=%s err=%v, want 120.00", hold, err)
	}
	if hold, _, err := affordableChargingLimit(decimal.NewFromInt(30), sessionPricing, gst, models.Connector{ConnectorTotalCapacity: 7.4}); err != nil || !hold.Equal(decimal.RequireFromString("25.50")) {
		t.Fatalf("session hold=%s err=%v, want 25.50", hold, err)
	}

	legacy, err := tariffPricingFromSnapshot(models.JSONB{"price_per_kwh": "10.00"})
	if err != nil {
		t.Fatal(err)
	}
	if amount, err := legacy.amountWithGST(1234, startedAt, stoppedAt, gst); err != nil || !amount.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("legacy amount=%s err=%v, want 12.34", amount, err)
	}
	legacyPerWh, err := tariffPricingFromSnapshot(models.JSONB{
		"price_per_unit": "16.91", "tariff_type": "fixed", "price_type": "energy", "units": "watt/hour",
	})
	if err != nil {
		t.Fatalf("decode released watt/hour snapshot: %v", err)
	}
	if base, err := legacyPerWh.baseAmount(7200, startedAt, stoppedAt); err != nil || !base.Equal(decimal.RequireFromString("121752")) {
		t.Fatalf("legacy watt/hour snapshot was silently reinterpreted: %s (%v)", base, err)
	}
}

func TestTariffPricingRejectsUnsupportedUnitCombinations(t *testing.T) {
	t.Parallel()
	tariffType := constants.TariffTypeFixed
	priceType := constants.PriceTypeTime
	units := constants.LegacyUnitWattHour
	if _, err := tariffPricingFromTariff(models.Tariff{TariffType: &tariffType, PriceType: &priceType, Units: &units}); !errors.Is(err, errUnsupportedTariffSemantics) {
		t.Fatalf("invalid time unit err=%v, want unsupported tariff semantics", err)
	}
	energy := constants.PriceTypeEnergy
	if _, err := tariffPricingFromTariff(models.Tariff{TariffType: &tariffType, PriceType: &energy, Units: &units}); !errors.Is(err, errUnsupportedTariffSemantics) {
		t.Fatalf("legacy energy unit err=%v, want unsupported tariff semantics", err)
	}
	if _, err := tariffPricingFromTariff(models.Tariff{TariffType: &tariffType, PriceType: &energy, Units: func() *constants.Unit { value := constants.UnitKWh; return &value }(), IdleFeePerMin: decimal.NewFromInt(1)}); !errors.Is(err, errUnsupportedTariffSemantics) {
		t.Fatalf("idle-fee tariff err=%v, want unsupported tariff semantics", err)
	}
}

func TestFrozenTaxSnapshotUsesCommercialComponentValidation(t *testing.T) {
	t.Parallel()

	pricing := tariffPricing{pricePerUnit: decimal.NewFromInt(10), tariffType: constants.TariffTypeFixed, priceType: constants.PriceTypeEnergy}
	startedAt := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	stoppedAt := startedAt.Add(time.Hour)
	for _, test := range []struct {
		name  string
		tax   models.JSONB
		valid bool
	}{
		{name: "valid split GST", tax: models.JSONB{"sgst_rate": "9", "cgst_rate": "9", "igst_rate": "0"}, valid: true},
		{name: "valid IGST", tax: models.JSONB{"sgst_rate": "0", "cgst_rate": "0", "igst_rate": "18"}, valid: true},
		{name: "mixed GST components", tax: models.JSONB{"sgst_rate": "9", "cgst_rate": "9", "igst_rate": "18"}},
		{name: "negative component", tax: models.JSONB{"sgst_rate": "-1", "cgst_rate": "0", "igst_rate": "0"}},
		{name: "over-limit component", tax: models.JSONB{"sgst_rate": "101", "cgst_rate": "0", "igst_rate": "0"}},
		{name: "missing component", tax: models.JSONB{"sgst_rate": "9", "cgst_rate": "9"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pricing.amountWithTaxSnapshot(1000, startedAt, stoppedAt, test.tax)
			if (err == nil) != test.valid {
				t.Fatalf("tax snapshot result err=%v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestTariffSnapshotAndMaterializedSessionFreezePricingSemantics(t *testing.T) {
	t.Parallel()

	tariffType := constants.TariffTypeFixed
	priceType := constants.PriceTypeTime
	units := constants.UnitMinutes
	tariff := models.Tariff{
		ID:           uuid.New(),
		Currency:     "INR",
		PricePerUnit: decimal.RequireFromString("2.50"),
		TariffType:   &tariffType,
		PriceType:    &priceType,
		Units:        &units,
	}
	pricing, err := tariffPricingFromTariff(tariff)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := pricing.snapshot(tariff)
	if snapshot["price_per_unit"] != "2.5" || snapshot["tariff_type"] != "fixed" || snapshot["price_type"] != "time" || snapshot["units"] != "minutes" {
		t.Fatalf("snapshot omitted or changed tariff semantics: %#v", snapshot)
	}
	if _, oldFieldPresent := snapshot["price_per_kwh"]; oldFieldPresent {
		t.Fatalf("new snapshot retained retired field: %#v", snapshot)
	}

	intent := models.ChargingStartIntent{
		ID:             uuid.New(),
		CPOID:          uuid.New(),
		CustomerID:     uuid.New(),
		ChargerID:      uuid.New(),
		ConnectorID:    uuid.New(),
		TariffID:       tariff.ID,
		TariffSnapshot: snapshot,
		TaxSnapshot:    models.JSONB{"sgst_rate": "0", "cgst_rate": "0", "igst_rate": "0"},
	}
	startedAt := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	session := materializedChargingSession(intent, 42, uuid.New(), startedAt, 100, startedAt)
	if session.TariffSnapshot["price_per_unit"] != "2.5" || session.TariffSnapshot["price_type"] != "time" || session.TariffSnapshot["units"] != "minutes" {
		t.Fatalf("materialized session did not retain frozen tariff snapshot: %#v", session.TariffSnapshot)
	}
	amount, err := chargingAmount(session.TariffSnapshot, session.TaxSnapshot, 0, startedAt, startedAt.Add(90*time.Second))
	if err != nil || !amount.Equal(decimal.RequireFromString("3.75")) {
		t.Fatalf("frozen time tariff amount=%s err=%v, want 3.75", amount, err)
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
	if detail.Financial == nil || detail.Financial.PaymentID != paymentID || detail.Financial.WalletTransactionID != ledgerID || detail.Pricing.LegacyPricePerKWh == nil || *detail.Pricing.LegacyPricePerKWh != "15.00" || detail.Tax.CGSTRate == nil || *detail.Tax.CGSTRate != "9.00" {
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
	session.SoCSequence = 4
	socEvent, ok := chargingSessionOperationalEvent("transaction.soc", session, models.JSONB{"soc_sequence": float64(4)})
	if !ok || socEvent.Type != "charging.telemetry_changed" || socEvent.ResourceID != sessionID.String() {
		t.Fatalf("SoC event did not invalidate its materialized session: %+v", socEvent)
	}
	if _, ok := chargingSessionOperationalEvent("transaction.soc", session, models.JSONB{"soc_sequence": float64(3)}); ok {
		t.Fatal("stale SoC sequence emitted an invalidation")
	}
}

func TestSoCFactValidationAndCustomerProjectionsNeverFabricateZero(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		payload models.JSONB
		valid   bool
	}{
		{payload: models.JSONB{"soc_percent": "0"}, valid: true},
		{payload: models.JSONB{"soc_percent": "100"}, valid: true},
		{payload: models.JSONB{"soc_percent": "67.125"}, valid: true},
		{payload: models.JSONB{"soc_percent": "-1"}},
		{payload: models.JSONB{"soc_percent": "100.001"}},
		{payload: models.JSONB{"soc_percent": "banana"}},
		{payload: models.JSONB{"soc_percent": float64(67)}},
	} {
		_, ok := factSoC(test.payload, "soc_percent")
		if ok != test.valid {
			t.Fatalf("payload=%#v valid=%t", test.payload, ok)
		}
	}
	observed := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	initial, latest := decimal.RequireFromString("35"), decimal.RequireFromString("63.5")
	session := models.ChargingSession{ID: uuid.New(), StartTime: observed.Add(-time.Hour), InitialSoCPercent: &initial, LatestSoCPercent: &latest, SoCObservedAt: &observed}
	history := customerChargingSessionHistoryView(session)
	if history.InitialSoCPercent == nil || *history.InitialSoCPercent != "35" || history.FinalSoCPercent == nil || *history.FinalSoCPercent != "63.5" || history.SoCObservedAt == nil || !history.SoCObservedAt.Equal(observed) {
		t.Fatalf("history lost observed SoC: %+v", history)
	}
	detail := customerChargingSessionDetailView(session, models.ChargingStartIntent{ID: uuid.New()}, liveops.SessionState{LatestSoCPercent: &latest, SoCObservedAt: &observed, SoCFreshness: liveops.FreshnessFresh}, liveops.ChargerState{}, liveops.ConnectorState{})
	if detail.SoCPercent == nil || *detail.SoCPercent != "63.5" || detail.SoCFreshness != liveops.FreshnessFresh {
		t.Fatalf("detail=%+v", detail)
	}
	unknown := customerChargingSessionHistoryView(models.ChargingSession{ID: uuid.New(), StartTime: observed})
	if unknown.InitialSoCPercent != nil || unknown.FinalSoCPercent != nil || unknown.SoCObservedAt != nil {
		t.Fatalf("unknown SoC became known: %+v", unknown)
	}
}

func TestChargingConnectorAllowsNewStartOnlyWhenFreshOnlineAndOCPPAllowsIt(t *testing.T) {
	t.Parallel()
	available, preparing, charging, faulted := "Available", "Preparing", "Charging", "Faulted"

	tests := []struct {
		name  string
		state liveops.ConnectorState
		want  bool
	}{
		{name: "available fresh online", state: liveops.ConnectorState{Availability: "AVAILABLE", Freshness: liveops.FreshnessFresh, ParentConnectionState: "ONLINE", LastOCPPStatus: &available}, want: true},
		{name: "preparing remains displayed charging but admits start", state: liveops.ConnectorState{Availability: "CHARGING", Freshness: liveops.FreshnessFresh, ParentConnectionState: "ONLINE", LastOCPPStatus: &preparing}, want: true},
		{name: "charging", state: liveops.ConnectorState{Availability: "CHARGING", Freshness: liveops.FreshnessFresh, ParentConnectionState: "ONLINE", LastOCPPStatus: &charging}},
		{name: "faulted", state: liveops.ConnectorState{Availability: "FAULTED", Freshness: liveops.FreshnessFresh, ParentConnectionState: "ONLINE", LastOCPPStatus: &faulted}},
		{name: "offline parent", state: liveops.ConnectorState{Availability: "AVAILABLE", Freshness: liveops.FreshnessFresh, ParentConnectionState: "OFFLINE", LastOCPPStatus: &available}},
		{name: "unknown", state: liveops.ConnectorState{Availability: "UNKNOWN", Freshness: liveops.FreshnessUnknown, ParentConnectionState: "ONLINE"}},
		{name: "stale", state: liveops.ConnectorState{Availability: "AVAILABLE", Freshness: liveops.FreshnessStale, ParentConnectionState: "ONLINE", LastOCPPStatus: &available}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := chargingConnectorAllowsNewStart(test.state); got != test.want {
				t.Fatalf("chargingConnectorAllowsNewStart(%+v)=%v, want %v", test.state, got, test.want)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }

func energyTariffMetadata() (constants.TariffType, constants.PriceType, constants.Unit) {
	return constants.TariffTypeFixed, constants.PriceTypeEnergy, constants.UnitKWh
}

func TestStartChargingTariffResolutionErrorDistinguishesTopologyAndInfrastructure(t *testing.T) {
	t.Parallel()

	topologyErr := startChargingTariffResolutionError(commercial.ErrTariffTemporalConflict)
	var apiError *APIError
	if !errors.As(topologyErr, &apiError) || apiError.Status != http.StatusConflict || apiError.Code != "no_eligible_tariff" {
		t.Fatalf("topology error=%v, want 409 no_eligible_tariff", topologyErr)
	}

	infrastructureErr := errors.New("query failed")
	if got := startChargingTariffResolutionError(infrastructureErr); !errors.Is(got, infrastructureErr) {
		t.Fatalf("infrastructure error=%v, want original query failure", got)
	}
}
