package customerauth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// This test exercises the complete migration chain against a disposable
// database. It catches both the cumulative total_kwh mapping and the fact
// that a historical materialized start must yield occupancy to its session.
func TestChargingSessionConnectorOccupancyWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	database, sqlDB, err := db.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(context.Background(), sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	fixture := newChargingAdmissionFixture(t, database)
	var tariff models.Tariff
	if err := database.Where("cpo_id = ?", fixture.cpo.ID).First(&tariff).Error; err != nil {
		t.Fatalf("load tariff: %v", err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	makeSession := func(status constants.SessionStatus, transaction int64) models.ChargingSession {
		halID := uuid.New()
		return models.ChargingSession{ID: uuid.New(), CPOID: fixture.cpo.ID, HALTransactionID: &halID, TransactionID: transaction, CustomerID: fixture.firstPrincipal.CustomerID, ChargerID: fixture.charger.ID, ConnectorID: fixture.connector.ID, TariffID: tariff.ID, StartTime: started, MeterStartWh: 0, TotalKWh: decimal.Zero, TotalAmount: decimal.Zero, Currency: "INR", TariffSnapshot: models.JSONB{}, TaxSnapshot: models.JSONB{}, Status: status, CreatedAt: started, UpdatedAt: started}
	}
	first := makeSession(constants.SessionStatusActive, 1001)
	if err := database.Create(&first).Error; err != nil {
		t.Fatalf("create active session: %v", err)
	}
	conflicting := makeSession(constants.SessionStatusStopPending, 1002)
	if err := database.Create(&conflicting).Error; err == nil {
		t.Fatal("second occupancy-owning session was accepted")
	}
	if err := database.Model(&models.ChargingSession{}).Where("id = ?", first.ID).Update("status", constants.SessionStatusCompleted).Error; err != nil {
		t.Fatalf("complete first session: %v", err)
	}
	second := makeSession(constants.SessionStatusActive, 1003)
	if err := database.Create(&second).Error; err != nil {
		t.Fatalf("second session after completion: %v", err)
	}
}
