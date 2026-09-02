package customerauth

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestCustomerSessionChargerFallbackUsesPersistedConnectorRelation(t *testing.T) {
	t.Parallel()

	cpoID, chargerID, connectorID := uuid.New(), uuid.New(), uuid.New()
	session := models.ChargingSession{
		ID:        uuid.New(),
		CPOID:     cpoID,
		ChargerID: uuid.Nil,
		Connector: models.Connector{
			ID: connectorID, CPOID: cpoID, ChargerID: chargerID,
			ConnectorNumber: 1, ConnectorType: "CCS2",
		},
	}
	chargers := map[uuid.UUID]models.Charger{
		chargerID: {ID: chargerID, CPOID: cpoID, ChargerID: "cp0001", ChargerName: "Salt Lake DC"},
	}

	assignCustomerSessionChargerFallback(cpoID, &session, chargers)
	if session.Charger.ID != chargerID || session.ChargerID != chargerID || session.Charger.ChargerID != "cp0001" || session.Charger.ChargerName != "Salt Lake DC" {
		t.Fatalf("valid persisted charger relation was not hydrated: %+v", session.Charger)
	}

	history := customerChargingSessionHistoryView(session)
	detail := customerChargingSessionDetailView(session, models.ChargingStartIntent{ID: uuid.New()}, liveops.SessionState{}, liveops.ChargerState{}, liveops.ConnectorState{})
	if history.Charger.ID != chargerID || history.Charger.ChargerID != "cp0001" || history.Charger.Name != "Salt Lake DC" ||
		detail.Charger.ID != chargerID || detail.Charger.ChargerID != "cp0001" || detail.Charger.Name != "Salt Lake DC" {
		t.Fatalf("history/detail charger identity diverged: %+v / %+v", history.Charger, detail.Charger)
	}

	snapshot := CustomerLiveChargingSessionListResponse{Sessions: []ChargingSessionFinancialProjectionView{{ChargingSessionView: detail}}, AsOf: time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)}
	var output bytes.Buffer
	if err := writeProjectionSSE(&output, "live_sessions", 7, snapshot); err != nil {
		t.Fatalf("write customer live-session replacement: %v", err)
	}
	frame := output.String()
	for _, expected := range []string{"event: live_sessions\n", `"id":"` + chargerID.String() + `"`, `"charger_id":"cp0001"`, `"name":"Salt Lake DC"`} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("customer live replacement missing %q: %s", expected, frame)
		}
	}
}

func TestCustomerSessionChargerFallbackPrefersConnectorOverLegacySessionKey(t *testing.T) {
	t.Parallel()

	cpoID, connectorChargerID, legacySessionChargerID := uuid.New(), uuid.New(), uuid.New()
	session := models.ChargingSession{
		CPOID: cpoID, ChargerID: legacySessionChargerID,
		Connector: models.Connector{CPOID: cpoID, ChargerID: connectorChargerID},
	}
	assignCustomerSessionChargerFallback(cpoID, &session, map[uuid.UUID]models.Charger{
		connectorChargerID:     {ID: connectorChargerID, CPOID: cpoID, ChargerID: "actual1", ChargerName: "Actual connector charger"},
		legacySessionChargerID: {ID: legacySessionChargerID, CPOID: cpoID, ChargerID: "legacy1", ChargerName: "Legacy session charger"},
	})
	if session.Charger.ID != connectorChargerID || session.ChargerID != connectorChargerID || session.Charger.ChargerID != "actual1" {
		t.Fatalf("connector charger did not win over stale session key: %+v", session)
	}
}

func TestCustomerSessionChargerFallbackRejectsCrossTenantOrMismatchedRelation(t *testing.T) {
	t.Parallel()

	cpoID, actualChargerID := uuid.New(), uuid.New()
	session := models.ChargingSession{
		CPOID:     cpoID,
		ChargerID: actualChargerID,
		Connector: models.Connector{
			CPOID: cpoID, ChargerID: actualChargerID,
		},
	}
	assignCustomerSessionChargerFallback(cpoID, &session, map[uuid.UUID]models.Charger{actualChargerID: {ID: actualChargerID, CPOID: uuid.New(), ChargerID: "other1", ChargerName: "Other CPO"}})
	if session.Charger.ID != uuid.Nil {
		t.Fatalf("cross-CPO connector charger was accepted: %+v", session.Charger)
	}

	session.Connector.ChargerID = uuid.New()
	assignCustomerSessionChargerFallback(cpoID, &session, map[uuid.UUID]models.Charger{})
	if session.Charger.ID != uuid.Nil {
		t.Fatalf("mismatched connector charger was accepted: %+v", session.Charger)
	}
}

func TestCustomerSessionChargerFallbackDoesNotChangeLiveSessionState(t *testing.T) {
	t.Parallel()

	// This guards the projection-only fallback: it restores read context but
	// never changes the durable session state or invents an active session.
	cpoID, chargerID := uuid.New(), uuid.New()
	session := models.ChargingSession{CPOID: cpoID, ChargerID: chargerID, Status: constants.SessionStatusActive, Connector: models.Connector{CPOID: cpoID, ChargerID: chargerID}}
	assignCustomerSessionChargerFallback(cpoID, &session, map[uuid.UUID]models.Charger{chargerID: {ID: chargerID, CPOID: cpoID}})
	if session.Status != constants.SessionStatusActive || session.ChargerID != chargerID {
		t.Fatalf("projection hydration changed durable session state: %+v", session)
	}
}
