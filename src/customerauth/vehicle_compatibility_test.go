package customerauth

import (
	"encoding/json"
	"reflect"
	"testing"
)

func vehicleTestI64(n int64) *int64 { return &n }
func vehicleTestBool(b bool) *bool  { return &b }

func TestVehicleCompatibilityUnknownAndNormalization(t *testing.T) {
	unknown, err := normalizeVehicleCompatibility(VehicleCompatibilityFields{})
	if err != nil || unknown.ConnectorTypes != nil || unknown.SupportsAC != nil || unknown.SupportsDC != nil {
		t.Fatalf("unknown must stay unknown: %+v, %v", unknown, err)
	}
	input := []string{" ccs2 ", "Type2"}
	got, err := normalizeVehicleCompatibility(VehicleCompatibilityFields{
		BatteryCapacityWh: vehicleTestI64(50000),
		ConnectorTypes:    &input,
		SupportsAC:        vehicleTestBool(true), SupportsDC: vehicleTestBool(true),
		MaxACPowerW: vehicleTestI64(11000), MaxDCPowerW: vehicleTestI64(60000),
	})
	if err != nil || !reflect.DeepEqual(*got.ConnectorTypes, []string{"CCS2", "TYPE2"}) {
		t.Fatalf("normalized compatibility = %+v, %v", got, err)
	}
	input[0] = "MUTATED"
	if (*got.ConnectorTypes)[0] != "CCS2" {
		t.Fatal("normalized field aliased input")
	}
	for _, bad := range []VehicleCompatibilityFields{
		{BatteryCapacityWh: vehicleTestI64(0)},
		{MaxDCPowerW: vehicleTestI64(-1)},
		{SupportsAC: vehicleTestBool(false), MaxACPowerW: vehicleTestI64(100)},
		{ConnectorTypes: &[]string{}},
		{ConnectorTypes: &[]string{"ccs2", "CCS2"}},
		{ConnectorTypes: &[]string{"hello world"}},
	} {
		if _, err := normalizeVehicleCompatibility(bad); err == nil {
			t.Fatalf("bad compatibility accepted: %+v", bad)
		}
	}
}

func TestVehicleCompatibilityPatchPresenceAndNull(t *testing.T) {
	var p UpdateVehicleRequest
	if err := json.Unmarshal([]byte(`{"battery_capacity_wh":null,"supports_ac":false,"connector_types":["ccs2"],"expected_version":4}`), &p); err != nil {
		t.Fatal(err)
	}
	if !p.batteryCapacityWhSet || !p.supportsACSet || !p.connectorTypesSet || !p.expectedVersionSet || p.ExpectedVersion == nil || *p.ExpectedVersion != 4 {
		t.Fatalf("PATCH field presence failed: %+v", p)
	}
	before := VehicleCompatibilityFields{BatteryCapacityWh: vehicleTestI64(40000)}
	patched, err := patchVehicleCompatibility(before, p)
	if err != nil || patched.BatteryCapacityWh != nil || patched.SupportsAC == nil || *patched.SupportsAC || (*patched.ConnectorTypes)[0] != "CCS2" {
		t.Fatalf("PATCH tuple = %+v err=%v", patched, err)
	}
	if before.BatteryCapacityWh == nil || *before.BatteryCapacityWh != 40000 {
		t.Fatal("PATCH mutated original")
	}
	for _, raw := range []string{`{"expected_version":null}`, `{"max_ac_power_w":"1000"}`, `{"connector_types":[false]}`, `{"cpo_id":"someone"}`} {
		var p UpdateVehicleRequest
		if err := json.Unmarshal([]byte(raw), &p); err == nil {
			// null expected_version is a semantic error checked by the service.
			if raw != `{"expected_version":null}` {
				t.Fatalf("invalid JSON input accepted: %s", raw)
			}
		}
	}
}
