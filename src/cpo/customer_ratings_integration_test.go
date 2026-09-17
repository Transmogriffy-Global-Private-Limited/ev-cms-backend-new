package cpo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// TestListCustomerRatingsRepositoryWithPostgreSQL verifies tenant isolation,
// association projection, filters, and keyset pagination on PostgreSQL. It
// runs only against an explicitly selected disposable TEST_DATABASE_URL.
func TestListCustomerRatingsRepositoryWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	base := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	cpoA, customerA, chargerA, _, _ := createChargingSessionListFixture(t, database, base)
	cpoB, customerB, chargerB, _, _ := createChargingSessionListFixture(t, database, base)
	makeRating := func(cpoID, customerID, chargerID uuid.UUID, created time.Time, score int) models.CustomerRating {
		return models.CustomerRating{
			ID: uuid.New(), CPOID: cpoID, CustomerID: customerID, ChargerID: chargerID,
			OverallRating: score, CreatedAt: created, UpdatedAt: created,
		}
	}
	rows := []models.CustomerRating{
		makeRating(cpoA, customerA, chargerA, base.Add(time.Minute), 2),
		makeRating(cpoA, customerA, chargerA, base.Add(2*time.Minute), 5),
		makeRating(cpoA, customerA, chargerA, base.Add(3*time.Minute), 4),
		makeRating(cpoB, customerB, chargerB, base.Add(4*time.Minute), 5),
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatalf("create rating fixtures: %v", err)
	}

	service := &Service{repository: NewRepository(database)}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoA}
	first, err := service.ListCustomerRatings(ctx, principal, CustomerRatingListQuery{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Ratings) != 2 || !first.HasMore || first.Ratings[0].ID != rows[2].ID || first.Ratings[1].ID != rows[1].ID {
		t.Fatalf("first page order/tenant scope: %+v", first)
	}
	if first.Ratings[0].CustomerEmail == "" || first.Ratings[0].ChargerCode != "s1t001" {
		t.Fatalf("parent associations not loaded into CPO projection: %+v", first.Ratings[0])
	}
	second, err := service.ListCustomerRatings(ctx, principal, CustomerRatingListQuery{
		Limit: 2, Before: first.NextBefore, BeforeID: first.NextBeforeID,
	})
	if err != nil || len(second.Ratings) != 1 || second.HasMore || second.Ratings[0].ID != rows[0].ID {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	filtered, err := service.ListCustomerRatings(ctx, principal, CustomerRatingListQuery{Limit: 10, MinOverall: intPointerForTest(5)})
	if err != nil || len(filtered.Ratings) != 1 || filtered.Ratings[0].ID != rows[1].ID {
		t.Fatalf("rating filter=%+v err=%v", filtered, err)
	}
	otherTenant, err := service.ListCustomerRatings(ctx, auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoB}, CustomerRatingListQuery{Limit: 10})
	if err != nil || len(otherTenant.Ratings) != 1 || otherTenant.Ratings[0].ID != rows[3].ID {
		t.Fatalf("other tenant page=%+v err=%v", otherTenant, err)
	}
}
