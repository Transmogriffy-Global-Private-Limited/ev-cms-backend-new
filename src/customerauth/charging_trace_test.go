package customerauth

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestSanitizedChargingTraceDataExcludesCredentialsAndIdentitySecrets(t *testing.T) {
	data := sanitizedChargingTraceData(models.JSONB{
		"cms_command_id": "safe", "meter_wh": int64(42), "amount": "42.50", "currency": "INR", "wallet_hold_id": "safe-hold", "limit_type": "ENERGY", "id_tag": "must-not-persist", "credential": "must-not-persist", "authorization": "must-not-persist",
	})
	if data["cms_command_id"] != "safe" || data["meter_wh"] != int64(42) || data["amount"] != "42.50" || data["currency"] != "INR" || data["wallet_hold_id"] != "safe-hold" || data["limit_type"] != "ENERGY" {
		t.Fatalf("safe diagnostic fields missing: %#v", data)
	}
	for _, forbidden := range []string{"id_tag", "credential", "authorization"} {
		if _, found := data[forbidden]; found {
			t.Fatalf("trace retained forbidden %s: %#v", forbidden, data)
		}
	}
}

func TestChargingTraceRootUpdatesFillOnlyMissingAuthoritativeLinkage(t *testing.T) {
	intentID, sessionID, commandID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	updates := chargingTraceRootUpdates(models.ChargingTrace{}, chargingTraceRoot{StartIntentID: &intentID, SessionID: &sessionID, CommandID: &commandID}, now)
	if updates["cms_start_intent_id"] != intentID || updates["cms_charging_session_id"] != sessionID || updates["cms_command_id"] != commandID {
		t.Fatalf("missing root linkage updates: %#v", updates)
	}
	root := models.ChargingTrace{CMSStartIntentID: &intentID, CMSChargingSessionID: &sessionID, CMSCommandID: &commandID}
	newIntent, newSession, newCommand := uuid.New(), uuid.New(), uuid.New()
	updates = chargingTraceRootUpdates(root, chargingTraceRoot{StartIntentID: &newIntent, SessionID: &newSession, CommandID: &newCommand}, now)
	for _, key := range []string{"cms_start_intent_id", "cms_charging_session_id", "cms_command_id"} {
		if _, exists := updates[key]; exists {
			t.Fatalf("existing linkage must not be overwritten: %#v", updates)
		}
	}
}
