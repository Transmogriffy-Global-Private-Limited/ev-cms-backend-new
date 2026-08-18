package commercial

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/shopspring/decimal"
)

func TestValidateHubGSTRequiresStateAndRateCompatibility(t *testing.T) {
	t.Parallel()
	nine, eighteen, zero := decimal.NewFromInt(9), decimal.NewFromInt(18), decimal.Zero

	for _, tc := range []struct {
		name               string
		hubState, gstState constants.IndianState
		sgst, cgst, igst   *decimal.Decimal
		valid              bool
	}{
		{"same state split tax", "West Bengal", "West Bengal", &nine, &nine, &zero, true},
		{"same state IGST", "West Bengal", "West Bengal", &nine, &nine, &eighteen, false},
		{"interstate IGST", "West Bengal", "Maharashtra", &zero, &zero, &eighteen, true},
		{"interstate split tax", "West Bengal", "Maharashtra", &nine, &nine, &zero, false},
		{"missing component", "West Bengal", "West Bengal", &nine, nil, &zero, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHubGST(tc.hubState, tc.gstState, tc.sgst, tc.cgst, tc.igst)
			if (err == nil) != tc.valid {
				t.Fatalf("ValidateHubGST() error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}
