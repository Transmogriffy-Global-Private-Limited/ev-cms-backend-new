package cpo

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type chargingSessionProjectionRepository struct {
	session      models.ChargingSession
	getCPOID     uuid.UUID
	listCPOID    uuid.UUID
	getSessionID uuid.UUID
}

func (r *chargingSessionProjectionRepository) GetAnalytics(context.Context, uuid.UUID, *uuid.UUID, AnalyticsQuery) (Analytics, error) {
	return Analytics{}, nil
}

func (r *chargingSessionProjectionRepository) ListWalletTransactions(context.Context, uuid.UUID, WalletTransactionListQuery) ([]WalletTransactionDetail, error) {
	return nil, nil
}

func (r *chargingSessionProjectionRepository) GetChargingSession(_ context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error) {
	r.getCPOID, r.getSessionID = cpoID, sessionID
	if sessionID != r.session.ID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := r.session
	return &copy, nil
}

func (r *chargingSessionProjectionRepository) ListChargingSessions(_ context.Context, cpoID uuid.UUID, _ ChargingSessionListQuery) ([]models.ChargingSession, error) {
	r.listCPOID = cpoID
	return []models.ChargingSession{r.session}, nil
}

func (r *chargingSessionProjectionRepository) ListLiveChargingSessions(context.Context, uuid.UUID, LiveChargingSessionListQuery) ([]models.ChargingSession, error) {
	return nil, nil
}

func (r *chargingSessionProjectionRepository) ListChargerTransactions(context.Context, uuid.UUID, ChargerTransactionListQuery) ([]ChargerTransaction, error) {
	return nil, nil
}

func (r *chargingSessionProjectionRepository) ListChargersByHub(context.Context, uuid.UUID, uuid.UUID) ([]models.Charger, error) {
	return nil, nil
}

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

func TestChargingSessionProjectionUsesFrozenCommercialContextAndRequestedLimit(t *testing.T) {
	t.Parallel()

	minutes, money := constants.UnitMinutes, constants.ChargingLimitTypeMoney
	requested := decimal.RequireFromString("250.50")
	currentSGST, currentCGST, currentIGST := decimal.NewFromInt(2), decimal.NewFromInt(2), decimal.NewFromInt(4)
	session := models.ChargingSession{
		ID:            uuid.New(),
		TransactionID: 654321,
		Tariff: models.Tariff{
			PricePerUnit: decimal.RequireFromString("99.99"),
			Units:        func() *constants.Unit { value := constants.UnitKWh; return &value }(),
		},
		TariffSnapshot: models.JSONB{"price_per_unit": "7.25", "units": "minutes"},
		TaxSnapshot:    models.JSONB{"sgst_rate": "9.00", "cgst_rate": "9.00", "igst_rate": "0.00"},
		StartIntent:    &models.ChargingStartIntent{LimitType: money, RequestedLimitValue: &requested},
		Customer:       models.Customer{ID: uuid.New(), FullName: "Historical Customer", Email: "customer@example.com"},
		Charger: models.Charger{ID: uuid.New(), ChargerID: "cp0001", ChargerName: "Historical charger", Hub: &models.Hub{
			Name: "Historical hub", GST: &models.GST{SGSTRate: &currentSGST, CGSTRate: &currentCGST, IGSTRate: &currentIGST},
		}},
		Connector: models.Connector{ID: uuid.New(), ConnectorNumber: 1, ConnectorType: "CCS2"},
	}

	view := toChargingSessionView(session)
	if !view.PricePerUnit.Equal(decimal.RequireFromString("7.25")) || view.Unit == nil || *view.Unit != minutes {
		t.Fatalf("tariff display=%s/%v, want frozen 7.25/minutes", view.PricePerUnit, view.Unit)
	}
	if !view.SGSTPercent.Equal(decimal.NewFromInt(9)) || !view.CGSTPercent.Equal(decimal.NewFromInt(9)) || !view.IGSTPercent.Equal(decimal.Zero) {
		t.Fatalf("tax display=%s/%s/%s, want frozen 9/9/0", view.SGSTPercent, view.CGSTPercent, view.IGSTPercent)
	}
	if view.StartCriteria == nil || *view.StartCriteria != money || view.RequestedLimitValue == nil || !view.RequestedLimitValue.Equal(requested) {
		t.Fatalf("limit display=%v/%v, want MONEY/250.50", view.StartCriteria, view.RequestedLimitValue)
	}
	if view.Customer.Name != "Historical Customer" || view.Charger.ChargerID != "cp0001" || view.Connector.Number != 1 {
		t.Fatalf("existing operational projection regressed: %+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"price_per_unit":"7.25"`, `"unit":"minutes"`, `"start_criteria":"MONEY"`, `"requested_limit_value":"250.5"`, `"sgst_percent":"9"`, `"cgst_percent":"9"`, `"igst_percent":"0"`} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("session JSON %s missing %s", encoded, expected)
		}
	}
}

