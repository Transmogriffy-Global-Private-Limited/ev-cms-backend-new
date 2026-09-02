package chargingtrace

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
)

func TestIngressAdoptsRootsIdempotentlyAndNeverMutatesSessionsWithPostgreSQL(t *testing.T) {
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
	now := time.Date(2026, 9, 2, 12, 0, 1, 0, time.UTC)
	cpoID := uuid.New()
	cpo := models.CPO{ID: cpoID, Slug: "trace-" + strings.ToLower(uuid.NewString()), BusinessName: "Trace Test CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: traceTestGSTIN(), Address: "1 Test Road", City: "Kolkata", State: constants.WestBengal, Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "test", StatusChangedAt: now, AppID: "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&cpo).Error; err != nil {
		t.Fatalf("create CPO: %v", err)
	}
	ingestor := NewIngestor(gormDB, "trace-bearer")
	ingestor.now = func() time.Time { return now }
	first := testEnvelope(t)
	first.TraceID, first.CPOID = uuid.New(), cpoID
	first.EventID = uuid.New()
	startIntentID := uuid.New()
	first.CMSStartIntentID = &startIntentID
	first.ImmutableContentSHA256 = mustDigest(t, first)
	if err := ingestor.Accept(ctx, "trace-bearer", first); err != nil {
		t.Fatalf("accept first: %v", err)
	}
	if err := ingestor.Accept(ctx, "trace-bearer", first); err != nil {
		t.Fatalf("accept exact duplicate: %v", err)
	}
	changed := first
	changed.Summary = "Conflicting immutable event"
	changed.ImmutableContentSHA256 = mustDigest(t, changed)
	if err := ingestor.Accept(ctx, "trace-bearer", changed); !hasStatus(err, 409) {
		t.Fatalf("changed event err=%v", err)
	}
	rootConflict := first
	rootConflict.TraceID, rootConflict.EventID = uuid.New(), uuid.New()
	rootConflict.ImmutableContentSHA256 = mustDigest(t, rootConflict)
	if err := ingestor.Accept(ctx, "trace-bearer", rootConflict); !hasStatus(err, 409) {
		t.Fatalf("conflicting root identity err=%v", err)
	}
	late := first
	late.EventID = uuid.New()
	late.OccurredAt = first.OccurredAt.Add(-time.Minute)
	late.ImmutableContentSHA256 = mustDigest(t, late)
	if err := ingestor.Accept(ctx, "trace-bearer", late); err != nil {
		t.Fatalf("accept out-of-order event: %v", err)
	}
	var events []models.ChargingTraceEvent
	if err := gormDB.Where("trace_id = ?", first.TraceID).Order("ingestion_sequence ASC").Find(&events).Error; err != nil || len(events) != 2 || events[0].IngestionSequence >= events[1].IngestionSequence {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	var sessions int64
	if err := gormDB.Model(&models.ChargingSession{}).Where("cpo_id = ?", cpoID).Count(&sessions).Error; err != nil || sessions != 0 {
		t.Fatalf("trace ingress mutated sessions=%d err=%v", sessions, err)
	}
}

func mustDigest(t *testing.T, envelope Envelope) string {
	t.Helper()
	digest, err := Digest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func hasStatus(err error, status int) bool {
	traceError, ok := err.(*Error)
	return ok && traceError.Status == status
}

func traceTestGSTIN() string { return testsupport.ValidGSTIN("19") }
