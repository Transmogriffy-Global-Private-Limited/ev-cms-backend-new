package customerauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
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
	var zeroLookupIdentity atomic.Bool
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
				"kind": "START", "state": "OCPP_ACCEPTED", "updated_at": time.Now().UTC(),
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
			halCommandID := uuid.New()
			if zeroLookupIdentity.Load() {
				halCommandID = uuid.Nil
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"command": map[string]any{
				"hal_command_id": halCommandID, "cms_command_id": commandID,
				"kind": "START", "state": "OCPP_ACCEPTED", "updated_at": time.Now().UTC(),
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
	if intent.Status != constants.StartIntentStatusReconciliation || command.State != "RECONCILIATION_REQUIRED" || command.HALCommandID != nil || hold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("ambiguous start state intent=%s command=%s hal_command_id=%v hold=%s", intent.Status, command.State, command.HALCommandID, hold.Status)
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
	if retryCommand.State != "OCPP_ACCEPTED" || retryHold.Status != constants.WalletHoldStatusHeld {
		t.Fatalf("exact command lookup did not retain normal start command=%s hold=%s", retryCommand.State, retryHold.Status)
	}
	if retryCommand.HALCommandID == nil || *retryCommand.HALCommandID == uuid.Nil {
		t.Fatalf("normal command did not retain HAL identity: %+v", retryCommand)
	}
	originalHALCommandID := *retryCommand.HALCommandID
	zeroLookupIdentity.Store(true)
	if _, err := operations.ReconcileCommand(ctx, retryCommand.CMSCommandID); !errors.Is(err, halclient.ErrInvalidCommandResponse) {
		t.Fatalf("zero HAL command lookup error=%v", err)
	}
	zeroLookupIdentity.Store(false)
	_, retryCommand, _ = loadStartAttempt(t, gormDB, retry.StartIntentID)
	if retryCommand.HALCommandID == nil || *retryCommand.HALCommandID != originalHALCommandID {
		t.Fatalf("zero HAL command lookup changed persisted identity: %+v", retryCommand)
	}

	// A malformed historical synchronous response may have persisted uuid.Nil.
	// Authoritative HAL start truth must repair it rather than strand the user.
	zero := uuid.Nil
	if err := gormDB.Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", retryCommand.CMSCommandID).Updates(map[string]any{"hal_command_id": zero, "state": "RECONCILIATION_REQUIRED"}).Error; err != nil {
		t.Fatalf("shape zero HAL command identity: %v", err)
	}
	if err := gormDB.Model(&models.ChargingStartIntent{}).Where("id = ?", retry.StartIntentID).Updates(map[string]any{"hal_command_id": zero, "status": constants.StartIntentStatusReconciliation}).Error; err != nil {
		t.Fatalf("shape zero start-intent HAL command identity: %v", err)
	}
	var root models.ChargingTrace
	if err := gormDB.First(&root, "trace_id = ?", retry.TraceID).Error; err != nil {
		t.Fatalf("load initial trace root: %v", err)
	}
	if root.CMSStartIntentID == nil || *root.CMSStartIntentID != retry.StartIntentID || root.CMSCommandID == nil || *root.CMSCommandID != retryCommand.CMSCommandID || root.CMSChargingSessionID != nil || root.HALTransactionID != nil || root.OCPPTransactionID != nil {
		t.Fatalf("durable command root linkage=%+v", root)
	}
	evidence := halops.StartEvidence{HALTransactionID: uuid.New(), HALCommandID: uuid.New(), CMSCommandID: retryCommand.CMSCommandID, CMSStartIntentID: retry.StartIntentID, CPOID: fixture.cpo.ID, CMSChargerID: fixture.charger.ID, CMSConnectorID: fixture.connector.ID, ChargerOCPPIdentity: fixture.charger.OCPPIdentity, OCPPConnectorNumber: fixture.connector.ConnectorNumber, OCPPTransactionID: 81, MeterStartWh: 100, ActualStartedAt: time.Now().UTC()}
	if err := service.MaterializeAuthoritativeStart(ctx, evidence); err != nil {
		t.Fatalf("materialize authoritative start over zero identity: %v", err)
	}
	if err := service.MaterializeAuthoritativeStart(ctx, evidence); err != nil {
		t.Fatalf("repeat authoritative start materialization: %v", err)
	}
	intent, command, _ = loadStartAttempt(t, gormDB, retry.StartIntentID)
	if intent.Status != constants.StartIntentStatusActuallyStarted || intent.HALCommandID == nil || *intent.HALCommandID != evidence.HALCommandID || command.State != "MATERIALIZED" || command.HALCommandID == nil || *command.HALCommandID != evidence.HALCommandID {
		t.Fatalf("zero identity was not repaired intent=%+v command=%+v", intent, command)
	}
	if err := gormDB.First(&root, "trace_id = ?", retry.TraceID).Error; err != nil {
		t.Fatalf("load materialized trace root: %v", err)
	}
	if root.CMSChargingSessionID == nil || root.HALTransactionID == nil || root.OCPPTransactionID == nil || *root.HALTransactionID != evidence.HALTransactionID || *root.OCPPTransactionID != evidence.OCPPTransactionID {
		t.Fatalf("materialized root linkage=%+v", root)
	}
	if err := gormDB.Transaction(func(tx *gorm.DB) error {
		return service.recordChargingTraceWithRoot(tx, retry.TraceID, fixture.cpo.ID, chargingTraceRoot{}, "CMS", "CMS", "DIAGNOSTIC", "POSTGRES", "CHARGING", "Later trace event", "", models.JSONB{})
	}); err != nil {
		t.Fatalf("append later trace event: %v", err)
	}
	var afterLaterEvent models.ChargingTrace
	if err := gormDB.First(&afterLaterEvent, "trace_id = ?", retry.TraceID).Error; err != nil {
		t.Fatalf("load root after later event: %v", err)
	}
	if afterLaterEvent.CMSStartIntentID == nil || *afterLaterEvent.CMSStartIntentID != retry.StartIntentID || afterLaterEvent.CMSCommandID == nil || *afterLaterEvent.CMSCommandID != retryCommand.CMSCommandID || afterLaterEvent.CMSChargingSessionID == nil || *afterLaterEvent.CMSChargingSessionID != *root.CMSChargingSessionID || afterLaterEvent.HALTransactionID == nil || *afterLaterEvent.HALTransactionID != evidence.HALTransactionID || afterLaterEvent.OCPPTransactionID == nil || *afterLaterEvent.OCPPTransactionID != evidence.OCPPTransactionID {
		t.Fatalf("later trace event erased root linkage=%+v", afterLaterEvent)
	}
	eventFailureTraceID, eventFailureIntentID, eventFailureCommandID := uuid.New(), uuid.New(), uuid.New()
	var eventFailureErr error
	if err := gormDB.Transaction(func(tx *gorm.DB) error {
		eventFailureErr = service.recordChargingTraceWithRoot(tx, eventFailureTraceID, fixture.cpo.ID, chargingTraceRoot{StartIntentID: &eventFailureIntentID, CommandID: &eventFailureCommandID}, "CMS", "CMS", "DIAGNOSTIC", "POSTGRES", "STARTING", strings.Repeat("x", 201), "", models.JSONB{})
		return nil // The caller intentionally isolates diagnostics from business state.
	}); err != nil {
		t.Fatalf("commit diagnostic-isolated root enrichment: %v", err)
	}
	if eventFailureErr == nil {
		t.Fatal("expected oversized diagnostic event to fail")
	}
	var eventFailureRoot models.ChargingTrace
	if err := gormDB.First(&eventFailureRoot, "trace_id = ?", eventFailureTraceID).Error; err != nil || eventFailureRoot.CMSStartIntentID == nil || *eventFailureRoot.CMSStartIntentID != eventFailureIntentID || eventFailureRoot.CMSCommandID == nil || *eventFailureRoot.CMSCommandID != eventFailureCommandID {
		t.Fatalf("event failure rolled back successful root enrichment: root=%+v err=%v", eventFailureRoot, err)
	}
	conflictingEvidence := evidence
	conflictingEvidence.HALCommandID = uuid.New()
	if err := service.MaterializeAuthoritativeStart(ctx, conflictingEvidence); err == nil {
		t.Fatal("different nonzero authoritative command identity was accepted")
	} else {
		var projectionError *halops.FactProjectionError
		if !errors.As(err, &projectionError) || projectionError.Code != "hal_start_evidence_conflict" {
			t.Fatalf("conflicting identity error=%v", err)
		}
	}

	traceFailureConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, traceFailureConnector.ID, "ONLINE", "Available", time.Now().UTC())
	traceFailureStart, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: traceFailureConnector.ID}, "trace-enrichment-failure")
	if err != nil || traceFailureStart.Status != constants.StartIntentStatusAcceptedForDelivery {
		t.Fatalf("trace failure start response=%+v err=%v", traceFailureStart, err)
	}
	traceFailureIntent, traceFailureCommand, _ := loadStartAttempt(t, gormDB, traceFailureStart.StartIntentID)
	if traceFailureCommand.HALCommandID == nil {
		t.Fatalf("trace failure start did not retain HAL command identity: %+v", traceFailureCommand)
	}
	conflictingTraceCommandID := uuid.New()
	if err := gormDB.Model(&models.ChargingTrace{}).Where("trace_id = ?", traceFailureStart.TraceID).Update("cms_command_id", conflictingTraceCommandID).Error; err != nil {
		t.Fatalf("inject trace root integrity failure: %v", err)
	}
	traceFailureEvidence := halops.StartEvidence{HALTransactionID: uuid.New(), HALCommandID: *traceFailureCommand.HALCommandID, CMSCommandID: traceFailureCommand.CMSCommandID, CMSStartIntentID: traceFailureIntent.ID, CPOID: fixture.cpo.ID, CMSChargerID: fixture.charger.ID, CMSConnectorID: traceFailureConnector.ID, ChargerOCPPIdentity: fixture.charger.OCPPIdentity, OCPPConnectorNumber: traceFailureConnector.ConnectorNumber, OCPPTransactionID: 82, MeterStartWh: 100, ActualStartedAt: time.Now().UTC()}
	if err := service.MaterializeAuthoritativeStart(ctx, traceFailureEvidence); err != nil {
		t.Fatalf("diagnostic root conflict rolled back authoritative start: %v", err)
	}
	var traceFailureSession models.ChargingSession
	if err := gormDB.First(&traceFailureSession, "start_intent_id = ?", traceFailureStart.StartIntentID).Error; err != nil || traceFailureSession.TransactionID != traceFailureEvidence.OCPPTransactionID {
		t.Fatalf("authoritative session missing after trace enrichment failure: session=%+v err=%v", traceFailureSession, err)
	}
	var traceFailureRoot models.ChargingTrace
	if err := gormDB.First(&traceFailureRoot, "trace_id = ?", traceFailureStart.TraceID).Error; err != nil || traceFailureRoot.CMSCommandID == nil || *traceFailureRoot.CMSCommandID != conflictingTraceCommandID {
		t.Fatalf("trace root conflict was overwritten after diagnostic failure: root=%+v err=%v", traceFailureRoot, err)
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
