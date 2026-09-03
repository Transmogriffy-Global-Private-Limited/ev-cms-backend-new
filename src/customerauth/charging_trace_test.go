package customerauth

import (
	"errors"
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

func TestChargingTraceRootBeginsWithOnlyKnownStartIdentity(t *testing.T) {
	intentID, traceID, cpoID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	root := chargingTraceRootModel(traceID, cpoID, chargingTraceRoot{StartIntentID: &intentID, ChargerOCPPIdentity: " charger-01 ", OCPPConnectorNumber: 2}, now)
	if root.TraceID != traceID || root.CPOID != cpoID || root.CMSStartIntentID == nil || *root.CMSStartIntentID != intentID || root.ChargerOCPPIdentity != "charger-01" || root.OCPPConnectorNumber != 2 {
		t.Fatalf("initial root=%+v", root)
	}
	if root.CMSCommandID != nil || root.CMSChargingSessionID != nil || root.HALTransactionID != nil || root.OCPPTransactionID != nil {
		t.Fatalf("initial root invented later identities: %+v", root)
	}
}

func TestChargingTraceRootEnrichmentBindsCommandThenMaterializedSession(t *testing.T) {
	intentID, commandID, sessionID, halTransactionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ocppTransactionID := int64(2131687302)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	root := models.ChargingTrace{CMSStartIntentID: &intentID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}

	updates, err := chargingTraceRootUpdates(root, chargingTraceRoot{StartIntentID: &intentID, CommandID: &commandID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}, now)
	if err != nil || updates["cms_command_id"] != commandID {
		t.Fatalf("command enrichment updates=%#v err=%v", updates, err)
	}
	root.CMSCommandID = &commandID
	updates, err = chargingTraceRootUpdates(root, chargingTraceRoot{StartIntentID: &intentID, SessionID: &sessionID, CommandID: &commandID, HALTransactionID: &halTransactionID, OCPPTransactionID: &ocppTransactionID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}, now)
	if err != nil || updates["cms_charging_session_id"] != sessionID || updates["hal_transaction_id"] != halTransactionID || updates["ocpp_transaction_id"] != ocppTransactionID {
		t.Fatalf("materialization enrichment updates=%#v err=%v", updates, err)
	}
	if _, exists := updates["cms_command_id"]; exists {
		t.Fatalf("same command identity was unexpectedly rewritten: %#v", updates)
	}
}

func TestChargingTraceRootMaterializationCanBackfillMissingCommand(t *testing.T) {
	intentID, commandID, sessionID, halTransactionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ocppTransactionID := int64(2131687302)
	updates, err := chargingTraceRootUpdates(models.ChargingTrace{CMSStartIntentID: &intentID}, chargingTraceRoot{StartIntentID: &intentID, SessionID: &sessionID, CommandID: &commandID, HALTransactionID: &halTransactionID, OCPPTransactionID: &ocppTransactionID}, time.Now().UTC())
	if err != nil || updates["cms_command_id"] != commandID || updates["cms_charging_session_id"] != sessionID || updates["hal_transaction_id"] != halTransactionID || updates["ocpp_transaction_id"] != ocppTransactionID {
		t.Fatalf("materialization did not backfill complete root linkage: updates=%#v err=%v", updates, err)
	}
}

func TestChargingTraceRootEnrichmentIsIdempotentAndRejectsConflicts(t *testing.T) {
	intentID, commandID, sessionID, halTransactionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ocppTransactionID := int64(2131687302)
	root := models.ChargingTrace{CMSStartIntentID: &intentID, CMSCommandID: &commandID, CMSChargingSessionID: &sessionID, HALTransactionID: &halTransactionID, OCPPTransactionID: &ocppTransactionID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}
	linkage := chargingTraceRoot{StartIntentID: &intentID, CommandID: &commandID, SessionID: &sessionID, HALTransactionID: &halTransactionID, OCPPTransactionID: &ocppTransactionID, ChargerOCPPIdentity: "charger-01", OCPPConnectorNumber: 2}

	updates, err := chargingTraceRootUpdates(root, linkage, time.Now().UTC())
	if err != nil || len(updates) != 0 {
		t.Fatalf("same-value enrichment updates=%#v err=%v", updates, err)
	}
	updates, err = chargingTraceRootUpdates(root, chargingTraceRoot{}, time.Now().UTC())
	if err != nil || len(updates) != 0 {
		t.Fatalf("later trace event erased or changed root linkage: updates=%#v err=%v", updates, err)
	}

	conflictingCommand := uuid.New()
	if _, err := chargingTraceRootUpdates(root, chargingTraceRoot{CommandID: &conflictingCommand}, time.Now().UTC()); !errors.Is(err, errChargingTraceRootIdentityConflict) {
		t.Fatalf("command conflict error=%v", err)
	}
	conflictingOCPP := ocppTransactionID + 1
	if _, err := chargingTraceRootUpdates(root, chargingTraceRoot{OCPPTransactionID: &conflictingOCPP}, time.Now().UTC()); !errors.Is(err, errChargingTraceRootIdentityConflict) {
		t.Fatalf("OCPP conflict error=%v", err)
	}
	if *root.CMSCommandID != commandID || *root.OCPPTransactionID != ocppTransactionID {
		t.Fatalf("conflicting enrichment overwrote root: %+v", root)
	}
}
