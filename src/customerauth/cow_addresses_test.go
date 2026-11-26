package customerauth

import (
	"encoding/json"
	"testing"
)

func validCowFields() CowAddressFields {
	return CowAddressFields{Label: " Home ", Line1: "42 Test Street", City: "Kolkata", State: "West Bengal", PostalCode: "700001", CountryCode: "in"}
}
func TestCowAddressNormalizationAndPatching(t *testing.T) {
	f := validCowFields()
	if err := normalizeCowAddress(&f); err != nil {
		t.Fatal(err)
	}
	if f.CountryCode != "IN" || f.Label != "Home" {
		t.Fatalf("normalization: %+v", f)
	}
	if err := patchCowAddress(&f, map[string]json.RawMessage{"line2": json.RawMessage(`"Suite 1"`), "latitude": json.RawMessage(`22.5726`), "longitude": json.RawMessage(`88.3639`)}); err != nil {
		t.Fatal(err)
	}
	if f.Latitude == nil || f.Longitude == nil || *f.Latitude != 22.5726 {
		t.Fatalf("coordinates absent: %+v", f)
	}
	if err := patchCowAddress(&f, map[string]json.RawMessage{"latitude": json.RawMessage(`null`), "longitude": json.RawMessage(`null`), "line2": json.RawMessage(`null`)}); err != nil {
		t.Fatal(err)
	}
	if f.Latitude != nil || f.Longitude != nil || f.Line2 != "" {
		t.Fatal("explicit null did not clear optional fields")
	}
	if err := patchCowAddress(&f, map[string]json.RawMessage{"country_code": json.RawMessage(`null`)}); err == nil {
		t.Fatal("required field cleared")
	}
	if err := patchCowAddress(&f, map[string]json.RawMessage{"latitude": json.RawMessage(`91`)}); err == nil {
		t.Fatal("orphan/out-of-range coordinate accepted")
	}
	if err := patchCowAddress(&f, map[string]json.RawMessage{"is_default": json.RawMessage(`true`)}); err == nil {
		t.Fatal("immutable field changed")
	}
}
func TestCowAddressPatchRejectsUnknownAndMissingVersion(t *testing.T) {
	for _, raw := range []string{
		`{"expected_version":1,"is_default":true}`,
		`{"expected_version":1}`,
		`{"line1":"hello"}`,
		`{"expected_version":0,"line1":"hello"}`,
		`{"expected_version":1,"label":42}`,
	} {
		var patch CowAddressPatch
		if err := json.Unmarshal([]byte(raw), &patch); err == nil {
			if err = patchCowAddress(new(CowAddressFields), patch.Changes); err == nil {
				t.Fatalf("invalid patch accepted: %s", raw)
			}
		}
	}
}
