package customerauth

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/google/uuid"
)

func TestNormalizeCustomerChargerIDsIsBoundedAndDeterministic(t *testing.T) {
	t.Parallel()
	ids, err := normalizeCustomerChargerIDs([]string{" ABC123,def456 ", "abc123", "ghi789"})
	if err != nil || strings.Join(ids, ",") != "abc123,def456,ghi789" {
		t.Fatalf("normalized IDs=%v err=%v", ids, err)
	}
	if _, err := normalizeCustomerChargerIDs([]string{"not-a-public-id"}); err == nil {
		t.Fatal("malformed public charger ID was accepted")
	}
	tooMany := make([]string, customerChargeabilityMaxChargers+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("a%05d", index)
	}
	if _, err := normalizeCustomerChargerIDs(tooMany); err == nil {
		t.Fatal("more than 100 public charger IDs were accepted")
	}
}

func TestChargeabilityUsesAllowsCMSControlledStartNotAvailabilityLabel(t *testing.T) {
	t.Parallel()
	preparing := "Preparing"
	live := liveops.ConnectorState{ParentConnectionState: "ONLINE", Freshness: liveops.FreshnessFresh, LastOCPPStatus: &preparing, Availability: "CHARGING"}
	if !chargingConnectorAllowsNewStart(live) {
		t.Fatal("fresh Preparing connector must remain eligible for CMS-controlled start")
	}
	if reason := chargeabilityLiveBlocker(live, true); reason != chargeabilityConnectorUnavailable {
		t.Fatalf("non-blocked preparing reason=%q, want only used when start predicate is false", reason)
	}
	faulted := "Faulted"
	live.LastOCPPStatus = &faulted
	if chargingConnectorAllowsNewStart(live) || chargeabilityLiveBlocker(live, true) != chargeabilityConnectorFaulted {
		t.Fatalf("faulted live state was not rejected: %#v", live)
	}
	live.Freshness = liveops.FreshnessStale
	if chargeabilityLiveBlocker(live, true) != chargeabilityConnectorStale {
		t.Fatalf("stale live state was not rejected as stale: %#v", live)
	}
	live.Freshness = liveops.FreshnessUnknown
	if chargeabilityLiveBlocker(live, true) != chargeabilityConnectorStateUnknown {
		t.Fatalf("unknown connector state was presented as a different condition: %#v", live)
	}
	live.Freshness = liveops.FreshnessFresh
	live.ParentConnectionState = "UNKNOWN"
	if chargeabilityLiveBlocker(live, true) != chargeabilityChargerStateUnknown {
		t.Fatalf("unknown parent state was presented as offline: %#v", live)
	}
	live.ParentConnectionState = "OFFLINE"
	if chargeabilityLiveBlocker(live, true) != chargeabilityChargerOffline {
		t.Fatalf("offline parent was not preserved as offline: %#v", live)
	}

	for _, test := range []struct {
		name   string
		detail liveops.ChargerDetail
		found  bool
		want   string
	}{
		{name: "missing", want: chargeabilityChargerStateUnknown},
		{name: "unknown", detail: liveops.ChargerDetail{Charger: liveops.ChargerState{ConnectionState: "UNKNOWN", ConnectionFreshness: liveops.FreshnessUnknown}}, found: true, want: chargeabilityChargerStateUnknown},
		{name: "stale", detail: liveops.ChargerDetail{Charger: liveops.ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: liveops.FreshnessStale}}, found: true, want: chargeabilityChargerStale},
		{name: "offline", detail: liveops.ChargerDetail{Charger: liveops.ChargerState{ConnectionState: "OFFLINE", ConnectionFreshness: liveops.FreshnessFresh}}, found: true, want: chargeabilityChargerOffline},
		{name: "fresh online", detail: liveops.ChargerDetail{Charger: liveops.ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: liveops.FreshnessFresh}}, found: true, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := chargeabilityChargerLiveBlocker(test.detail, test.found); got != test.want {
				t.Fatalf("charger blocker=%q, want %q", got, test.want)
			}
		})
	}
}

func TestChargeabilityAggregatesAnyEligibleConnector(t *testing.T) {
	t.Parallel()
	view := CustomerChargerView{Connectors: []CustomerConnectorView{{ID: uuid.New(), Status: constants.ChargerStatusActive, ChargeabilityReason: chargeabilityConnectorOccupied}, {ID: uuid.New(), Status: constants.ChargerStatusActive, CanCharge: true, ChargeabilityReason: chargeabilityAvailable}}}
	aggregateChargerChargeability(&view)
	if !view.CanCharge || view.ChargeabilityReason != chargeabilityAvailable {
		t.Fatalf("charger aggregate=%+v, want chargeable", view)
	}
	view.Connectors[1].CanCharge = false
	aggregateChargerChargeability(&view)
	if view.CanCharge || view.ChargeabilityReason != chargeabilityNoChargeableConnector {
		t.Fatalf("all-blocked aggregate=%+v", view)
	}
}

func TestCustomerChargeabilitySSEProjectionIsFullReplacement(t *testing.T) {
	t.Parallel()
	snapshot := CustomerChargeabilityResponse{AsOf: time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC), Chargers: []CustomerChargerChargeability{{ChargerID: "abc123", CanCharge: false, ChargeabilityReason: chargeabilityNoChargeableConnector, Connectors: []CustomerConnectorChargeability{{ConnectorID: uuid.New(), ConnectorNumber: 1, CanCharge: false, ChargeabilityReason: chargeabilityConnectorOccupied}}}}}
	fingerprint, err := customerChargeabilityFingerprint(snapshot)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	var output bytes.Buffer
	if err := writeProjectionSSE(&output, "charger_chargeability", 91, snapshot); err != nil {
		t.Fatalf("write SSE: %v", err)
	}
	for _, expected := range []string{"event: charger_chargeability", `"charger_id":"abc123"`, `"can_charge":false`, `"chargeability_reason":"CONNECTOR_OCCUPIED"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("SSE frame missing %q: %s", expected, output.String())
		}
	}
	later := snapshot
	later.AsOf = later.AsOf.Add(time.Minute)
	laterFingerprint, err := customerChargeabilityFingerprint(later)
	if err != nil || !bytes.Equal(fingerprint, laterFingerprint) {
		t.Fatalf("as_of-only refresh changed fingerprint: %v", err)
	}
	later.Chargers[0].Connectors[0].CanCharge = true
	later.Chargers[0].Connectors[0].ChargeabilityReason = chargeabilityAvailable
	changed, err := customerChargeabilityFingerprint(later)
	if err != nil || bytes.Equal(fingerprint, changed) {
		t.Fatalf("semantic chargeability change did not change fingerprint: %v", err)
	}
}
