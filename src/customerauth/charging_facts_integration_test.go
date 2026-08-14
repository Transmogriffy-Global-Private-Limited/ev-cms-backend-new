package customerauth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"gorm.io/gorm"
)

func TestConnectionFactSequenceRejectsStaleObservationWithPostgreSQL(t *testing.T) {
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
	fixture := newChargingAdmissionFixture(t, gormDB)
	service, err := NewService(gormDB, config.Auth{}, false, nil, nil)
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	service.now = func() time.Time { return time.Date(2026, time.August, 14, 12, 10, 0, 0, time.UTC) }

	firstObserved := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	current := models.JSONB{
		"cpo_id":                fixture.cpo.ID.String(),
		"cms_charger_id":        fixture.charger.ID.String(),
		"charger_ocpp_identity": fixture.charger.OCPPIdentity,
		"connection_state":      "ONLINE",
		"connection_generation": float64(1),
		"connection_sequence":   float64(2),
		"observed_at":           firstObserved.Format(time.RFC3339Nano),
	}
	if err := gormDB.Transaction(func(tx *gorm.DB) error { return service.applyConnectionFact(tx, current) }); err != nil {
		t.Fatalf("apply current connection fact: %v", err)
	}
	stale := models.JSONB{
		"cpo_id":                fixture.cpo.ID.String(),
		"cms_charger_id":        fixture.charger.ID.String(),
		"charger_ocpp_identity": fixture.charger.OCPPIdentity,
		"connection_state":      "OFFLINE",
		"connection_generation": float64(0),
		"connection_sequence":   float64(1),
		"observed_at":           firstObserved.Add(time.Minute).Format(time.RFC3339Nano),
	}
	if err := gormDB.Transaction(func(tx *gorm.DB) error { return service.applyConnectionFact(tx, stale) }); err != nil {
		t.Fatalf("apply stale connection fact: %v", err)
	}
	if err := gormDB.Transaction(func(tx *gorm.DB) error { return service.applyConnectionFact(tx, current) }); err != nil {
		t.Fatalf("apply duplicate connection fact: %v", err)
	}

	var runtime models.HALChargerRuntime
	if err := gormDB.First(&runtime, "cms_charger_id = ?", fixture.charger.ID).Error; err != nil {
		t.Fatalf("load connection runtime: %v", err)
	}
	if runtime.ConnectionState != "ONLINE" || runtime.ConnectionGeneration != 1 || runtime.ConnectionSequence != 2 || !runtime.ObservedAt.Equal(firstObserved) {
		t.Fatalf("connection runtime=%#v", runtime)
	}
}
