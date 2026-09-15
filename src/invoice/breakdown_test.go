package invoice

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestInvoiceBreakdownFromFrozenSessionSnapshot(t *testing.T) {
	for _, test := range []struct {
		name, total, cgst, sgst, igst string
		amounts                       []string
	}{
		{"split GST", "118", "9", "9", "0", []string{"100.00", "9.00", "9.00", "0.00", "118.00"}},
		{"integrated GST", "118", "0", "0", "18", []string{"100.00", "0.00", "0.00", "18.00", "118.00"}},
		{"zero GST", "100", "0", "0", "0", []string{"100.00", "0.00", "0.00", "0.00", "100.00"}},
		{"free session", "0", "9", "9", "0", []string{"0.00", "0.00", "0.00", "0.00", "0.00"}},
		{"positive rounding", "0.04", "9", "9", "0", []string{"0.03", "0.00", "0.00", "0.00", "0.01", "0.04"}},
		{"negative rounding", "0.07", "9", "9", "0", []string{"0.06", "0.01", "0.01", "0.00", "-0.01", "0.07"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := models.ChargingSession{
				TotalAmount: decimal.RequireFromString(test.total), Currency: "INR", SettlementStatus: "SETTLED",
				TaxSnapshot: models.JSONB{"cgst_rate": test.cgst, "sgst_rate": test.sgst, "igst_rate": test.igst},
			}
			snapshot := buildSnapshot(uuid.New(), session, models.CPO{}, models.Settings{}, time.Now(), "26-27", 1)
			// Exercise the persisted JSON boundary consumed by the PDF worker.
			raw, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var restored issuanceSnapshot
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			rows := invoiceChargeRows(restored.Commercial)
			if len(rows) != len(test.amounts) {
				t.Fatalf("rows = %+v", rows)
			}
			sum := decimal.Zero
			for index, amount := range test.amounts {
				if rows[index].Amount != "INR "+amount {
					t.Fatalf("row %d = %+v, want INR %s", index, rows[index], amount)
				}
				if !rows[index].Total {
					sum = sum.Add(decimal.RequireFromString(amount))
				}
			}
			if !sum.Equal(session.TotalAmount) {
				t.Fatalf("lines sum to %s, settled %s", sum, session.TotalAmount)
			}
			if len(rows) == 6 && rows[4].Description != "Rounding adjustment" {
				t.Fatalf("rounding is not explicit: %+v", rows)
			}
		})
	}
}

func TestInvoiceBreakdownDoesNotInventMissingOrInvalidTax(t *testing.T) {
	for _, tax := range []models.JSONB{
		{}, {"cgst_rate": "9", "sgst_rate": "9"},
		{"cgst_rate": "bad", "sgst_rate": "9", "igst_rate": "0"},
		{"cgst_rate": "9", "sgst_rate": "9", "igst_rate": "18"},
		{"cgst_rate": "-1", "sgst_rate": "0", "igst_rate": "0"},
		{"cgst_rate": "101", "sgst_rate": "0", "igst_rate": "0"},
	} {
		rows := invoiceChargeRows(commercialSnapshot{TotalAmount: "118.00", Currency: "INR", Tax: invoiceTaxProjection(tax)})
		for _, row := range rows {
			if !row.Total && row.Amount != "—" {
				t.Fatalf("invented amount from incomplete/invalid tax: %+v", rows)
			}
		}
		if rows[len(rows)-1].Amount != "INR 118.00" {
			t.Fatalf("lost frozen total: %+v", rows)
		}
	}
}

func TestInvoiceBreakdownAlwaysReconcilesDisplayedAmounts(t *testing.T) {
	for cents := int64(0); cents < 10000; cents++ {
		snapshot := commercialSnapshot{TotalAmount: decimal.New(cents, -2).StringFixed(2), Currency: "INR", Tax: invoiceTaxSnapshot{CGSTRate: stringPointer("9"), SGSTRate: stringPointer("9"), IGSTRate: stringPointer("0")}}
		rows := invoiceChargeRows(snapshot)
		sum := decimal.Zero
		for _, row := range rows[:len(rows)-1] {
			amount, err := decimal.NewFromString(strings.TrimPrefix(row.Amount, "INR "))
			if err != nil {
				t.Fatal(err)
			}
			sum = sum.Add(amount)
		}
		if sum.StringFixed(2) != snapshot.TotalAmount {
			t.Fatalf("sum %s differs from %s: %+v", sum, snapshot.TotalAmount, rows)
		}
	}
}
