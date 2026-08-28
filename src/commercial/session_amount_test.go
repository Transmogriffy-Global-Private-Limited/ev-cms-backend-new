package commercial

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/shopspring/decimal"
)

func TestSessionAmountFromSnapshotsMatchesBillingDimensions(t *testing.T) {
	started := time.Date(2026, 8, 28, 5, 30, 0, 0, time.UTC)

	tax := models.JSONB{
		"sgst_rate": "9",
		"cgst_rate": "9",
		"igst_rate": "0",
	}

	tests := []struct {
		name       string
		tariff     models.JSONB
		consumedWh int64
		projected  time.Time
		want       string
	}{
		{
			name: "energy",
			tariff: models.JSONB{
				"price_per_unit": "10",
				"tariff_type":    "fixed",
				"price_type":     "energy",
				"units":          "kwh",
			},
			consumedWh: 5000,
			projected:  started.Add(15 * time.Minute),
			want:       "59.00",
		},
		{
			name: "time",
			tariff: models.JSONB{
				"price_per_unit": "6",
				"tariff_type":    "fixed",
				"price_type":     "time",
				"units":          "minutes",
			},
			projected: started.Add(90 * time.Second),
			want:      "10.62",
		},
		{
			name: "session",
			tariff: models.JSONB{
				"price_per_unit": "100",
				"tariff_type":    "fixed",
				"price_type":     "sessions",
			},
			projected: started,
			want:      "118.00",
		},
		{
			name: "legacy per kWh",
			tariff: models.JSONB{
				"price_per_kwh": "10",
			},
			consumedWh: 4500,
			projected:  started.Add(10 * time.Minute),
			want:       "53.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SessionAmountFromSnapshots(
				tt.tariff,
				tax,
				tt.consumedWh,
				started,
				tt.projected,
			)
			if err != nil {
				t.Fatalf("SessionAmountFromSnapshots() error = %v", err)
			}

			want, _ := decimal.NewFromString(tt.want)
			if !got.Equal(want) {
				t.Fatalf("amount = %s, want %s", got, want)
			}
		})
	}
}
