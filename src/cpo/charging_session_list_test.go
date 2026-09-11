package cpo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type chargingSessionListRepository struct {
	sessions []models.ChargingSession
	cpoID    uuid.UUID
	query    ChargingSessionListQuery
}

func (r *chargingSessionListRepository) GetAnalytics(context.Context, uuid.UUID, *uuid.UUID, AnalyticsQuery) (Analytics, error) {
	return Analytics{}, nil
}
func (r *chargingSessionListRepository) ListWalletTransactions(context.Context, uuid.UUID, WalletTransactionListQuery) ([]WalletTransactionDetail, error) {
	return nil, nil
}
func (r *chargingSessionListRepository) GetChargingSession(context.Context, uuid.UUID, uuid.UUID) (*models.ChargingSession, error) {
	return nil, nil
}
func (r *chargingSessionListRepository) ListChargingSessions(_ context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error) {
	r.cpoID, r.query = cpoID, query
	return r.sessions, nil
}
func (r *chargingSessionListRepository) ListLiveChargingSessions(context.Context, uuid.UUID, LiveChargingSessionListQuery) ([]models.ChargingSession, error) {
	return nil, nil
}
func (r *chargingSessionListRepository) ListChargerTransactions(context.Context, uuid.UUID, ChargerTransactionListQuery) ([]ChargerTransaction, error) {
	return nil, nil
}
func (r *chargingSessionListRepository) ListChargersByHub(context.Context, uuid.UUID, uuid.UUID) ([]models.Charger, error) {
	return nil, nil
}
func (r *chargingSessionListRepository) ListVehicles(context.Context, uuid.UUID, VehicleListQuery) ([]VehicleDetail, error) {
	return nil, nil
}

func chargingSessionListQueryForTest(t *testing.T, rawQuery string) (ChargingSessionListQuery, bool, *httptest.ResponseRecorder) {
	t.Helper()
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cpo/charging-sessions?"+rawQuery, nil)
	query, ok := parseChargingSessionListQuery(ctx)
	return query, ok, writer
}

func TestParseChargingSessionListQueryPreservesDistinctBoundsAndTypedCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.New()
	asOf := "2026-09-11T12:00:00.123456789Z"
	query, ok, recorder := chargingSessionListQueryForTest(t, "sort_by=usage&sort_order=asc&cursor_value=12.345&cursor_id="+id.String()+"&total_kwh_gt=1.1&total_kwh_min=2.2&total_kwh_lt=9.9&total_kwh_max=10.1&total_amount_gt=3.3&total_amount_min=4.4&total_amount_lt=8.8&total_amount_max=9.9&duration_min=0&duration_max=60&currency=inr&as_of="+asOf)
	if !ok || recorder.Code != http.StatusOK {
		t.Fatalf("parse status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if query.Cursor == nil || query.Cursor.ID != id || query.Cursor.UsageKWh == nil || !query.Cursor.UsageKWh.Equal(decimal.RequireFromString("12.345")) {
		t.Fatalf("typed usage cursor=%+v", query.Cursor)
	}
	if query.TotalKWhGT == nil || query.TotalKWhMin == nil || query.TotalKWhLT == nil || query.TotalKWhMax == nil || query.TotalAmountGT == nil || query.TotalAmountMin == nil || query.TotalAmountLT == nil || query.TotalAmountMax == nil {
		t.Fatalf("strict and inclusive bounds were collapsed: %+v", query)
	}
	if query.Currency == nil || *query.Currency != "INR" || query.AsOf == nil || query.AsOf.Format(time.RFC3339Nano) != asOf {
		t.Fatalf("canonical currency/as_of=%+v", query)
	}
}

