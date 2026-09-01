package cpo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func TestChargingTraceEventViewUsesTheDocumentedJSONContract(t *testing.T) {
	encoded, err := json.Marshal(ChargingTraceEventView{
		ID: "11111111-1111-4111-8111-111111111111", TraceID: "22222222-2222-4222-8222-222222222222",
		Source: "HAL", Target: "CMS", Category: "LIFECYCLE", Protocol: "OCPP1.6", Phase: "CHARGING", Summary: "Start accepted",
		OccurredAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), RecordedAt: time.Date(2026, 9, 1, 12, 0, 1, 0, time.UTC),
		Data: models.JSONB{"safe": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, field := range []string{"\"id\"", "\"trace_id\"", "\"occurred_at\"", "\"recorded_at\"", "\"data\""} {
		if !strings.Contains(text, field) {
			t.Fatalf("trace event JSON missing %s: %s", field, text)
		}
	}
	if strings.Contains(text, "\"ID\"") || strings.Contains(text, "\"TraceID\"") {
		t.Fatalf("trace event leaked Go field names: %s", text)
	}
}
