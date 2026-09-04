package cpo

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http/httptest"
)

func TestValidateChargerOperationHistoryQueryDefaultsCursorAndTypedFilters(t *testing.T) {
	before := time.Date(2026, time.September, 4, 10, 0, 0, 0, time.UTC)
	beforeID := uuid.New()
	reset := "soft"
	query, err := validateChargerOperationHistoryQuery(ChargerOperationHistoryQuery{Before: &before, BeforeID: &beforeID, ResetType: &reset})
	if err != nil {
		t.Fatal(err)
	}
	if query.Limit != 50 || query.Kind == nil || *query.Kind != "RESET" || query.ResetType == nil || *query.ResetType != "SOFT" {
		t.Fatalf("normalized query = %#v", query)
	}
}

func TestValidateChargerOperationHistoryQueryRejectsUnsafeOrIncompatibleFilters(t *testing.T) {
	reset, availability, wrongKind := "HARD", "OPERATIVE", "CLEAR_CACHE"
	after := time.Now().UTC()
	before := after.Add(-time.Second)
	for _, test := range []ChargerOperationHistoryQuery{
		{Before: &after},
		{Limit: 201},
		{CreatedAfter: &after, CreatedBefore: &before},
		{ResetType: &reset, AvailabilityType: &availability},
		{Kind: &wrongKind, ResetType: &reset},
	} {
		if _, err := validateChargerOperationHistoryQuery(test); err == nil {
			t.Fatalf("query %#v unexpectedly succeeded", test)
		}
	}
}

func TestChargerOperationHistoryProjectionRedactsConfigurationValueAndUnknownFields(t *testing.T) {
	row := chargerOperationHistoryRow{ChargerOperation: models.ChargerOperation{
		ID: uuid.New(), ChargerID: uuid.New(), ActorUserID: uuid.New(), Kind: "CHANGE_CONFIGURATION", State: "OCPP_CONFIRMED",
		Parameters: models.JSONB{"key": "ConnectionTimeOut", "value": "private operational value", "_connector_number": "2", "unexpected": "must not escape"},
	}, ChargerCode: "CP-001", ChargerName: "Main forecourt", ActorFullName: "CPO Operator"}
	view := chargerOperationHistoryView(row)
	if view.Parameters == nil || view.Parameters.Key != "ConnectionTimeOut" || view.Parameters.Type != "" || view.Parameters.Reason != "" {
		t.Fatalf("configuration projection = %#v", view.Parameters)
	}
	if view.Connector != nil || view.Charger.ChargerID != "CP-001" || view.Actor.FullName != "CPO Operator" {
		t.Fatalf("enriched projection = %#v", view)
	}
}

func TestChargerOperationHistoryProjectionRetainsOnlyTypedResetParameters(t *testing.T) {
	parameters := models.JSONB{"type": "SOFT", "reason": "Safe maintenance", "value": "must not escape", "_connector_number": "1"}
	view := chargerOperationHistoryView(chargerOperationHistoryRow{ChargerOperation: models.ChargerOperation{ID: uuid.New(), ChargerID: uuid.New(), ActorUserID: uuid.New(), Kind: "RESET", Parameters: parameters}})
	if view.Parameters == nil || view.Parameters.Type != "SOFT" || view.Parameters.Reason != "Safe maintenance" || view.Parameters.Key != "" || view.Parameters.RequestedMessage != "" {
		t.Fatalf("reset projection = %#v", view.Parameters)
	}
}

func TestParseChargerOperationHistoryQueryRequiresCanonicalUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest("GET", "/?charger_id="+uuid.NewString()+"%20", nil)
	writer := httptest.NewRecorder()
	engine := gin.New()
	engine.GET("/", func(ctx *gin.Context) {
		if _, ok := parseChargerOperationHistoryQuery(ctx); ok {
			t.Error("non-canonical UUID was accepted")
		}
	})
	engine.ServeHTTP(writer, request)
	if writer.Code != 400 {
		t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
	}
}