func TestParseChargingSessionListQueryRejectsInvalidCursorAndDurationInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validID := uuid.New().String()
	for _, rawQuery := range []string{
		"cursor_value=2026-09-11T12:00:00Z",
		"cursor_id=" + validID,
		"cursor_value=not-a-time&cursor_id=" + validID,
		"sort_by=usage&cursor_value=not-a-decimal&cursor_id=" + validID,
		"sort_by=duration&cursor_value=-1&cursor_id=" + validID,
		"sort_by=end_time&cursor_value=not-a-time&cursor_id=" + validID,
		"before_id=" + validID,
		"before=2026-09-11T12:00:00Z&before_id=" + validID + "&cursor_value=2026-09-11T12:00:00Z&cursor_id=" + validID,
		"duration_min=-1",
		"currency=INRR",
	} {
		if _, ok, recorder := chargingSessionListQueryForTest(t, rawQuery); ok || recorder.Code != http.StatusBadRequest {
			t.Fatalf("query %q status=%d ok=%t, want 400/false", rawQuery, recorder.Code, ok)
		}
	}

	legacy, ok, recorder := chargingSessionListQueryForTest(t, "before=2026-09-11T12:00:00Z")
	if !ok || recorder.Code != http.StatusOK || legacy.Before == nil || legacy.BeforeID != nil {
		t.Fatalf("before-only legacy cursor=%+v status=%d body=%s", legacy, recorder.Code, recorder.Body.String())
	}

	query, ok, recorder := chargingSessionListQueryForTest(t, "sort_by=end_time&cursor_value=null&cursor_id="+validID)
	if !ok || recorder.Code != http.StatusOK || query.Cursor == nil || !query.Cursor.EndTimeIsNull {
		t.Fatalf("nullable end-time cursor=%+v status=%d body=%s", query.Cursor, recorder.Code, recorder.Body.String())
	}
}

func TestListChargingSessionsValidatesRangesAndFreezesDurationChains(t *testing.T) {
	cpoID := uuid.New()
	service := &Service{repository: &chargingSessionListRepository{}, now: func() time.Time { return time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC) }}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoID}
	decimalOne, decimalTwo := decimal.NewFromInt(1), decimal.NewFromInt(2)
	zero, one := int64(0), int64(1)
	for _, query := range []ChargingSessionListQuery{
		{TotalKWhGT: &decimalOne, TotalKWhMin: &decimalTwo},
		{TotalAmountLT: &decimalOne, TotalAmountMax: &decimalTwo},
		{TotalKWhGT: &decimalOne, TotalKWhLT: &decimalOne},
		{TotalAmountMin: &decimalTwo, TotalAmountMax: &decimalOne},
		{DurationMin: &one, DurationMax: &zero},
		{DurationMin: func() *int64 { negative := int64(-1); return &negative }()},
		{Cursor: &ChargingSessionCursor{ID: uuid.New(), DurationSeconds: &zero}, SortBy: "duration"},
		{AsOf: func() *time.Time { value := time.Now().UTC(); return &value }()},
	} {
		if _, err := service.ListChargingSessions(context.Background(), principal, query); err == nil {
			t.Fatalf("query %+v was accepted", query)
		}
	}

	response, err := service.ListChargingSessions(context.Background(), principal, ChargingSessionListQuery{SortBy: "duration", DurationMin: &zero, Limit: 1})
	if err != nil || response.AsOf == nil || !response.AsOf.Equal(time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("initial duration query response=%+v err=%v", response, err)
	}
}

func TestListChargingSessionsRestoresActiveLiveKWhProjectionWithoutNPlusOne(t *testing.T) {
	cpoID := uuid.New()
	started := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	latest := int64(1_800)
	repository := &chargingSessionListRepository{sessions: []models.ChargingSession{
		{ID: uuid.New(), CPOID: cpoID, Status: constants.SessionStatusActive, MeterStartWh: 1_000, LatestMeterWh: &latest, TotalKWh: decimal.Zero, StartTime: started, CreatedAt: started},
		{ID: uuid.New(), CPOID: cpoID, Status: constants.SessionStatusCompleted, MeterStartWh: 1_000, TotalKWh: decimal.NewFromInt(2), StartTime: started, CreatedAt: started.Add(-time.Minute)},
	}}
	service := &Service{repository: repository}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoID}
	response, err := service.ListChargingSessions(context.Background(), principal, ChargingSessionListQuery{Limit: 1})
	if err != nil || len(response.Sessions) != 1 || !response.Sessions[0].TotalKWh.Equal(decimal.RequireFromString("0.8")) {
		t.Fatalf("list response=%+v err=%v", response, err)
	}
	if repository.cpoID != cpoID || repository.query.Limit != 1 || repository.query.SortBy != "created_at" || repository.query.SortOrder != "desc" {
		t.Fatalf("repository query=%+v cpo=%s", repository.query, repository.cpoID)
	}
	if !response.HasMore || response.NextCursorValue == nil || response.NextCursorID == nil || response.NextBefore == nil || response.NextBeforeID == nil {
		t.Fatalf("legacy and generic cursors were not emitted: %+v", response)
	}
}
