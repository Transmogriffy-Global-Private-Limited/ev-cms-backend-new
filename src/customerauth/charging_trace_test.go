package customerauth

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func TestSanitizedChargingTraceDataExcludesCredentialsAndIdentitySecrets(t *testing.T) {
	data := sanitizedChargingTraceData(models.JSONB{
		"cms_command_id": "safe", "meter_wh": int64(42), "id_tag": "must-not-persist", "credential": "must-not-persist", "authorization": "must-not-persist",
	})
	if data["cms_command_id"] != "safe" || data["meter_wh"] != int64(42) {
		t.Fatalf("safe diagnostic fields missing: %#v", data)
	}
	for _, forbidden := range []string{"id_tag", "credential", "authorization"} {
		if _, found := data[forbidden]; found {
			t.Fatalf("trace retained forbidden %s: %#v", forbidden, data)
		}
	}
}
