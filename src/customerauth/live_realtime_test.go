package customerauth

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestCustomerLiveSessionProjectionUsesSharedAsOfAndImmutableSnapshots(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC)
	asOf := started.Add(90 * time.Minute)
	consumed := int64(2500)
	view := ChargingSessionView{ConsumedWh: &consumed}

	tax := models.JSONB{"sgst_rate": "0", "cgst_rate": "0", "igst_rate": "0"}
	energy, err := customerChargingSessionProjectedAmount(view, models.ChargingSession{
		StartTime: started, MeterStartWh: 1000, TariffSnapshot: models.JSONB{"price_per_unit": "10", "tariff_type": "fixed", "price_type": "energy", "units": "kwh"}, TaxSnapshot: tax,
	}, asOf)
	if err != nil || energy != "25.00" {
		t.Fatalf("energy projection = %q, %v; want 25.00", energy, err)
	}

	timeBased, err := customerChargingSessionProjectedAmount(ChargingSessionView{}, models.ChargingSession{
		StartTime: started, TariffSnapshot: models.JSONB{"price_per_unit": "2", "tariff_type": "fixed", "price_type": "time", "units": "minutes"}, TaxSnapshot: tax,
	}, asOf)
	if err != nil || timeBased != "180.00" {
		t.Fatalf("time projection = %q, %v; want 180.00", timeBased, err)
	}
}

func TestCustomerLiveSessionSnapshotIsFullReplacementAndFingerprintIgnoresOnlyAsOf(t *testing.T) {
	t.Parallel()
	chargerID, connectorID := uuid.New(), uuid.New()
	snapshot := CustomerLiveChargingSessionListResponse{Sessions: []ChargingSessionFinancialProjectionView{{
		ChargingSessionView: ChargingSessionView{ID: uuid.New(), State: string(constants.SessionStatusActive), Charger: ChargingSessionChargerView{ID: chargerID, ChargerID: "8d2bd0", Name: "Forecourt"}, Connector: ChargingSessionConnectorView{ID: connectorID, Number: 1, Type: "CCS2"}},
		ProjectedAmount:     stringPointer("12.50"),
	}}, AsOf: time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)}

	fingerprint, err := customerLiveSessionFingerprint(snapshot)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	later := CustomerLiveChargingSessionListResponse{Sessions: append([]ChargingSessionFinancialProjectionView(nil), snapshot.Sessions...), AsOf: snapshot.AsOf}
	later.Sessions[0].ProjectedAmount = stringPointer(*snapshot.Sessions[0].ProjectedAmount)
	later.AsOf = later.AsOf.Add(time.Minute)
	laterFingerprint, err := customerLiveSessionFingerprint(later)
	if err != nil || !bytes.Equal(fingerprint, laterFingerprint) {
		t.Fatalf("as_of-only refresh changed fingerprint: %v", err)
	}
	later.Sessions[0].ProjectedAmount = stringPointer("13.00")
	changed, err := customerLiveSessionFingerprint(later)
	if err != nil || bytes.Equal(fingerprint, changed) {
		t.Fatalf("client-visible money change did not change fingerprint: %v", err)
	}

	var output bytes.Buffer
	if err := writeProjectionSSE(&output, "live_sessions", 41, snapshot); err != nil {
		t.Fatalf("write projection SSE: %v", err)
	}
	frame := output.String()
	for _, expected := range []string{"id: 41\n", "event: live_sessions\n", `"sessions":[`, `"projected_amount":"12.50"`, `"charger_id":"8d2bd0"`, `"connector":{"id":`, `"as_of":"2026-08-31T12:00:00Z"`} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("frame missing %q: %s", expected, frame)
		}
	}

	chargers, connectors := customerLiveProjectionResourceIDs(snapshot)
	if len(chargers) != 1 || chargers[0] != chargerID || len(connectors) != 1 || connectors[0] != connectorID {
		t.Fatalf("projection resource IDs = %v / %v", chargers, connectors)
	}
}

func TestCustomerChargerAvailabilitySnapshotKeepsEveryConnector(t *testing.T) {
	t.Parallel()
	snapshot := CustomerChargerView{ID: uuid.New(), Connectors: []CustomerConnectorView{{ID: uuid.New(), ConnectorNumber: 1, Availability: "CHARGING"}, {ID: uuid.New(), ConnectorNumber: 2, Availability: "AVAILABLE"}}}
	ids := customerChargerConnectorIDs(snapshot)
	if len(ids) != 2 || ids[0] != snapshot.Connectors[0].ID || ids[1] != snapshot.Connectors[1].ID {
		t.Fatalf("connector IDs = %v", ids)
	}
	var output bytes.Buffer
	if err := writeProjectionSSE(&output, "charger_availability", 52, snapshot); err != nil {
		t.Fatalf("write projection SSE: %v", err)
	}
	frame := output.String()
	for _, expected := range []string{"event: charger_availability\n", `"connector_number":1`, `"availability":"CHARGING"`, `"connector_number":2`, `"availability":"AVAILABLE"`} {
		if !strings.Contains(frame, expected) {
			t.Fatalf("frame missing %q: %s", expected, frame)
		}
	}
}

func stringPointer(value string) *string { return &value }
