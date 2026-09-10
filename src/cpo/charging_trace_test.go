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

func TestClassifyChargerOperationFollowOnIsTemporalDiagnosticOnly(t *testing.T) {
	acceptedAt := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	operation := models.ChargerOperation{
		Kind:        "TRIGGER_MESSAGE",
		State:       "OCPP_CONFIRMED",
		OCPPResult:  "Accepted",
		Parameters:  models.JSONB{"requested_message": "StatusNotification"},
		CompletedAt: &acceptedAt,
	}
	root := models.ChargingTrace{ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}
	valid := models.ChargingTraceEvent{
		Category:   "CHARGER_OPERATION_FOLLOW_ON",
		OccurredAt: acceptedAt.Add(30 * time.Second),
		Data:       models.JSONB{"expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "charger-01", "connector_number": 2},
	}

	tests := []struct {
		name      string
		operation models.ChargerOperation
		rows      []models.ChargingTraceEvent
		now       time.Time
		want      string
	}{
		{name: "pending before deadline", operation: operation, now: acceptedAt.Add(59 * time.Second), want: "PENDING"},
		{name: "observed matching evidence", operation: operation, rows: []models.ChargingTraceEvent{valid}, now: acceptedAt.Add(61 * time.Second), want: "OBSERVED"},
		{name: "not observed after deadline", operation: operation, now: acceptedAt.Add(time.Minute), want: "NOT_OBSERVED"},
		{name: "wrong connector ignored", operation: operation, rows: []models.ChargingTraceEvent{{Category: valid.Category, OccurredAt: valid.OccurredAt, Data: models.JSONB{"expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "charger-01", "connector_number": 1}}}, now: acceptedAt.Add(61 * time.Second), want: "NOT_OBSERVED"},
		{name: "wrong action ignored", operation: operation, rows: []models.ChargingTraceEvent{{Category: valid.Category, OccurredAt: valid.OccurredAt, Data: models.JSONB{"expected_message": "Heartbeat", "observed_action": "Heartbeat", "charger_ocpp_identity": "charger-01"}}}, now: acceptedAt.Add(61 * time.Second), want: "NOT_OBSERVED"},
		{name: "wrong identity ignored", operation: operation, rows: []models.ChargingTraceEvent{{Category: valid.Category, OccurredAt: valid.OccurredAt, Data: models.JSONB{"expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "other-charger", "connector_number": 2}}}, now: acceptedAt.Add(61 * time.Second), want: "NOT_OBSERVED"},
		{name: "outside window ignored", operation: operation, rows: []models.ChargingTraceEvent{{Category: valid.Category, OccurredAt: acceptedAt.Add(time.Minute + time.Nanosecond), Data: valid.Data}}, now: acceptedAt.Add(time.Minute + time.Second), want: "NOT_OBSERVED"},
		{name: "not accepted is not applicable", operation: models.ChargerOperation{Kind: "TRIGGER_MESSAGE", State: "OCPP_CONFIRMED", OCPPResult: "Rejected", Parameters: operation.Parameters, CompletedAt: &acceptedAt}, rows: []models.ChargingTraceEvent{valid}, now: acceptedAt.Add(time.Second), want: "NOT_APPLICABLE"},
		{name: "non trigger is not applicable", operation: models.ChargerOperation{Kind: "RESET", State: "OCPP_CONFIRMED", OCPPResult: "Accepted", CompletedAt: &acceptedAt}, rows: []models.ChargingTraceEvent{valid}, now: acceptedAt.Add(time.Second), want: "NOT_APPLICABLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.operation
			got := classifyChargerOperationFollowOn(test.operation, root, test.rows, test.now)
			if got.Status != test.want {
				t.Fatalf("status = %s, want %s", got.Status, test.want)
			}
			if got.Status == "OBSERVED" && (got.ObservedAt == nil || got.ObservedAction != "StatusNotification") {
				t.Fatalf("observed result = %+v", got)
			}
			if before.State != test.operation.State || before.OCPPResult != test.operation.OCPPResult || before.CompletedAt != test.operation.CompletedAt {
				t.Fatalf("diagnostic classification mutated operation: before=%+v after=%+v", before, test.operation)
			}
		})
	}
}
