package liveops

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestConnectionAndMeterFreshnessAreIndependent(t *testing.T) {
	now := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	service := New(nil, config.HAL{MeterStaleAfter: 30 * time.Second, ConnectionStaleAfter: 15 * time.Minute})
	service.now = func() time.Time { return now }

	observed := now.Add(-5 * time.Minute)
	if got := service.connectionFreshness(observed); got != FreshnessFresh {
		t.Fatalf("connection freshness=%q, want %q", got, FreshnessFresh)
	}
	if got := service.meterFreshness(observed); got != FreshnessStale {
		t.Fatalf("meter freshness=%q, want %q", got, FreshnessStale)
	}
}

func TestConnectorFreshnessUsesParentConnectionLiveness(t *testing.T) {
	now := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	service := New(nil, config.HAL{MeterStaleAfter: 30 * time.Second, ConnectionStaleAfter: 15 * time.Minute})
	service.now = func() time.Time { return now }
	connector := models.Connector{ID: uuid.New(), ChargerID: uuid.New(), CPOID: uuid.New()}
	runtime := models.HALConnectorRuntime{CMSConnectorID: connector.ID, OCPPConnectorStatus: "Available", ConnectorStatusSequence: 1, ObservedAt: now.Add(-10 * time.Minute)}

	freshParent := ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: FreshnessFresh}
	state := service.connectorState(connector, runtime, freshParent)
	if state.Availability != "AVAILABLE" || state.Freshness != FreshnessFresh {
		t.Fatalf("fresh parent connector=%#v", state)
	}

	staleParent := ChargerState{ConnectionState: "ONLINE", ConnectionFreshness: FreshnessStale}
	state = service.connectorState(connector, runtime, staleParent)
	if state.Availability != "UNAVAILABLE" || state.Freshness != FreshnessStale {
		t.Fatalf("stale parent connector=%#v", state)
	}
}
