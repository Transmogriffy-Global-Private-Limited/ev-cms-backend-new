package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type operationRecoveryFixture struct {
	t                  *testing.T
	database           *gorm.DB
	service            *Service
	principal          auth.Principal
	charger, connector uuid.UUID
	identity           string
	mu                 sync.Mutex
	sends, reads       map[uuid.UUID]int
	modes              map[uuid.UUID]string
	requests           map[uuid.UUID]halclient.ChargerOperationRequest
	correlations       map[uuid.UUID]string
}

func newOperationRecoveryFixture(t *testing.T) *operationRecoveryFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	database, sqlDB, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.ApplyMigrations(context.Background(), sqlDB); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cpo := models.CPO{ID: uuid.New(), Slug: "recovery-" + uuid.NewString(), BusinessName: "Recovery CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: uniqueCPOGSTIN(), Address: "1 Test Road", City: "Kolkata", State: constants.WestBengal, Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "test", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	user := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", PasswordHash: "unused-test-hash", FullName: "Recovery Admin", IsActive: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	hub := models.Hub{ID: uuid.New(), CPOID: cpo.ID, Name: "Recovery Hub", Address: "1 Test Road", State: constants.WestBengal, CreatedAt: now, UpdatedAt: now}
	charger := models.Charger{ID: uuid.New(), CPOID: cpo.ID, HubID: &hub.ID, ChargerID: "r1t001", OCPPIdentity: "recovery-" + uuid.NewString(), Status: constants.ChargerStatusActive, ChargerName: "Recovery charger", NumberOfConnectors: 1, CreatedAt: now, UpdatedAt: now}
	connector := models.Connector{ID: uuid.New(), CPOID: cpo.ID, ChargerID: charger.ID, ConnectorNumber: 1, ConnectorType: "CCS2", ConnectorTotalCapacity: 7.4, Status: constants.ChargerStatusActive, CreatedAt: now, UpdatedAt: now}
	mapping := models.HALChargerMapping{CMSChargerID: charger.ID, CPOID: cpo.ID, ChargerOCPPIdentity: charger.OCPPIdentity, SyncState: "SYNCHRONIZED", CreatedAt: now, UpdatedAt: now}
	for _, row := range []any{&cpo, &user, &hub, &charger, &connector, &mapping} {
		if err := database.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	f := &operationRecoveryFixture{t: t, database: database, principal: auth.Principal{UserID: user.ID, Scope: constants.AuthScopeCPO, CPOID: &cpo.ID}, charger: charger.ID, connector: connector.ID, identity: charger.OCPPIdentity, sends: map[uuid.UUID]int{}, reads: map[uuid.UUID]int{}, modes: map[uuid.UUID]string{}, requests: map[uuid.UUID]halclient.ChargerOperationRequest{}, correlations: map[uuid.UUID]string{}}
	server := httptest.NewServer(http.HandlerFunc(f.serveHAL))
	t.Cleanup(server.Close)
	f.service = &Service{database: database, now: func() time.Time { return time.Now().UTC() }, halOperations: halops.New(database, config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: time.Second}), operationalEvents: operationalrealtime.New(database, config.Platform{})}
	t.Cleanup(func() {
		for _, row := range []any{&models.OperationalEvent{}, &models.ChargerOperation{}, &models.HALChargerMapping{}, &models.Connector{}, &models.Charger{}, &models.Hub{}} {
			if err := database.Where("cpo_id = ?", cpo.ID).Delete(row).Error; err != nil {
				t.Error(err)
			}
		}
		if err := database.Delete(&cpo).Error; err != nil {
			t.Error(err)
		}
		if err := database.Delete(&user).Error; err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f *operationRecoveryFixture) serveHAL(w http.ResponseWriter, r *http.Request) {
	var request halclient.ChargerOperationRequest
	var id uuid.UUID
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			f.t.Error(err)
			w.WriteHeader(400)
			return
		}
		id = request.CMSOperationID
		var operation models.ChargerOperation
		if err := f.database.First(&operation, "id = ?", id).Error; err != nil || operation.State != "DELIVERY_ATTEMPTED" || operation.DeliveryAttemptedAt == nil {
			f.t.Errorf("external call before committed attempt: state=%s err=%v", operation.State, err)
		}
		if r.Header.Get("Idempotency-Key") != id.String() {
			f.t.Error("HAL idempotency identity changed")
		}
	} else {
		id, _ = uuid.Parse(r.URL.Query().Get("cms_operation_id"))
	}
	f.mu.Lock()
	if r.Method == http.MethodPost {
		f.sends[id]++
		f.requests[id] = request
		f.correlations[id] = r.Header.Get("X-Correlation-ID")
	} else {
		f.reads[id]++
	}
	mode := f.modes[id]
	sent := f.sends[id]
	original := f.requests[id]
	f.mu.Unlock()
	if r.Method == http.MethodPost && mode == "transport" {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	if r.Method == http.MethodGet && sent == 0 && mode != "known" {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not_found"}`))
		return
	}
	kind := original.Kind
	if kind == "" {
		kind = "CLEAR_CACHE"
	}
	state := "OCPP_CONFIRMED"
	if mode == "accepted" {
		state = "HAL_ACCEPTED"
	}
	if mode == "hal_persisted" {
		state = "PERSISTED"
	}
	if mode == "hal_attempted" {
		state = "DELIVERY_ATTEMPTED"
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": halclient.ChargerOperation{HALOperationID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(id.String())), CMSOperationID: id, Kind: kind, State: state, OCPPResult: "Accepted", UpdatedAt: now, CompletedAt: &now}})
}

func (f *operationRecoveryFixture) persist(parameters models.JSONB) models.ChargerOperation {
	f.t.Helper()
	now := time.Now().UTC()
	number := 0
	row := models.ChargerOperation{ID: uuid.New(), TraceID: uuid.New(), CPOID: *f.principal.CPOID, ChargerID: f.charger, ActorUserID: f.principal.UserID, IdempotencyKey: uuid.NewString(), RequestDigest: strings.Repeat("a", 64), CorrelationID: uuid.NewString(), Kind: "CLEAR_CACHE", Parameters: parameters, State: "PERSISTED", DispatchChargerIdentity: &f.identity, DispatchConnectorNumber: &number, CreatedAt: now, UpdatedAt: now, RecoveryAfter: now}
	if err := f.database.Create(&row).Error; err != nil {
		f.t.Fatal(err)
	}
	return row
}
func (f *operationRecoveryFixture) load(id uuid.UUID) models.ChargerOperation {
	f.t.Helper()
	var row models.ChargerOperation
	if err := f.database.First(&row, "id = ?", id).Error; err != nil {
		f.t.Fatal(err)
	}
	return row
}
func (f *operationRecoveryFixture) due(id uuid.UUID, expireClaim bool) {
	f.t.Helper()
	past := time.Now().Add(-time.Minute)
	values := map[string]any{"recovery_after": past}
	if expireClaim {
		values["dispatch_claim_expires_at"] = past
	}
	if err := f.database.Model(&models.ChargerOperation{}).Where("id = ?", id).Updates(values).Error; err != nil {
		f.t.Fatal(err)
	}
}
func (f *operationRecoveryFixture) counts(id uuid.UUID) (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sends[id], f.reads[id]
}
func (f *operationRecoveryFixture) claim(id uuid.UUID) models.ChargerOperation {
	f.t.Helper()
	row, won, err := f.service.claimChargerOperation(context.Background(), id)
	if err != nil || !won {
		f.t.Fatalf("claim won=%v err=%v", won, err)
	}
	return row
}
func (f *operationRecoveryFixture) mark(row models.ChargerOperation) {
	f.t.Helper()
	won, err := f.service.markChargerOperationAttempt(context.Background(), row)
	if err != nil || !won {
		f.t.Fatalf("mark won=%v err=%v", won, err)
	}
}
func (f *operationRecoveryFixture) recover() {
	f.t.Helper()
	if err := f.service.recoverChargerOperations(context.Background()); err != nil {
		f.t.Fatal(err)
	}
}

func TestChargerOperationRecoveryCrashBoundariesWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	ctx := context.Background()
	t.Run("A persisted survives restart", func(t *testing.T) {
		row := f.persist(nil)
		restarted := *f.service
		f.service = &restarted
		f.recover()
		f.recover()
		if sends, _ := f.counts(row.ID); sends != 1 {
			t.Fatalf("sends=%d", sends)
		}
	})
	t.Run("B I expired claim fences stale owner", func(t *testing.T) {
		row := f.persist(nil)
		old := f.claim(row.ID)
		f.recover()
		if sends, _ := f.counts(row.ID); sends != 0 {
			t.Fatal("stole active claim")
		}
		f.due(row.ID, true)
		fresh := f.claim(row.ID)
		if won, err := f.service.markChargerOperationAttempt(ctx, old); err != nil || won {
			t.Fatalf("stale owner advanced won=%v err=%v", won, err)
		}
		f.due(row.ID, true)
		f.recover()
		if sends, _ := f.counts(row.ID); sends != 1 {
			t.Fatalf("sends=%d fresh=%s", sends, fresh.ID)
		}
	})
	t.Run("C G attempted before send only reconciles absence", func(t *testing.T) {
		row := f.persist(nil)
		f.mark(f.claim(row.ID))
		f.due(row.ID, false)
		f.recover()
		f.recover()
		sends, reads := f.counts(row.ID)
		stored := f.load(row.ID)
		if sends != 0 || reads != 1 || stored.State != "CONFIRMED_ABSENT" || stored.DeliveryAttemptedAt == nil || stored.CompletedAt == nil {
			t.Fatalf("sends=%d reads=%d state=%s", sends, reads, stored.State)
		}
	})
	t.Run("D F HAL received result lost converges by GET", func(t *testing.T) {
		row := f.persist(nil)
		f.mark(f.claim(row.ID))
		request, err := frozenChargerOperationRequest(row)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.halOperations.RequestChargerOperation(ctx, request, row.CorrelationID); err != nil {
			t.Fatal(err)
		}
		f.due(row.ID, false)
		f.recover()
		sends, reads := f.counts(row.ID)
		if sends != 1 || reads != 1 || f.load(row.ID).State != "OCPP_CONFIRMED" {
			t.Fatalf("sends=%d reads=%d", sends, reads)
		}
	})
	t.Run("E transport ambiguity never replays", func(t *testing.T) {
		row := f.persist(nil)
		f.modes[row.ID] = "transport"
		response, err := f.service.dispatchChargerOperation(ctx, row.ID)
		if err != nil || response.State != "RECONCILIATION_REQUIRED" || response.CompletedAt != nil {
			t.Fatalf("state=%s err=%v", response.State, err)
		}
		f.due(row.ID, false)
		f.recover()
		if sends, reads := f.counts(row.ID); sends != 1 || reads != 1 {
			t.Fatalf("sends=%d reads=%d", sends, reads)
		}
	})
	t.Run("accepted remains incomplete and converges", func(t *testing.T) {
		row := f.persist(nil)
		f.modes[row.ID] = "accepted"
		response, err := f.service.dispatchChargerOperation(ctx, row.ID)
		if err != nil || response.State != "HAL_ACCEPTED" || response.CompletedAt != nil {
			t.Fatal("nonterminal HAL response presented completed")
		}
		f.mu.Lock()
		f.modes[row.ID] = ""
		f.mu.Unlock()
		f.due(row.ID, false)
		f.recover()
		if f.load(row.ID).State != "OCPP_CONFIRMED" {
			t.Fatal("accepted did not converge")
		}
	})
	t.Run("H concurrent dispatchers", func(t *testing.T) {
		row := f.persist(nil)
		var group sync.WaitGroup
		for i := 0; i < 12; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				if _, err := f.service.dispatchChargerOperation(ctx, row.ID); err != nil {
					t.Error(err)
				}
			}()
		}
		group.Wait()
		if sends, _ := f.counts(row.ID); sends != 1 {
			t.Fatalf("sends=%d", sends)
		}
	})
	t.Run("J concurrent idempotent requests", func(t *testing.T) {
		input := ChargerOperationInput{Kind: "CLEAR_CACHE", IdempotencyKey: uuid.NewString(), CorrelationID: uuid.NewString(), Parameters: map[string]string{}}
		var group sync.WaitGroup
		ids := make(chan uuid.UUID, 8)
		for i := 0; i < 8; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				response, err := f.service.RequestChargerOperation(ctx, f.principal, f.charger, input)
				if err != nil {
					t.Error(err)
					return
				}
				ids <- response.ID
			}()
		}
		group.Wait()
		close(ids)
		var id uuid.UUID
		for next := range ids {
			if id != uuid.Nil && next != id {
				t.Fatal("multiple idempotency rows")
			}
			id = next
		}
		if sends, _ := f.counts(id); sends != 1 {
			t.Fatalf("sends=%d", sends)
		}
		input.Kind = "RESET"
		if _, err := f.service.RequestChargerOperation(ctx, f.principal, f.charger, input); err == nil {
			t.Fatal("changed digest reused key")
		}
	})
	t.Run("K mixed rows and poison isolation", func(t *testing.T) {
		poison := f.persist(models.JSONB{"invalid": 123})
		queued := f.persist(nil)
		active := f.persist(nil)
		f.claim(active.ID)
		expired := f.persist(nil)
		f.claim(expired.ID)
		f.due(expired.ID, true)
		attempted := f.persist(nil)
		f.mark(f.claim(attempted.ID))
		f.due(attempted.ID, false)
		ambiguous := f.persist(nil)
		f.modes[ambiguous.ID] = "transport"
		if _, err := f.service.dispatchChargerOperation(ctx, ambiguous.ID); err != nil {
			t.Fatal(err)
		}
		f.due(ambiguous.ID, false)
		terminal := f.persist(nil)
		if _, err := f.service.dispatchChargerOperation(ctx, terminal.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.service.recoverChargerOperations(ctx); err == nil {
			t.Fatal("poison failure not surfaced")
		}
		for _, row := range []models.ChargerOperation{queued, expired, ambiguous, terminal} {
			if sends, _ := f.counts(row.ID); sends != 1 {
				t.Fatalf("operation %s sends=%d", row.ID, sends)
			}
		}
		for _, row := range []models.ChargerOperation{poison, active, attempted} {
			if sends, _ := f.counts(row.ID); sends != 0 {
				t.Fatalf("illegal send %s", row.ID)
			}
		}
		if f.load(poison.ID).State != "PERSISTED" || !f.load(poison.ID).RecoveryAfter.After(time.Now()) {
			t.Fatal("poison not backed off")
		}
	})
	t.Run("immutable inputs and attempt evidence", func(t *testing.T) {
		row := f.persist(nil)
		if err := f.database.Model(&row).Update("parameters", models.JSONB{"changed": "value"}).Error; err == nil {
			t.Fatal("mutable dispatch parameters")
		}
		f.mark(f.claim(row.ID))
		if err := f.database.Model(&row).Update("delivery_attempted_at", nil).Error; err == nil {
			t.Fatal("erased attempt marker")
		}
		if err := f.database.Model(&row).Update("state", "PERSISTED").Error; err == nil {
			t.Fatal("requeued attempted operation")
		}
	})
}

func TestChargerOperationFrozenAdmissionWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	ctx := context.Background()
	// Fail before claim after the admission transaction has durably committed.
	if err := f.database.Callback().Update().Before("gorm:update").Register("test:claim_crash", func(tx *gorm.DB) {
		if tx.Statement.Table == "charger_operations" {
			tx.AddError(errors.New("simulated crash before claim"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	input := ChargerOperationInput{Kind: "GET_CONFIGURATION", ConnectorID: &f.connector, ConfigurationKeys: []string{"HeartbeatInterval", "ConnectionTimeOut"}, Parameters: map[string]string{"reason": "frozen"}, IdempotencyKey: uuid.NewString(), CorrelationID: uuid.NewString()}
	_, requestErr := f.service.RequestChargerOperation(ctx, f.principal, f.charger, input)
	_ = f.database.Callback().Update().Remove("test:claim_crash")
	if requestErr == nil {
		t.Fatal("expected crash")
	}
	var row models.ChargerOperation
	if err := f.database.Where("idempotency_key = ?", input.IdempotencyKey).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.State != "PERSISTED" {
		t.Fatal("persist did not survive")
	}
	if err := f.database.Model(&models.HALChargerMapping{}).Where("cms_charger_id = ?", f.charger).Update("charger_ocpp_identity", "changed-"+uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.database.Model(&models.Connector{}).Where("id = ?", f.connector).Update("connector_number", 2).Error; err != nil {
		t.Fatal(err)
	}
	restarted := *f.service
	f.service = &restarted
	f.recover()
	f.mu.Lock()
	request := f.requests[row.ID]
	correlation := f.correlations[row.ID]
	f.mu.Unlock()
	if request.ChargerOCPPIdentity != f.identity || request.OCPPConnectorNumber != 1 || !reflect.DeepEqual(request.ConfigurationKeys, input.ConfigurationKeys) || request.CMSOperationID != row.ID || request.TraceID != row.TraceID || request.CPOID != *f.principal.CPOID || request.CMSChargerID != f.charger || request.CMSConnectorID == nil || *request.CMSConnectorID != f.connector || correlation != input.CorrelationID || request.Parameters["reason"] != "frozen" {
		t.Fatal("frozen HAL request drifted")
	}
	var count int64
	if err := f.database.Model(&models.OperationalEvent{}).Where("cpo_id = ? AND resource_id = ? AND event_type = ?", *f.principal.CPOID, row.ID.String(), "charger.operation_changed").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("persist/claim/attempt/result events=%d", count)
	}
}

func TestChargerOperationAttemptCommitFailureWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	row := f.persist(nil)
	ctx := context.Background()
	// A deferred constraint trigger fails COMMIT, after the UPDATE and its event.
	function := "test_attempt_commit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql := `CREATE FUNCTION ` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='` + row.ID.String() + `'::uuid AND NEW.state='DELIVERY_ATTEMPTED' THEN RAISE EXCEPTION 'injected commit failure'; END IF; RETURN NEW; END $$; CREATE CONSTRAINT TRIGGER ` + function + ` AFTER UPDATE ON charger_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ` + function + `();`
	if err := f.database.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.database.Exec(`DROP TRIGGER ` + function + ` ON charger_operations; DROP FUNCTION ` + function + `();`).Error; err != nil {
			t.Error(err)
		}
	})
	if _, err := f.service.dispatchChargerOperation(ctx, row.ID); err == nil {
		t.Fatal("commit failure ignored")
	}
	if sends, _ := f.counts(row.ID); sends != 0 {
		t.Fatal("HAL called despite failed commit")
	}
	stored := f.load(row.ID)
	if stored.State != "DISPATCH_CLAIMED" || stored.DeliveryAttemptedAt != nil {
		t.Fatal("attempt was not rolled back")
	}
}

func TestChargerOperationRecoveryCancellationWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.service.RunChargerOperationRecovery(ctx, nil, "test") }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestChargerOperationHALQueueStatesAndStaleReconciliationWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	ctx := context.Background()
	for _, mode := range []string{"hal_persisted", "hal_attempted"} {
		row := f.persist(nil)
		f.modes[row.ID] = mode
		response, err := f.service.dispatchChargerOperation(ctx, row.ID)
		if err != nil || response.State != "HAL_ACCEPTED" || response.HALOperationID == nil || response.CompletedAt != nil {
			t.Fatalf("HAL queue state mishandled mode=%s state=%s err=%v", mode, response.State, err)
		}
		f.due(row.ID, false)
		f.recover()
		if sends, reads := f.counts(row.ID); sends != 1 || reads != 1 {
			t.Fatalf("HAL queue replayed sends=%d reads=%d", sends, reads)
		}
	}
	row := f.persist(nil)
	f.mark(f.claim(row.ID))
	oldToken, newToken := uuid.New(), uuid.New()
	if err := f.database.Model(&row).Update("recovery_token", newToken).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.recordChargerOperationResult(ctx, row, halops.ChargerOperation{}, halops.ErrChargerOperationNotFound, &oldToken); err != nil {
		t.Fatal(err)
	}
	if f.load(row.ID).State != "DELIVERY_ATTEMPTED" {
		t.Fatal("stale reconciler overwrote newer reservation")
	}
	result := halops.ChargerOperation{CMSOperationID: row.ID, HALOperationID: uuid.New(), Kind: row.Kind, State: "OCPP_CONFIRMED", OCPPResult: "Accepted"}
	if err := f.service.recordChargerOperationResult(ctx, row, result, nil, &newToken); err != nil {
		t.Fatal(err)
	}
	if err := f.service.recordChargerOperationResult(ctx, row, halops.ChargerOperation{}, errors.New("late error"), nil); err != nil {
		t.Fatal(err)
	}
	if f.load(row.ID).State != "OCPP_CONFIRMED" {
		t.Fatal("late dispatch error regressed terminal result")
	}
}

