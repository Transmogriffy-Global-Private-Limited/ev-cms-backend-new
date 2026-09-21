package customerauth

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCustomerChargerRatingAggregatesWithPostgreSQL(t *testing.T) {
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
	fixture := newChargingAdmissionFixture(t, database)
	if err := database.Model(&models.Charger{}).Where("id = ? AND cpo_id = ?", fixture.charger.ID, fixture.cpo.ID).Update("customer_visibility", true).Error; err != nil {
		t.Fatalf("publish aggregate fixture charger: %v", err)
	}
	fixture.charger.CustomerVisibility = true
	second := newPublishedRatingChargerFixture(t, database, fixture, "a2b3c4")
	unrated := newPublishedRatingChargerFixture(t, database, fixture, "a3b4c5")
	service, err := NewService(database, config.Auth{}, false, nil, nil)
	if err != nil {
		t.Fatalf("new customer service: %v", err)
	}

	noRatings, err := service.GetCustomerCharger(ctx, fixture.firstPrincipal, unrated.charger.ChargerID)
	if err != nil {
		t.Fatalf("get unrated charger: %v", err)
	}
	assertCustomerChargerRating(t, noRatings, nil, 0)

	firstSession := createRateableSession(t, database, fixture, fixture.firstPrincipal, constants.SessionStatusCompleted)
	stationOne, chargerOne := 1, 1
	if _, created, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, firstSession.ID, CustomerSessionRatingRequest{OverallRating: 5, StationRating: &stationOne, ChargerRating: &chargerOne}); err != nil || !created {
		t.Fatalf("create first charger rating: created=%t err=%v", created, err)
	}
	secondSession := createRateableSession(t, database, fixture, fixture.secondPrincipal, constants.SessionStatusCompleted)
	if _, created, err := service.PutCustomerSessionRating(ctx, fixture.secondPrincipal, secondSession.ID, CustomerSessionRatingRequest{OverallRating: 3}); err != nil || !created {
		t.Fatalf("create second charger rating: created=%t err=%v", created, err)
	}

	thirdPrincipal := createChargingAdmissionCustomer(t, database, fixture.cpo.ID, time.Now().UTC())
	for _, score := range []int{5, 4, 4} {
		session := createRateableSession(t, database, second, thirdPrincipal, constants.SessionStatusCompleted)
		if _, created, err := service.PutCustomerSessionRating(ctx, thirdPrincipal, session.ID, CustomerSessionRatingRequest{OverallRating: score, ChargerRating: intPointerForRatingTest(1), StationRating: intPointerForRatingTest(5)}); err != nil || !created {
			t.Fatalf("create second-charger rating score=%d created=%t err=%v", score, created, err)
		}
	}
	if err := database.Create(&models.CustomerRating{ID: uuid.New(), CPOID: fixture.cpo.ID, CustomerID: fixture.firstPrincipal.CustomerID, ChargerID: fixture.charger.ID, HubID: fixture.charger.HubID, OverallRating: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("create nullable-session historical rating: %v", err)
	}
	otherFixture := newChargingAdmissionFixture(t, database)
	otherSession := createRateableSession(t, database, otherFixture, otherFixture.firstPrincipal, constants.SessionStatusCompleted)
	if _, created, err := service.PutCustomerSessionRating(ctx, otherFixture.firstPrincipal, otherSession.ID, CustomerSessionRatingRequest{OverallRating: 1}); err != nil || !created {
		t.Fatalf("create other-CPO rating: created=%t err=%v", created, err)
	}

	list, err := service.ListCustomerChargers(ctx, fixture.firstPrincipal, CustomerChargerListQuery{Limit: 20})
	if err != nil {
		t.Fatalf("list customer chargers: %v", err)
	}
	byID := customerChargersByID(list.Chargers)
	assertCustomerChargerRating(t, byID[fixture.charger.ID], float64PointerForRatingTest(4), 2)
	assertCustomerChargerRating(t, byID[second.charger.ID], float64PointerForRatingTest(4.33), 3)
	assertCustomerChargerRating(t, byID[unrated.charger.ID], nil, 0)

	detail, err := service.GetCustomerCharger(ctx, fixture.firstPrincipal, fixture.charger.ChargerID)
	if err != nil {
		t.Fatalf("get rated charger detail: %v", err)
	}
	assertCustomerChargerRating(t, detail, float64PointerForRatingTest(4), 2)
	hub, err := service.GetCustomerHub(ctx, fixture.firstPrincipal, *fixture.charger.HubID)
	if err != nil {
		t.Fatalf("get rated hub detail: %v", err)
	}
	hubChargers := customerChargersByID(hub.Chargers)
	assertCustomerChargerRating(t, hubChargers[fixture.charger.ID], float64PointerForRatingTest(4), 2)
	assertCustomerChargerRating(t, hubChargers[second.charger.ID], float64PointerForRatingTest(4.33), 3)

	if _, created, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, firstSession.ID, CustomerSessionRatingRequest{OverallRating: 1}); err != nil || created {
		t.Fatalf("replace first charger rating: created=%t err=%v", created, err)
	}
	updated, err := service.GetCustomerCharger(ctx, fixture.firstPrincipal, fixture.charger.ChargerID)
	if err != nil {
		t.Fatalf("get replaced charger aggregate: %v", err)
	}
	assertCustomerChargerRating(t, updated, float64PointerForRatingTest(2), 2)
}

func newPublishedRatingChargerFixture(t *testing.T, database *gorm.DB, base chargingAdmissionFixture, publicID string) chargingAdmissionFixture {
	t.Helper()
	now := time.Now().UTC()
	fixture := base
	hubID := *base.charger.HubID
	fixture.charger = models.Charger{
		ID: uuid.New(), CPOID: base.cpo.ID, HubID: &hubID, ChargerID: publicID,
		OCPPIdentity: "rating-" + uuid.NewString(), Status: constants.ChargerStatusActive,
		ChargerName: "Rating charger " + publicID, NumberOfConnectors: 1,
		CustomerVisibility: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&fixture.charger).Error; err != nil {
		t.Fatalf("create published rating charger: %v", err)
	}
	fixture.connector = fixture.newConnector(t)
	return fixture
}

func customerChargersByID(chargers []CustomerChargerView) map[uuid.UUID]CustomerChargerView {
	byID := make(map[uuid.UUID]CustomerChargerView, len(chargers))
	for _, charger := range chargers {
		byID[charger.ID] = charger
	}
	return byID
}

func assertCustomerChargerRating(t *testing.T, charger CustomerChargerView, average *float64, count int64) {
	t.Helper()
	if charger.RatingCount != count {
		t.Fatalf("charger %s rating_count=%d, want %d", charger.ID, charger.RatingCount, count)
	}
	if average == nil {
		if charger.AverageRating != nil {
			t.Fatalf("charger %s average_rating=%v, want absent", charger.ID, *charger.AverageRating)
		}
		return
	}
	if charger.AverageRating == nil || math.Abs(*charger.AverageRating-*average) > 0.000001 {
		t.Fatalf("charger %s average_rating=%v, want %v", charger.ID, charger.AverageRating, *average)
	}
}

func float64PointerForRatingTest(value float64) *float64 { return &value }
