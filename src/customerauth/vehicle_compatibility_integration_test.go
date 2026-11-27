package customerauth

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// Use ONLY an explicitly selected disposable database with TEST_DATABASE_URL.
// Never point TEST_DATABASE_URL at the development or production database.
func TestCustomerVehicleCoWCompatibilityWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	service := newVehicleTestService(t, database)
	cpo := createActiveTestCPO(t, database)
	foreignCPO := createActiveTestCPO(t, database)
	owner := createVehicleTestCustomer(t, database, cpo.ID, "run2b-owner-"+uuid.NewString()+"@example.com")
	other := createVehicleTestCustomer(t, database, cpo.ID, "run2b-other-"+uuid.NewString()+"@example.com")
	foreign := createVehicleTestCustomer(t, database, foreignCPO.ID, "run2b-foreign-"+uuid.NewString()+"@example.com")
	p := Principal{CPOID: cpo.ID, CustomerID: owner.ID}
	otherP := Principal{CPOID: cpo.ID, CustomerID: other.ID}
	foreignP := Principal{CPOID: foreignCPO.ID, CustomerID: foreign.ID}

	// Old clients need not send compatibility fields; no synthetic defaults.
	legacy, err := service.CreateVehicle(ctx, p, CreateVehicleRequest{VehicleNumber: "WB02LEGACY"})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.BatteryCapacityWh != nil || legacy.SupportsAC != nil || legacy.SupportsDC != nil || legacy.ConnectorTypes != nil || legacy.Version != 1 {
		t.Fatalf("legacy unknowns: %+v", legacy)
	}

	labels := []string{" ccs2 ", " TYPE2 "}
	withCompat, err := service.CreateVehicle(ctx, p, CreateVehicleRequest{
		VehicleNumber:              "WB02EVCOMP",
		VehicleCompatibilityFields: VehicleCompatibilityFields{BatteryCapacityWh: vehicleTestI64(50000), ConnectorTypes: &labels, SupportsAC: vehicleTestBool(true), SupportsDC: vehicleTestBool(true), MaxACPowerW: vehicleTestI64(11000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if withCompat.Version != 1 || withCompat.ConnectorTypes == nil || (*withCompat.ConnectorTypes)[0] != "CCS2" {
		t.Fatalf("create normalization: %+v", withCompat)
	}
	reread, err := service.GetVehicle(ctx, p, withCompat.ID)
	if err != nil || reread.ConnectorTypes == nil || len(*reread.ConnectorTypes) != 2 || reread.BatteryCapacityWh == nil || *reread.BatteryCapacityWh != 50000 {
		t.Fatalf("db roundtrip: %+v err=%v", reread, err)
	}
	for _, q := range []Principal{otherP, foreignP} {
		if _, err := service.GetVehicle(ctx, q, withCompat.ID); apiErrorCode(err) != "vehicle_not_found" {
			t.Fatalf("foreign read: %v", err)
		}
		if _, err := service.UpdateVehicle(ctx, q, withCompat.ID, UpdateVehicleRequest{VehicleModel: stringPointer("a"), vehicleModelSet: true}); apiErrorCode(err) != "vehicle_not_found" {
			t.Fatalf("foreign update: %v", err)
		}
		if err := service.DeleteVehicle(ctx, q, withCompat.ID); apiErrorCode(err) != "vehicle_not_found" {
			t.Fatalf("foreign delete: %v", err)
		}
	}

	var patch UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"expected_version":1,"battery_capacity_wh":55000}`), &patch); err != nil {
		t.Fatal(err)
	}
	newView, err := service.UpdateVehicle(ctx, p, withCompat.ID, patch)
	if err != nil || newView.Version != 2 || *newView.BatteryCapacityWh != 55000 {
		t.Fatalf("versioned patch: %+v err=%v", newView, err)
	}
	if _, err := service.UpdateVehicle(ctx, p, withCompat.ID, patch); apiErrorCode(err) != "vehicle_version_conflict" {
		t.Fatalf("stale CAS accepted: %v", err)
	}

	var noop UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"expected_version":2,"battery_capacity_wh":55000}`), &noop); err != nil {
		t.Fatal(err)
	}
	same, err := service.UpdateVehicle(ctx, p, withCompat.ID, noop)
	if err != nil || same.Version != 2 {
		t.Fatalf("no-op bumped version: %+v %v", same, err)
	}

	var clear UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"connector_types":null,"supports_ac":null,"max_ac_power_w":null}`), &clear); err != nil {
		t.Fatal(err)
	}
	cleared, err := service.UpdateVehicle(ctx, p, withCompat.ID, clear)
	if err != nil || cleared.Version != 3 || cleared.ConnectorTypes != nil || cleared.SupportsAC != nil || cleared.MaxACPowerW != nil {
		t.Fatalf("clear: %+v %v", cleared, err)
	}

	if _, err := service.CreateVehicle(ctx, p, CreateVehicleRequest{VehicleNumber: "BAD", VehicleCompatibilityFields: VehicleCompatibilityFields{BatteryCapacityWh: vehicleTestI64(-1)}}); apiErrorCode(err) != "invalid_vehicle_battery_capacity_wh" {
		t.Fatalf("bad power accepted: %v", err)
	}
	if err := database.Exec(`UPDATE vehicles SET max_dc_power_w=-1 WHERE id=?`, withCompat.ID).Error; err == nil {
		t.Fatal("SQL negative power accepted")
	}
	if err := database.Exec(`UPDATE vehicles SET supports_ac=false, max_ac_power_w=11 WHERE id=?`, withCompat.ID).Error; err == nil {
		t.Fatal("SQL inconsistent mode accepted")
	}
	var vehicle models.Vehicle
	if err := database.First(&vehicle, "id=? AND cpo_id=? AND customer_id=?", withCompat.ID, p.CPOID, p.CustomerID).Error; err != nil {
		t.Fatal(err)
	}
	if vehicle.Version != 3 || vehicle.ConnectorTypes != nil {
		t.Fatalf("post-rejections altered durable state: %+v", vehicle)
	}
}
