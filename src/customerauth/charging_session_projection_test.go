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

func TestCustomerSessionChargerHydrationUsesMatchingTenantConnectorRelation(t *testing.T) {
	t.Parallel()

	cpoID, chargerID, connectorID := uuid.New(), uuid.New(), uuid.New()
	session := models.ChargingSession{
		ID:        uuid.New(),
		CPOID:     cpoID,
		ChargerID: chargerID,
		Connector: models.Connector{
			ID: connectorID, CPOID: cpoID, ChargerID: chargerID,
			ConnectorNumber: 1, ConnectorType: "CCS2",
			Charger: models.Charger{ID: chargerID, CPOID: cpoID, ChargerID: "cp0001", ChargerName: "Salt Lake DC"},
		},
	}

	hydrateCustomerSessionCharger(cpoID, &session)
	if session.Charger.ID != chargerID || session.Charger.ChargerID != "cp0001" || session.Charger.ChargerName != "Salt Lake DC" {
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

func TestCustomerSessionChargerHydrationRejectsCrossTenantOrMismatchedRelation(t *testing.T) {
	t.Parallel()

	cpoID, actualChargerID := uuid.New(), uuid.New()
	session := models.ChargingSession{
		CPOID:     cpoID,
		ChargerID: actualChargerID,
		Connector: models.Connector{
			CPOID: cpoID, ChargerID: actualChargerID,
			Charger: models.Charger{ID: actualChargerID, CPOID: uuid.New(), ChargerID: "other1", ChargerName: "Other CPO"},
		},
	}
	hydrateCustomerSessionCharger(cpoID, &session)
	if session.Charger.ID != uuid.Nil {
		t.Fatalf("cross-CPO connector charger was accepted: %+v", session.Charger)
	}

	session.Connector.Charger = models.Charger{ID: uuid.New(), CPOID: cpoID, ChargerID: "wrong1", ChargerName: "Wrong relation"}
	hydrateCustomerSessionCharger(cpoID, &session)
	if session.Charger.ID != uuid.Nil {
		t.Fatalf("mismatched connector charger was accepted: %+v", session.Charger)
	}
}

func TestCustomerSessionChargerHydrationDoesNotChangeLiveSessionState(t *testing.T) {
	t.Parallel()

	// This guards the projection-only fallback: it restores read context but
	// never changes the durable session state or invents an active session.
	cpoID, chargerID := uuid.New(), uuid.New()
	session := models.ChargingSession{CPOID: cpoID, ChargerID: chargerID, Status: constants.SessionStatusActive, Connector: models.Connector{CPOID: cpoID, ChargerID: chargerID, Charger: models.Charger{ID: chargerID, CPOID: cpoID}}}
	hydrateCustomerSessionCharger(cpoID, &session)
	if session.Status != constants.SessionStatusActive || session.ChargerID != chargerID {
		t.Fatalf("projection hydration changed durable session state: %+v", session)
	}
}
