package chargingtrace

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
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
