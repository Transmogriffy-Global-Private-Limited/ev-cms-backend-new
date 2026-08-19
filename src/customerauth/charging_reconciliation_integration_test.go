package customerauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestChargingStartReconciliationWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var mappingFails atomic.Bool
	var abortStart atomic.Bool
	var lookupStatus atomic.Int32
	var startRequests atomic.Int32
	halServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPut && request.URL.Path != "":
			if mappingFails.Load() {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/remote-commands/start":
			startRequests.Add(1)
			if abortStart.Load() {
				panic(http.ErrAbortHandler)
			}
			var start struct {
				CMSCommandID uuid.UUID `json:"cms_command_id"`
			}
			if err := json.NewDecoder(request.Body).Decode(&start); err != nil {
				t.Fatalf("decode HAL start: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"command": map[string]any{
				"hal_command_id": uuid.New(), "cms_command_id": start.CMSCommandID,
				"kind": "START", "state": "ACCEPTED", "updated_at": time.Now().UTC(),
			}})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/remote-commands":
			status := int(lookupStatus.Load())
			if status != 0 {
				writer.WriteHeader(status)
				return
			}
			commandID, err := uuid.Parse(request.URL.Query().Get("cms_command_id"))
			if err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"command": map[string]any{
				"hal_command_id": uuid.New(), "cms_command_id": commandID,
				"kind": "START", "state": "ACCEPTED", "updated_at": time.Now().UTC(),
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer halServer.Close()

	fixture := newChargingAdmissionFixture(t, gormDB)
	service, err := NewService(gormDB, config.Auth{}, false, nil, nil)
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	operations := halops.New(gormDB, config.HAL{BaseURL: halServer.URL, CMSBearerToken: "test", MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute})
	service.WithHALOperations(operations, liveops.New(gormDB, config.HAL{MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute}), config.HAL{MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute})

	setChargingAdmissionProjection(t, gormDB, fixture, fixture.connector.ID, "ONLINE", "Available", time.Now().UTC())
	before := chargingAdmissionSideEffects(t, gormDB, fixture.cpo.ID)
	mappingFails.Store(true)
	_, err = service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "mapping-failure")
	assertChargingMappingUnavailable(t, err)
	if after := chargingAdmissionSideEffects(t, gormDB, fixture.cpo.ID); after != before || startRequests.Load() != 0 {
		t.Fatalf("mapping prerequisite failure created side effects=%+v (before=%+v) or called start=%d", after, before, startRequests.Load())
	}

	mappingFails.Store(false)
	abortStart.Store(true)
	response, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "ambiguous-start")
	if err != nil || response.Status != constants.StartIntentStatusReconciliation {
		t.Fatalf("ambiguous start response=%+v err=%v, want reconciliation", response, err)
	}
	intent, command, hold := loadStartAttempt(t, gormDB, response.StartIntentID)
	if intent.Status != constants.StartIntentStatusReconciliation || command.State != "RECONCILIATION_REQUIRED" || hold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("ambiguous start state intent=%s command=%s hold=%s", intent.Status, command.State, hold.Status)
	}
	if err := gormDB.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", command.CMSCommandID).Updates(map[string]any{
		"last_error_category": "provider_http",
		"last_error_detail":   "legacy generic reconciliation detail",
	}).Error; err != nil {
		t.Fatalf("shape historical reconciliation zombie: %v", err)
	}

	lookupStatus.Store(http.StatusInternalServerError)
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile lookup 500: %v", err)
	}
	intent, command, hold = loadStartAttempt(t, gormDB, response.StartIntentID)
	if intent.Status != constants.StartIntentStatusReconciliation || command.State != "RECONCILIATION_REQUIRED" || hold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("lookup 500 changed unresolved start intent=%s command=%s hold=%s", intent.Status, command.State, hold.Status)
	}

	lookupStatus.Store(http.StatusNotFound)
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile exact 404: %v", err)
	}
	intent, command, hold = loadStartAttempt(t, gormDB, response.StartIntentID)
	if intent.Status != constants.StartIntentStatusRejected || command.State != "CONFIRMED_ABSENT" || command.LastErrorCategory != "confirmed_absent" || hold.Status != constants.WalletHoldStatusReleased || hold.ReleasedAt == nil {
		t.Fatalf("exact 404 did not terminalize start intent=%s command=%s/%s hold=%s released=%v", intent.Status, command.State, command.LastErrorCategory, hold.Status, hold.ReleasedAt)
	}
	releasedAt := *hold.ReleasedAt
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("repeat exact 404 reconciliation: %v", err)
	}
	_, command, hold = loadStartAttempt(t, gormDB, response.StartIntentID)
	if command.State != "CONFIRMED_ABSENT" || hold.Status != constants.WalletHoldStatusReleased || !hold.ReleasedAt.Equal(releasedAt) {
		t.Fatalf("confirmed absence cleanup was not idempotent command=%s hold=%s released=%v", command.State, hold.Status, hold.ReleasedAt)
	}

	abortStart.Store(false)
	lookupStatus.Store(0)
	retry, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "fresh-retry")
	if err != nil || retry.StartIntentID == response.StartIntentID || retry.Status != constants.StartIntentStatusAcceptedForDelivery {
		t.Fatalf("fresh retry response=%+v err=%v, want a new accepted start", retry, err)
	}
	_, retryCommand, retryHold := loadStartAttempt(t, gormDB, retry.StartIntentID)
	if err := gormDB.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", retryCommand.CMSCommandID).Update("state", "RECONCILIATION_REQUIRED").Error; err != nil {
		t.Fatalf("mark command for exact lookup: %v", err)
	}
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile found command: %v", err)
	}
	_, retryCommand, retryHold = loadStartAttempt(t, gormDB, retry.StartIntentID)
	if retryCommand.State != "ACCEPTED" || retryHold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("exact command lookup did not retain normal start command=%s hold=%s", retryCommand.State, retryHold.Status)
	}

	raceConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, raceConnector.ID, "ONLINE", "Available", time.Now().UTC())
	abortStart.Store(true)
	race, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: raceConnector.ID}, "fact-race")
	if err != nil || race.Status != constants.StartIntentStatusReconciliation {
		t.Fatalf("fact-race start response=%+v err=%v", race, err)
	}
	if err := gormDB.Model(&models.ChargingStartIntent{}).Where("id = ?", race.StartIntentID).Update("materialized_session_id", uuid.New()).Error; err != nil {
		t.Fatalf("simulate materialized start fact: %v", err)
	}
	lookupStatus.Store(http.StatusNotFound)
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile fact-race exact 404: %v", err)
	}
	intent, command, hold = loadStartAttempt(t, gormDB, race.StartIntentID)
	if intent.Status != constants.StartIntentStatusReconciliation || command.State != "RECONCILIATION_REQUIRED" || hold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("fact-race cleanup changed started candidate intent=%s command=%s hold=%s", intent.Status, command.State, hold.Status)
	}

	expiredConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, expiredConnector.ID, "ONLINE", "Available", time.Now().UTC())
	expired, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: expiredConnector.ID}, "expired-absence")
	if err != nil || expired.Status != constants.StartIntentStatusReconciliation {
		t.Fatalf("expired start response=%+v err=%v", expired, err)
	}
	_, expiredCommand, _ := loadStartAttempt(t, gormDB, expired.StartIntentID)
	if err := gormDB.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", expiredCommand.CMSCommandID).Update("command_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatalf("expire command: %v", err)
	}
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile expired exact 404: %v", err)
	}
	intent, command, hold = loadStartAttempt(t, gormDB, expired.StartIntentID)
	if intent.Status != constants.StartIntentStatusExpired || command.State != "CONFIRMED_ABSENT" || hold.Status != constants.WalletHoldStatusReleased {
		t.Fatalf("expired exact absence intent=%s command=%s hold=%s", intent.Status, command.State, hold.Status)
	}

	otherConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, otherConnector.ID, "ONLINE", "Available", time.Now().UTC())
	otherPending, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: otherConnector.ID}, "other-customer-absence")
	if err != nil || otherPending.Status != constants.StartIntentStatusReconciliation {
		t.Fatalf("other-customer pending start response=%+v err=%v", otherPending, err)
	}
	if err := operations.ReconcilePending(ctx, 10); err != nil {
		t.Fatalf("reconcile other-customer exact 404: %v", err)
	}
	abortStart.Store(false)
	lookupStatus.Store(0)
	otherFresh, err := service.StartCharging(ctx, fixture.secondPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: otherConnector.ID}, "other-customer-fresh")
	if err != nil || otherFresh.Status != constants.StartIntentStatusAcceptedForDelivery {
		t.Fatalf("other customer remained blocked after confirmed absence response=%+v err=%v", otherFresh, err)
	}
}

func assertChargingMappingUnavailable(t *testing.T, err error) {
	t.Helper()
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusServiceUnavailable || apiError.Code != "charger_mapping_unavailable" {
		t.Fatalf("mapping error=%v, want 503 charger_mapping_unavailable", err)
	}
}

func loadStartAttempt(t *testing.T, database *gorm.DB, intentID uuid.UUID) (models.ChargingStartIntent, models.HALCommandRecord, models.WalletHold) {
	t.Helper()
	var intent models.ChargingStartIntent
	if err := database.First(&intent, "id = ?", intentID).Error; err != nil {
		t.Fatalf("load start intent: %v", err)
	}
	var command models.HALCommandRecord
	if err := database.First(&command, "start_intent_id = ?", intentID).Error; err != nil {
		t.Fatalf("load start command: %v", err)
	}
	var hold models.WalletHold
	if err := database.First(&hold, "start_intent_id = ?", intentID).Error; err != nil {
		t.Fatalf("load wallet hold: %v", err)
	}
	return intent, command, hold
}
