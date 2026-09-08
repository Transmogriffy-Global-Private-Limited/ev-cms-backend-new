package customerauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestUpdateVehicleRequestTracksOmittedNullAndRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	var omitted UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("decode omitted vehicle update: %v", err)
	}
	if omitted.vehicleNumberSet || omitted.vehicleTypeSet || omitted.vehicleMakeSet || omitted.vehicleModelSet {
		t.Fatalf("omitted fields were marked present: %#v", omitted)
	}
	var cleared UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"vehicle_type":null}`), &cleared); err != nil {
		t.Fatalf("decode null vehicle type: %v", err)
	}
	if !cleared.vehicleTypeSet || cleared.VehicleType != nil {
		t.Fatalf("null vehicle type was not retained as a clear: %#v", cleared)
	}
	var supplied UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"vehicle_number":"KA01AB1234","vehicle_make":"Tata"}`), &supplied); err != nil {
		t.Fatalf("decode supplied vehicle fields: %v", err)
	}
	if !supplied.vehicleNumberSet || supplied.VehicleNumber == nil || *supplied.VehicleNumber != "KA01AB1234" || !supplied.vehicleMakeSet {
		t.Fatalf("supplied fields were not preserved: %#v", supplied)
	}
	if err := json.Unmarshal([]byte(`{"customer_id":"`+uuid.NewString()+`"}`), &supplied); err == nil {
		t.Fatal("server-owned vehicle field was accepted")
	}
}

func TestCustomerVehicleListQueryNormalizesAndValidatesBounds(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/vehicles?search=%20Tata%20&vehicle_type=%20SUV%20&vehicle_make=%20Tata%20&vehicle_model=%20Nexon%20&limit=50", nil)
	query, err := customerVehicleListQuery(context)
	if err != nil {
		t.Fatalf("parse valid vehicle query: %v", err)
	}
	if query.Search != "Tata" || query.VehicleType != "SUV" || query.VehicleMake != "Tata" || query.VehicleModel != "Nexon" || query.Limit != 50 {
		t.Fatalf("unexpected normalized query: %#v", query)
	}
	for _, raw := range []string{"?search=" + strings.Repeat("x", 101), "?vehicle_type=" + strings.Repeat("x", 51), "?vehicle_make=" + strings.Repeat("x", 101), "?vehicle_model=" + strings.Repeat("x", 101), "?limit=101", "?before=2026-01-01T00:00:00Z"} {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodGet, "/vehicles"+raw, nil)
		if _, err := customerVehicleListQuery(context); err == nil {
			t.Errorf("query %q unexpectedly succeeded", raw)
		}
	}
}

func TestVehicleValidationAndViewDoNotExposeOwnership(t *testing.T) {
	t.Parallel()
	if _, err := normalizeVehicleNumber("   "); err == nil {
		t.Fatal("blank vehicle number was accepted")
	}
	blank := "  "
	if value, err := normalizeVehicleOptional(&blank, 50, "vehicle_type"); err != nil || value != nil {
		t.Fatalf("blank optional metadata = %v, %v; want nil, nil", value, err)
	}
	if _, err := normalizeVehicleOptional(stringPointer("x"), 0, "vehicle_type"); err == nil {
		t.Fatal("overlength optional metadata was accepted")
	}
	now := time.Now().UTC().Round(0)
	record := models.Vehicle{ID: uuid.New(), CPOID: uuid.New(), CustomerID: uuid.New(), VehicleNumber: "KA01AB1234", Type: stringPointer("SUV"), DateAdded: now, CreatedAt: now, UpdatedAt: now}
	view := customerVehicleView(record)
	if view.ID != record.ID || view.VehicleType == nil || *view.VehicleType != "SUV" || !view.DateAdded.Equal(now) {
		t.Fatalf("unexpected customer vehicle view: %#v", view)
	}
}