func TestChargerOperationRecoveryBatchAndReadCompatibilityWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	ctx := context.Background()
	rows := make([]models.ChargerOperation, chargerOperationBatch+1)
	for i := range rows {
		rows[i] = f.persist(nil)
	}
	f.recover()
	total := 0
	for _, row := range rows {
		sent, _ := f.counts(row.ID)
		total += sent
	}
	if total != chargerOperationBatch {
		t.Fatalf("batch dispatched %d", total)
	}
	f.recover()
	for _, row := range rows {
		if sent, _ := f.counts(row.ID); sent != 1 {
			t.Fatal("batch starved later row")
		}
	}
	row := f.persist(nil)
	f.mark(f.claim(row.ID))
	f.due(row.ID, false)
	view, err := f.service.GetChargerOperation(ctx, f.principal, row.ID)
	if err != nil || view.State != "CONFIRMED_ABSENT" {
		t.Fatalf("read reconciliation state=%s err=%v", view.State, err)
	}
	if sent, reads := f.counts(row.ID); sent != 0 || reads != 1 {
		t.Fatal("read replayed operation")
	}
}

func TestChargerOperationClaimFailureDoesNotBlockDrainWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	poison, healthy := f.persist(nil), f.persist(nil)
	if err := f.database.Callback().Create().Before("gorm:create").Register("test:poison_event", func(tx *gorm.DB) {
		if event, ok := tx.Statement.Dest.(*models.OperationalEvent); ok && event.ResourceID == poison.ID.String() {
			tx.AddError(errors.New("injected poison event failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := f.service.recoverChargerOperations(context.Background())
	_ = f.database.Callback().Create().Remove("test:poison_event")
	if err == nil {
		t.Fatal("claim/event failure hidden")
	}
	if sends, _ := f.counts(healthy.ID); sends != 1 {
		t.Fatal("failed claim blocked later work")
	}
	if row := f.load(poison.ID); row.State != "PERSISTED" || !row.RecoveryAfter.After(time.Now()) {
		t.Fatal("failed claim lacks durable backoff")
	}
}

func TestChargerOperationRecoveryCancelsInFlightCallWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	row := f.persist(nil)
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer server.Close()
	f.service.halOperations = halops.New(f.database, config.HAL{BaseURL: server.URL, CMSBearerToken: "test", RequestTimeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); f.service.RunChargerOperationRecovery(ctx, nil, "test") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not dispatch")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight worker did not stop")
	}
	if stored := f.load(row.ID); stored.DeliveryAttemptedAt == nil || stored.State == "PERSISTED" || stored.State == "DISPATCH_CLAIMED" {
		t.Fatal("cancellation lost attempt evidence")
	}
}

func TestChargerOperationLeaseUsesDatabaseClockWithPostgreSQL(t *testing.T) {
	f := newOperationRecoveryFixture(t)
	row := f.persist(nil)
	f.claim(row.ID)
	f.service.now = func() time.Time { return time.Now().Add(24 * time.Hour) }
	if _, claimed, err := f.service.claimChargerOperation(context.Background(), row.ID); err != nil || claimed {
		t.Fatalf("clock skew stole active claim claimed=%v err=%v", claimed, err)
	}
	f.recover()
	if sends, _ := f.counts(row.ID); sends != 0 {
		t.Fatal("clock skew dispatched active claim")
	}
}
