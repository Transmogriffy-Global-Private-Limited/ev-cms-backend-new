package cpo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
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

func TestChargingTraceGetResponseUsesPersistedRootIdentities(t *testing.T) {
	traceID, intentID, sessionID, commandID, halTransactionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ocppTransactionID := int64(2131687302)
	response := chargingTraceResponseFromRoot(traceID, models.ChargingTrace{
		TraceID: traceID, CMSStartIntentID: &intentID, CMSChargingSessionID: &sessionID, CMSCommandID: &commandID,
		HALTransactionID: &halTransactionID, OCPPTransactionID: &ocppTransactionID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2,
	})
	if response.TraceID != traceID || response.StartIntentID == nil || *response.StartIntentID != intentID || response.SessionID == nil || *response.SessionID != sessionID || response.CMSCommandID == nil || *response.CMSCommandID != commandID || response.HALTransactionID == nil || *response.HALTransactionID != halTransactionID || response.OCPPTransactionID == nil || *response.OCPPTransactionID != ocppTransactionID {
		t.Fatalf("response did not preserve root identities: %+v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"\"cms_start_intent_id\"", "\"session_id\"", "\"cms_command_id\"", "\"hal_transaction_id\"", "\"ocpp_transaction_id\""} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("trace response JSON missing %s: %s", field, encoded)
		}
	}
}
