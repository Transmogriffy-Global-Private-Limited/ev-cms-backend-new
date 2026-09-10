package chargingtrace

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestOperationEvidenceRejectsRawConfigurationAndRedactsMutationValue(t *testing.T) {
	change := models.JSONB{"unique_id": "op-1", "action": "ChangeConfiguration", "message_type": "CALL", "payload": map[string]any{"key": "MeterValueSampleInterval", "redacted": true}}
	if !validOperationEvidence(change) {
		t.Fatal("expected redacted configuration mutation evidence to be valid")
	}
	safe := sanitize(change)
	payload := safe["payload"].(map[string]any)
	if _, exists := payload["value"]; exists || payload["redacted"] != true {
		t.Fatalf("unsafe mutation projection: %#v", payload)
	}

	raw := models.JSONB{"unique_id": "op-2", "action": "GetConfiguration", "message_type": "CALLRESULT", "payload": map[string]any{"configuration_keys": []any{map[string]any{"key": "AuthorizationKey", "redacted": true, "value": "leak"}}, "unknown_keys": []any{}}}
	if validOperationEvidence(raw) {
		t.Fatal("expected redacted configuration value to be rejected")
	}
}

func TestTriggerMessageFollowOnEvidenceIsStrictlySanitized(t *testing.T) {
	safe := models.JSONB{"expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "charger-01", "connector_number": 2}
	if !validTriggerMessageFollowOnEvidence(safe) {
		t.Fatal("expected bounded follow-on evidence to be valid")
	}
	if got := sanitize(safe); len(got) != 4 || got["connector_number"] != 2 {
		t.Fatalf("safe follow-on projection = %#v", got)
	}
	unsafe := models.JSONB{"expected_message": "StatusNotification", "observed_action": "StatusNotification", "charger_ocpp_identity": "charger-01", "connector_number": 2, "id_tag": "must-not-persist"}
	if validTriggerMessageFollowOnEvidence(unsafe) || len(sanitize(unsafe)) != 0 {
		t.Fatalf("unsafe follow-on evidence was accepted: %#v", sanitize(unsafe))
	}
	operationID, halOperationID := uuid.New(), uuid.New()
	err := validateEnvelope(Envelope{SchemaVersion: 1, TraceID: uuid.New(), EventID: uuid.New(), CPOID: uuid.New(), CMSChargerOperationID: &operationID, HALChargerOperationID: &halOperationID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2, Source: "CHARGER", Target: "HAL", Category: "CHARGER_OPERATION_FOLLOW_ON", Protocol: "OCPP1.6", Phase: "STARTING", Summary: "TriggerMessage follow-on observed", OccurredAt: time.Now().UTC(), Data: unsafe, ImmutableContentSHA256: "0000000000000000000000000000000000000000000000000000000000000000"})
	if err == nil {
		t.Fatal("expected unsafe follow-on envelope to be rejected")
	}
}