func TestChargingSessionProjectionUsesCurrentCommercialDataOnlyWithoutSnapshot(t *testing.T) {
	t.Parallel()

	kwh := constants.UnitKWh
	sgst, cgst, igst := decimal.NewFromInt(6), decimal.NewFromInt(6), decimal.Zero
	session := models.ChargingSession{
		Tariff:         models.Tariff{PricePerUnit: decimal.RequireFromString("11.50"), Units: &kwh},
		Charger:        models.Charger{Hub: &models.Hub{GST: &models.GST{SGSTRate: &sgst, CGSTRate: &cgst, IGSTRate: &igst}}},
		TariffSnapshot: models.JSONB{},
		TaxSnapshot:    models.JSONB{},
	}

	view := toChargingSessionView(session)
	if !view.PricePerUnit.Equal(decimal.RequireFromString("11.50")) || view.Unit == nil || *view.Unit != kwh {
		t.Fatalf("tariff fallback=%s/%v, want current 11.50/kwh", view.PricePerUnit, view.Unit)
	}
	if !view.SGSTPercent.Equal(sgst) || !view.CGSTPercent.Equal(cgst) || !view.IGSTPercent.Equal(igst) {
		t.Fatalf("tax fallback=%s/%s/%s, want current 6/6/0", view.SGSTPercent, view.CGSTPercent, view.IGSTPercent)
	}
	if view.StartCriteria != nil || view.RequestedLimitValue != nil {
		t.Fatalf("missing start intent must remain absent, got %v/%v", view.StartCriteria, view.RequestedLimitValue)
	}
}

