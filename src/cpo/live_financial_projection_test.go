package cpo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestLiveChargingSessionFinancialProjectionUsesImmutableSnapshots(
	t *testing.T,
) {
	started := time.Date(2026, 8, 28, 5, 30, 0, 0, time.UTC)
	consumed := int64(4500)

	view := LiveChargingSessionView{
		SessionID:  uuid.New(),
		StartedAt:  started,
		ConsumedWh: &consumed,
	}

	source := models.ChargingSession{
		ID:           view.SessionID,
		StartTime:    started,
		MeterStartWh: 100000,
		Currency:     "INR",
		TariffSnapshot: models.JSONB{
			"price_per_unit": "10",
			"tariff_type":    "fixed",
			"price_type":     "energy",
			"units":          "kwh",
		},
		TaxSnapshot: models.JSONB{
			"sgst_rate": "9",
			"cgst_rate": "9",
			"igst_rate": "0",
		},
	}

	got, err := projectLiveChargingSessionFinancial(
		view,
		source,
		started.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatalf("projection error = %v", err)
	}

	if got.ProjectedAmount.StringFixed(2) != "53.10" {
		t.Fatalf(
			"projected amount = %s, want 53.10",
			got.ProjectedAmount,
		)
	}

	if got.Currency != "INR" {
		t.Fatalf("currency = %s, want INR", got.Currency)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}

	if payload["projected_amount"] != "53.1" {
		t.Fatalf("unexpected financial JSON: %s", encoded)
	}

	if payload["currency"] != "INR" {
		t.Fatalf("unexpected currency JSON: %s", encoded)
	}
}
