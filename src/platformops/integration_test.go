package platformops

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPlatformEventsAuditAndWorkerLifecycleWithPostgreSQL(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Microsecond)
	service := NewService(gormDB, config.Platform{
		EventRetention:    time.Hour,
		RealtimePoll:      10 * time.Millisecond,
		RealtimeHeartbeat: 100 * time.Millisecond,
		RealtimeBatchSize: 10,
		WorkerStaleAfter:  time.Minute,
		MaintenanceEvery:  time.Minute,
	})
	service.now = func() time.Time { return now }
	principal := auth.Principal{
		UserID: uuid.New(),
		Scope:  constants.AuthScopePlatform,
	}

	var first models.PlatformEvent
	if err := gormDB.Transaction(func(tx *gorm.DB) error {
		var emitErr error
		first, emitErr = service.Emit(tx, EventInput{
			Type:         "platform.test.created",
			ResourceType: "TEST",
			Data:         models.JSONB{"state": "CREATED"},
		})
		return emitErr
	}); err != nil {
		t.Fatalf("emit platform event: %v", err)
	}
	if first.ID == 0 || !first.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected emitted event: %#v", first)
	}
	page, err := service.ListEvents(ctx, principal, EventQuery{
		AfterID: first.ID - 1,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("list platform events: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].ID != first.ID {
		t.Fatalf("unexpected event page: %#v", page)
	}

	audit := models.AuditLog{
		ID:        uuid.New(),
		Action:    "PLATFORM_TEST",
		Entity:    "PLATFORM_EVENT",
		EntityID:  nil,
		Details:   models.JSONB{"event_id": first.ID},
		CreatedAt: now,
	}
	if err := gormDB.Create(&audit).Error; err != nil {
		t.Fatalf("create audit record: %v", err)
	}
	auditPage, err := service.ListAudit(ctx, principal, AuditQuery{
		Action: "PLATFORM_TEST",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit records: %v", err)
	}
	if len(auditPage.Records) != 1 || auditPage.Records[0].ID != audit.ID {
		t.Fatalf("unexpected audit page: %#v", auditPage)
	}
	sameTimeAudit := models.AuditLog{
		ID:        uuid.New(),
		Action:    "PLATFORM_TEST",
		Entity:    "PLATFORM_EVENT",
		Details:   models.JSONB{"event_id": first.ID},
		CreatedAt: now,
	}
	if err := gormDB.Create(&sameTimeAudit).Error; err != nil {
		t.Fatalf("create same-time audit record: %v", err)
	}
	firstAuditPage, err := service.ListAudit(ctx, principal, AuditQuery{
		Action: "PLATFORM_TEST",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("list first audit cursor page: %v", err)
	}
	if !firstAuditPage.HasMore ||
		firstAuditPage.NextBefore == nil ||
		firstAuditPage.NextBeforeID == nil {
		t.Fatalf("missing composite audit cursor: %#v", firstAuditPage)
	}
	secondAuditPage, err := service.ListAudit(ctx, principal, AuditQuery{
		Before:   firstAuditPage.NextBefore,
		BeforeID: firstAuditPage.NextBeforeID,
		Action:   "PLATFORM_TEST",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("list second audit cursor page: %v", err)
	}
	if len(secondAuditPage.Records) != 1 ||
		secondAuditPage.Records[0].ID == firstAuditPage.Records[0].ID {
		t.Fatalf("composite cursor skipped or repeated record: %#v", secondAuditPage)
	}

	instanceKey := uuid.NewString()
	if err := service.Heartbeat(ctx, "test-worker", instanceKey); err != nil {
		t.Fatalf("record worker heartbeat: %v", err)
	}
	workers, err := service.ListWorkers(ctx, principal)
	if err != nil {
		t.Fatalf("list workers: %v", err)
	}
	found := false
	for _, worker := range workers.Workers {
		if worker.InstanceKey == instanceKey {
			found = true
			if worker.Status != "HEALTHY" {
				t.Fatalf("fresh worker status = %s, want HEALTHY", worker.Status)
			}
		}
	}
	if !found {
		t.Fatal("new worker heartbeat not listed")
	}
	ready, err := service.RequiredWorkersReady(ctx)
	if err != nil || !ready {
		t.Fatalf("fresh required worker readiness = %v, %v", ready, err)
	}

	service.now = func() time.Time { return now.Add(2 * time.Minute) }
	workers, err = service.ListWorkers(ctx, principal)
	if err != nil {
		t.Fatalf("list stale workers: %v", err)
	}
	for _, worker := range workers.Workers {
		if worker.InstanceKey == instanceKey && worker.Status != "STALE" {
			t.Fatalf("stale worker status = %s, want STALE", worker.Status)
		}
	}
	ready, err = service.RequiredWorkersReady(ctx)
	if err != nil || ready {
		t.Fatalf("stale required worker readiness = %v, %v", ready, err)
	}

	replacementInstanceKey := uuid.NewString()
	if err := service.Heartbeat(ctx, "test-worker", replacementInstanceKey); err != nil {
		t.Fatalf("record replacement worker heartbeat: %v", err)
	}
	ready, err = service.RequiredWorkersReady(ctx)
	if err != nil || !ready {
		t.Fatalf(
			"replacement instance must satisfy worker readiness despite stale peer = %v, %v",
			ready,
			err,
		)
	}

	if err := gormDB.Delete(&models.AuditLog{}, "id = ?", audit.ID).Error; err != nil {
		t.Fatalf("delete test audit: %v", err)
	}
	if err := gormDB.Delete(&models.AuditLog{}, "id = ?", sameTimeAudit.ID).Error; err != nil {
		t.Fatalf("delete same-time test audit: %v", err)
	}
	if err := gormDB.Delete(
		&models.WorkerInstance{},
		"instance_key IN ?",
		[]string{instanceKey, replacementInstanceKey},
	).Error; err != nil {
		t.Fatalf("delete test worker: %v", err)
	}
	if err := gormDB.Delete(&models.PlatformEvent{}, "id = ?", first.ID).Error; err != nil &&
		!errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("delete test event: %v", err)
	}
}
