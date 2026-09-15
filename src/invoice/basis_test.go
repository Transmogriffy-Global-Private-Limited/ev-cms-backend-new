package invoice

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func TestInvoiceBasisTariffAndStopLimitMatrix(t *testing.T) {
	fonts, err := newInvoiceFonts()
	if err != nil {
		t.Fatal(err)
	}
	defer fonts.latin.Destroy()
	defer fonts.bengali.Destroy()
	defer fonts.devanagari.Destroy()
	for _, tariff := range []struct {
		priceType, unit, price, want string
	}{
		{"energy", "kwh", "20.00", "5.250 kWh × INR 20.00/kWh"},
		{"time", "minutes", "2.00", "80 min at INR 2.00/min"},
		{"sessions", "", "50.00", "1 session × INR 50.00/session"},
	} {
		for _, limit := range []string{"AUTO", "ENERGY", "TIME", "MONEY"} {
			t.Run(tariff.priceType+"_"+limit, func(t *testing.T) {
				snapshot := customerPresentationSnapshot()
				raw := models.JSONB{"tariff_type": "fixed", "price_type": tariff.priceType, "price_per_unit": tariff.price}
				if tariff.unit != "" {
					raw["units"] = tariff.unit
				}
				snapshot.Commercial.Tariff = invoiceTariffProjection(raw)
				snapshot.Charging.LimitType = stringPointer(limit)
				// Valid admission limits, deliberately above actual usage: a
				// customer may stop early. MONEY covers even the fixed charge.
				snapshot.Charging.RequestedLimitValue = nil
				switch limit {
				case "ENERGY":
					snapshot.Charging.RequestedLimitValue = stringPointer("10")
				case "TIME":
					snapshot.Charging.RequestedLimitValue = stringPointer("120")
				case "MONEY":
					snapshot.Charging.RequestedLimitValue = stringPointer("250")
				}
				amount, err := commercial.SessionAmountFromSnapshots(raw, models.JSONB{"cgst_rate": "9", "sgst_rate": "9", "igst_rate": "0"}, 5250, snapshot.Charging.StartedAt, *snapshot.Charging.EndedAt)
				if err != nil {
					t.Fatal(err)
				}
				snapshot.Commercial.TotalAmount = amount.StringFixed(2)
				rows := invoiceSessionChargeRows(snapshot)
				if rows[0].Basis != tariff.want {
					t.Fatalf("basis %q, want %q", rows[0].Basis, tariff.want)
				}
				legacy := invoiceChargeRows(snapshot.Commercial)
				if rows[0].Amount != legacy[0].Amount || !reflect.DeepEqual(rows[1:], legacy[1:]) {
					t.Fatal("basis correction changed monetary rows")
				}
				doc, height, err := composeInvoicePageV3(fonts, snapshot, nil, time.UTC)
				if err != nil || height != 297 {
					t.Fatalf("A4 layout: height %v err %v", height, err)
				}
				inspection := &invoicePageInspection{height: height}
				doc.RenderTo(inspection)
				text := strings.Join(inspection.texts, "\n")
				if !strings.Contains(text, tariff.want) || !strings.Contains(text, optionalLimitDescription(snapshot.Charging, "INR")) {
					t.Fatal("PDF composition lost billing basis or independent stop selection")
				}
				if root := os.Getenv("INVOICE_BASIS_SAMPLES"); root != "" {
					pdf, err := renderInvoice(snapshot, rendererVersion, models.InvoiceAsset{}, false, time.UTC)
					if err != nil || bytes.Count(pdf, []byte("/Type/Page/")) != 1 {
						t.Fatalf("render single-page PDF: %v", err)
					}
					if err := os.WriteFile(filepath.Join(root, tariff.priceType+"_"+limit+".pdf"), pdf, 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestInvoiceTimeBasisUsesActualUnroundedTimestamps(t *testing.T) {
	snapshot := customerPresentationSnapshot()
	snapshot.Commercial.Tariff = invoiceTariffProjection(models.JSONB{"tariff_type": "fixed", "price_type": "time", "units": "minutes", "price_per_unit": "2.00"})
	for _, test := range []struct {
		duration time.Duration
		want     string
	}{
		{0, "0 min at INR 2.00/min"},
		{90 * time.Second, "1 min 30 sec at INR 2.00/min"},
		{time.Minute + 125*time.Millisecond, "1 min 0.125 sec at INR 2.00/min"},
	} {
		end := snapshot.Charging.StartedAt.Add(test.duration)
		snapshot.Charging.EndedAt = &end
		// Stale rounded duration must not determine billed elapsed time.
		snapshot.Charging.DurationSeconds = 9999
		if got := invoiceSessionChargeRows(snapshot)[0].Basis; got != test.want {
			t.Fatalf("basis %q, want %q", got, test.want)
		}
	}
	snapshot.Charging.EndedAt = nil
	if got := invoiceSessionChargeRows(snapshot)[0].Basis; got != "Elapsed time unavailable · INR 2.00/min" {
		t.Fatalf("invented duration: %q", got)
	}
}

func TestInvoiceBasisHistoricalAndInvalidTariffs(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  models.JSONB
		want string
	}{
		{"legacy kWh", models.JSONB{"price_per_kwh": "20.00"}, "5.250 kWh × INR 20.00/kWh"},
		{"legacy Wh", models.JSONB{"tariff_type": "fixed", "price_type": "energy", "units": "watt/hour", "price_per_unit": "0.02"}, "5250 Wh × INR 0.02/Wh"},
		{"free session", models.JSONB{"tariff_type": "fixed", "price_type": "sessions", "price_per_unit": "0"}, "1 session × INR 0/session"},
		{"missing price type", models.JSONB{"tariff_type": "fixed", "price_per_unit": "20.00", "units": "kwh"}, "Billing basis unavailable"},
		{"invalid session unit", models.JSONB{"tariff_type": "fixed", "price_type": "sessions", "price_per_unit": "20.00", "units": "kwh"}, "Billing basis unavailable"},
		{"invalid canonical precedence", models.JSONB{"price_per_unit": "bad", "price_per_kwh": "20.00"}, "Billing basis unavailable"},
		{"negative price", models.JSONB{"tariff_type": "fixed", "price_type": "time", "price_per_unit": "-2", "units": "minutes"}, "Billing basis unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := customerPresentationSnapshot()
			snapshot.Commercial.Tariff = invoiceTariffProjection(test.raw)
			if got := invoiceSessionChargeRows(snapshot)[0].Basis; got != test.want {
				t.Fatalf("basis %q, want %q", got, test.want)
			}
		})
	}
}