func TestChargingSessionModelExposesScopedStartIntentAssociation(t *testing.T) {
	t.Parallel()

	parsed, err := schema.Parse(&models.ChargingSession{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Relationships.Relations["StartIntent"] == nil {
		t.Fatal("ChargingSession must expose StartIntent for repository-scoped preload")
	}
}

func TestCPOChargingSessionGetAndListKeepCommercialProjectionTenantScoped(t *testing.T) {
	t.Parallel()

	cpoID := uuid.New()
	criteria := constants.ChargingLimitTypeTime
	requested := decimal.RequireFromString("45")
	session := models.ChargingSession{
		ID:             uuid.New(),
		CPOID:          cpoID,
		TariffSnapshot: models.JSONB{"price_per_unit": "3", "units": "minutes"},
		TaxSnapshot:    models.JSONB{"sgst_rate": "9", "cgst_rate": "9", "igst_rate": "0"},
		StartIntent:    &models.ChargingStartIntent{LimitType: criteria, RequestedLimitValue: &requested},
	}
	repository := &chargingSessionProjectionRepository{session: session}
	service := &Service{repository: repository}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoID}

	got, err := service.GetChargingSession(context.Background(), principal, session.ID)
	if err != nil || repository.getCPOID != cpoID || repository.getSessionID != session.ID {
		t.Fatalf("single session read=%+v err=%v scope=%s/%s", got, err, repository.getCPOID, repository.getSessionID)
	}
	if !got.PricePerUnit.Equal(decimal.NewFromInt(3)) || got.StartCriteria == nil || *got.StartCriteria != criteria || got.RequestedLimitValue == nil || !got.RequestedLimitValue.Equal(requested) {
		t.Fatalf("single session commercial projection=%+v", got)
	}

	listed, err := service.ListChargingSessions(context.Background(), principal, ChargingSessionListQuery{Limit: 1})
	if err != nil || repository.listCPOID != cpoID || len(listed.Sessions) != 1 {
		t.Fatalf("list session response=%+v err=%v scope=%s", listed, err, repository.listCPOID)
	}
	if !listed.Sessions[0].PricePerUnit.Equal(decimal.NewFromInt(3)) || listed.Sessions[0].StartCriteria == nil || *listed.Sessions[0].StartCriteria != criteria {
		t.Fatalf("list session commercial projection=%+v", listed.Sessions[0])
	}
}

func TestLiveChargingSessionProjectionContainsActiveStaticContextAndTelemetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	meter, consumed := int64(50120), int64(120)
	soc := decimal.RequireFromString("63.5")
	session := models.ChargingSession{
		ID:            uuid.New(),
		TransactionID: 654321,
		Status:        constants.SessionStatusActive,
		StartTime:     now,
		Customer:      models.Customer{ID: uuid.New(), FullName: "Chitradeep Ghosh", Email: "chitradeep@example.test"},
		Charger: models.Charger{
			ID:          uuid.New(),
			ChargerID:   "cp0001",
			ChargerName: "Main forecourt DC charger",
			Hub:         &models.Hub{Name: "Salt Lake Hub"},
		},
		Connector: models.Connector{ID: uuid.New(), ConnectorNumber: 2, ConnectorType: "CCS2"},
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
	for _, forbidden := range []string{"wallet", "total_amount", "total_kwh"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("live session projection leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestLiveChargingSessionProjectionRetainsNormalActiveStaticContext(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	initial := decimal.RequireFromString("31.5")
	limit := decimal.RequireFromString("45")
	criteria := constants.ChargingLimitTypeTime
	vendor, model := "TransEV", "DC47"
	hubID := uuid.New()
	session := models.ChargingSession{
		ID: uuid.New(), TransactionID: 987654, Status: constants.SessionStatusActive, StartTime: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour), InitialSoCPercent: &initial,
		Currency: "INR", TariffSnapshot: models.JSONB{"price_per_unit": "2", "units": "minutes"}, TaxSnapshot: models.JSONB{"sgst_rate": "0", "cgst_rate": "9", "igst_rate": "0"},
		Customer:    models.Customer{ID: uuid.New(), FullName: "Puja Das", Email: "puja@example.test"},
		Charger:     models.Charger{ID: uuid.New(), ChargerID: "cp0002", OCPPIdentity: "station-2", ChargerName: "City DC", HubID: &hubID, Hub: &models.Hub{ID: hubID, Name: "City Hub", Address: "2 Test Lane"}, MaxPowerKW: 47, Vendor: &vendor, Model: &model},
		Connector:   models.Connector{ID: uuid.New(), ConnectorNumber: 2, ConnectorType: "CCS2", ConnectorTotalCapacity: 47},
		StartIntent: &models.ChargingStartIntent{LimitType: criteria, RequestedLimitValue: &limit},
	}

	normal := toChargingSessionView(session)
	live := toLiveChargingSessionView(session, liveops.SessionState{MeterFreshness: liveops.FreshnessFresh, SoCFreshness: liveops.FreshnessFresh}, now)
	if live.Customer != normal.Customer || live.Charger != normal.Charger || live.Connector != normal.Connector || live.InitialSoCPercent == nil || !live.InitialSoCPercent.Equal(initial) || !live.PricePerUnit.Equal(normal.PricePerUnit) || live.Unit == nil || normal.Unit == nil || *live.Unit != *normal.Unit || live.StartCriteria == nil || *live.StartCriteria != criteria || live.RequestedLimitValue == nil || !live.RequestedLimitValue.Equal(limit) || !live.SGSTPercent.Equal(normal.SGSTPercent) || !live.CGSTPercent.Equal(normal.CGSTPercent) || !live.IGSTPercent.Equal(normal.IGSTPercent) || !live.CreatedAt.Equal(normal.CreatedAt) {
		t.Fatalf("live static projection diverged from normal session view: live=%+v normal=%+v", live, normal)
	}
	if live.CustomerName != normal.Customer.Name || live.ChargerID != normal.Charger.ChargerID || live.ChargerName != normal.Charger.Name || live.HubName == nil || *live.HubName != *normal.Charger.HubName || live.ConnectorID != normal.Connector.ID || live.ConnectorNumber != normal.Connector.Number {
		t.Fatalf("legacy live display fields diverged from canonical context: %+v", live)
	}
}

func TestLiveChargingSessionSnapshotSSEContainsTheFullOperationalProjection(t *testing.T) {
	t.Parallel()

	unit := constants.UnitMinutes
	snapshot := LiveChargingSessionFinancialListResponse{
		Sessions: []LiveChargingSessionFinancialView{{LiveChargingSessionView: LiveChargingSessionView{
			SessionID: uuid.New(), OCPPTransactionID: 654321, CustomerName: "Chitradeep Ghosh", ChargerID: "cp0001", ChargerName: "Main forecourt DC charger",
			Customer: ChargingSessionCustomerView{ID: uuid.New(), Name: "Chitradeep Ghosh", Email: "chitradeep@example.test"}, Charger: ChargingSessionChargerView{ID: uuid.New(), ChargerID: "cp0001", Name: "Main forecourt DC charger"},
			Connector: ChargingSessionConnectorView{ID: uuid.New(), Number: 1, ConnectorType: "CCS2"}, ConnectorID: uuid.New(), DurationSeconds: 92, Status: constants.SessionStatusActive,
			PricePerUnit: decimal.RequireFromString("2"), Unit: &unit,
		}, ProjectedAmount: decimal.RequireFromString("12.50"), Currency: "INR"}},
		AsOf: time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC),
	}
	for _, eventType := range []string{"snapshot", "live_sessions"} {
		var output bytes.Buffer
		if err := writeLiveSessionSnapshot(&output, eventType, 17, snapshot); err != nil {
			t.Fatalf("write %s live-session snapshot: %v", eventType, err)
		}

		frame := output.String()
		for _, expected := range []string{"id: 17\n", "event: " + eventType + "\n", `"sessions":[`, `"ocpp_transaction_id":654321`, `"duration_seconds":92`, `"customer":{"id":`, `"charger":{"id":`, `"connector":{"id":`, `"price_per_unit":"2"`, `"projected_amount":"12.5"`, `"as_of":"2026-08-25T12:00:00Z"`} {
			if !strings.Contains(frame, expected) {
				t.Fatalf("%s SSE frame %q missing %q", eventType, frame, expected)
			}
		}
		for _, forbidden := range []string{"wallet", "total_amount", "total_kwh"} {
			if strings.Contains(frame, forbidden) {
				t.Fatalf("%s SSE frame leaked %q: %s", eventType, forbidden, frame)
			}
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
