package customerauth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestAffordableChargingLimitUsesExactIntegerWh(t *testing.T) {
	t.Parallel()
	tariff := models.Tariff{PricePerKWh: decimal.RequireFromString("10.0000")}
	hold, limit, err := affordableChargingLimit(decimal.RequireFromString("12.34"), tariff)
	if err != nil {
		t.Fatal(err)
	}
	if limit != 1234 || !hold.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("hold=%s limit=%d, want 12.34/1234", hold, limit)
	}
}

func TestChargingCredentialIsShortOpaqueAndStoredAsHash(t *testing.T) {
	t.Parallel()
	credential, hash, err := newChargingCredential()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(credential, "appv1_") || len(credential) > 20 || len(hash) != 64 || strings.Contains(hash, credential) {
		t.Fatalf("invalid credential/hash shape credential=%q hash=%q", credential, hash)
	}
}

func TestCanonicalFactDigestIsStableAndDetectsPayloadChanges(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("c0c7ed9f-5f39-4f7f-9a3d-bd95be4d6eaf")
	makeEnvelope := func(payload string) HALFactEnvelope {
		return HALFactEnvelope{FactID: id, FactType: "transaction.meter", SchemaVersion: 1, OccurredAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC), Producer: "ocpp-hal-go-new", Payload: json.RawMessage(payload)}
	}
	first, err := canonicalFactDigest(makeEnvelope(`{"z":7,"a":{"value_wh":42}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalFactDigest(makeEnvelope(`{"a":{"value_wh":42},"z":7}`))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := canonicalFactDigest(makeEnvelope(`{"a":{"value_wh":43},"z":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first == changed {
		t.Fatalf("digest stability/integrity failed first=%s second=%s changed=%s", first, second, changed)
	}
}
