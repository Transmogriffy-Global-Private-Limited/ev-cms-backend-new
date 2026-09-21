package customerauth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestCustomerSessionRatingWithPostgreSQL(t *testing.T) {
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
	service, err := NewService(database, config.Auth{}, false, nil, nil)
	if err != nil {
		t.Fatalf("new customer service: %v", err)
	}
	completed := createRateableSession(t, database, fixture, fixture.firstPrincipal, constants.SessionStatusCompleted)
	five, four := 5, 4
	created, firstCreated, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, completed.ID, CustomerSessionRatingRequest{OverallRating: 5, StationRating: &four, ChargerRating: &five, Reason: stringPointerForRatingTest("Good charging experience.")})
	if err != nil || !firstCreated || created.SessionID != completed.ID || created.ID == uuid.Nil || created.StationRating == nil || created.ChargerRating == nil {
		t.Fatalf("first PUT rating=%+v created=%t err=%v", created, firstCreated, err)
	}
	var stored models.CustomerRating
	if err := database.First(&stored, "id = ?", created.ID).Error; err != nil || stored.CPOID != fixture.cpo.ID || stored.CustomerID != fixture.firstPrincipal.CustomerID || stored.ChargerID != fixture.charger.ID || stored.HubID == nil || *stored.HubID != *fixture.charger.HubID || stored.SessionID == nil || *stored.SessionID != completed.ID {
		t.Fatalf("derived identity=%+v err=%v", stored, err)
	}
	originalHubID, originalCreatedAt := *stored.HubID, stored.CreatedAt
	otherHub := models.Hub{ID: uuid.New(), CPOID: fixture.cpo.ID, Name: "Later assigned hub", Address: "2 Test Road", State: constants.WestBengal, Latitude: 22.5730, Longitude: 88.3640, CustomerVisible: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.Create(&otherHub).Error; err != nil {
		t.Fatalf("create later assigned hub: %v", err)
	}
	if err := database.Model(&models.Charger{}).Where("id = ? AND cpo_id = ?", fixture.charger.ID, fixture.cpo.ID).Update("hub_id", otherHub.ID).Error; err != nil {
		t.Fatalf("reassign charger hub: %v", err)
	}
	updated, secondCreated, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, completed.ID, CustomerSessionRatingRequest{OverallRating: 1})
	if err != nil || secondCreated || updated.ID != created.ID || updated.StationRating != nil || updated.ChargerRating != nil || updated.Reason != nil || updated.OverallRating != 1 {
		t.Fatalf("replacement rating=%+v created=%t err=%v", updated, secondCreated, err)
	}
	var frozen models.CustomerRating
	if err := database.First(&frozen, "id = ?", created.ID).Error; err != nil || frozen.CPOID != fixture.cpo.ID || frozen.CustomerID != fixture.firstPrincipal.CustomerID || frozen.SessionID == nil || *frozen.SessionID != completed.ID || frozen.ChargerID != fixture.charger.ID || frozen.HubID == nil || *frozen.HubID != originalHubID || !frozen.CreatedAt.Equal(originalCreatedAt) {
		t.Fatalf("rating identity mutated after charger hub reassignment: %+v err=%v", frozen, err)
	}
	repeated, repeatedCreated, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, completed.ID, CustomerSessionRatingRequest{OverallRating: 1})
	if err != nil || repeatedCreated || repeated.ID != created.ID || !repeated.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatalf("equivalent PUT rating=%+v created=%t err=%v", repeated, repeatedCreated, err)
	}
	got, err := service.GetCustomerSessionRating(ctx, fixture.firstPrincipal, completed.ID)
	if err != nil || got.ID != created.ID || got.OverallRating != 1 {
		t.Fatalf("GET rating=%+v err=%v", got, err)
	}
	if _, err := service.GetCustomerSessionRating(ctx, fixture.secondPrincipal, completed.ID); !customerRatingAPIError(err, http.StatusNotFound, "charging session_not_found") {
		t.Fatalf("cross-customer rating read error=%v", err)
	}
	if _, _, err := service.PutCustomerSessionRating(ctx, fixture.secondPrincipal, completed.ID, CustomerSessionRatingRequest{OverallRating: 5}); !customerRatingAPIError(err, http.StatusNotFound, "charging session_not_found") {
		t.Fatalf("cross-customer write error=%v", err)
	}
	otherFixture := newChargingAdmissionFixture(t, database)
	otherCompleted := createRateableSession(t, database, otherFixture, otherFixture.firstPrincipal, constants.SessionStatusCompleted)
	if _, err := service.GetCustomerSessionRating(ctx, fixture.firstPrincipal, otherCompleted.ID); !customerRatingAPIError(err, http.StatusNotFound, "charging session_not_found") {
		t.Fatalf("cross-CPO rating read error=%v", err)
	}

	activeFixture := fixture
	activeFixture.connector = fixture.newConnector(t)
	active := createRateableSession(t, database, activeFixture, fixture.firstPrincipal, constants.SessionStatusActive)
	if _, _, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, active.ID, CustomerSessionRatingRequest{OverallRating: 5}); !customerRatingAPIError(err, http.StatusConflict, "session_not_rateable") {
		t.Fatalf("active rating error=%v", err)
	}
	reconcilingFixture := fixture
	reconcilingFixture.connector = fixture.newConnector(t)
	reconciling := createRateableSession(t, database, reconcilingFixture, fixture.firstPrincipal, constants.SessionStatusReconciliationRequired)
	if _, _, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, reconciling.ID, CustomerSessionRatingRequest{OverallRating: 5}); !customerRatingAPIError(err, http.StatusConflict, "session_not_rateable") {
		t.Fatalf("reconciliation rating error=%v", err)
	}
	concurrent := createRateableSession(t, database, fixture, fixture.firstPrincipal, constants.SessionStatusCompleted)
	var group sync.WaitGroup
	results := make(chan CustomerSessionRatingView, 2)
	errorsOut := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			rating, _, putErr := service.PutCustomerSessionRating(context.Background(), fixture.firstPrincipal, concurrent.ID, CustomerSessionRatingRequest{OverallRating: 5})
			if putErr != nil {
				errorsOut <- putErr
				return
			}
			results <- rating
		}()
	}
	group.Wait()
	close(errorsOut)
	close(results)
	for err := range errorsOut {
		t.Fatalf("concurrent PUT: %v", err)
	}
	var concurrentRows int64
	if err := database.Model(&models.CustomerRating{}).Where("cpo_id = ? AND customer_id = ? AND session_id = ?", fixture.cpo.ID, fixture.firstPrincipal.CustomerID, concurrent.ID).Count(&concurrentRows).Error; err != nil || concurrentRows != 1 {
		t.Fatalf("concurrent rows=%d err=%v", concurrentRows, err)
	}
	for rating := range results {
		if rating.SessionID != concurrent.ID {
			t.Fatalf("concurrent result=%+v", rating)
		}
	}
	hublessFixture := newHublessRatingFixture(t, database, fixture)
	hubless := createRateableSession(t, database, hublessFixture, fixture.firstPrincipal, constants.SessionStatusCompleted)
	hublessRating, hublessCreated, err := service.PutCustomerSessionRating(ctx, fixture.firstPrincipal, hubless.ID, CustomerSessionRatingRequest{OverallRating: 3})
	if err != nil || !hublessCreated {
		t.Fatalf("hubless PUT rating=%+v created=%t err=%v", hublessRating, hublessCreated, err)
	}
	var storedHubless models.CustomerRating
	if err := database.First(&storedHubless, "id = ?", hublessRating.ID).Error; err != nil || storedHubless.HubID != nil || storedHubless.ChargerID != hublessFixture.charger.ID {
		t.Fatalf("hubless identity=%+v err=%v", storedHubless, err)
	}
	listed, err := cpo.NewRepository(database).ListCustomerRatings(ctx, fixture.cpo.ID, cpo.CustomerRatingListQuery{Limit: 20})
	if err != nil || !ratingListContains(listed, created.ID) {
		t.Fatalf("CPO rating listing rows=%+v err=%v", listed, err)
	}
}

func newHublessRatingFixture(t *testing.T, database *gorm.DB, base chargingAdmissionFixture) chargingAdmissionFixture {
	t.Helper()
	now := time.Now().UTC()
	fixture := base
	fixture.charger = models.Charger{
		ID: uuid.New(), CPOID: base.cpo.ID, ChargerID: "hubless-" + uuid.NewString()[:12],
		OCPPIdentity: "rating-hubless-" + uuid.NewString(), Status: constants.ChargerStatusActive,
		ChargerName: "Hubless rating charger", NumberOfConnectors: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&fixture.charger).Error; err != nil {
		t.Fatalf("create hubless charger: %v", err)
	}
	fixture.connector = fixture.newConnector(t)
	return fixture
}

func createRateableSession(t *testing.T, database *gorm.DB, fixture chargingAdmissionFixture, principal Principal, status constants.SessionStatus) models.ChargingSession {
	t.Helper()
	var tariff models.Tariff
	if err := database.Where("cpo_id = ?", fixture.cpo.ID).First(&tariff).Error; err != nil {
		t.Fatalf("load fixture tariff: %v", err)
	}
	now := time.Now().UTC()
	completedAt := now
	session := models.ChargingSession{ID: uuid.New(), CPOID: fixture.cpo.ID, TransactionID: now.UnixNano(), CustomerID: principal.CustomerID, ChargerID: fixture.charger.ID, ConnectorID: fixture.connector.ID, TariffID: tariff.ID, StartTime: now.Add(-time.Minute), EndTime: &completedAt, MeterStartWh: 0, MeterStopWh: int64PointerForRatingTest(1000), TotalKWh: decimal.NewFromInt(1), TotalAmount: decimal.NewFromInt(10), Currency: "INR", TariffSnapshot: models.JSONB{}, TaxSnapshot: models.JSONB{}, Status: status, SettlementStatus: "COMPLETED", CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&session).Error; err != nil {
		t.Fatalf("create rateable session: %v", err)
	}
	return session
}

func int64PointerForRatingTest(value int64) *int64 { return &value }

func customerRatingAPIError(err error, status int, code string) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Status == status && apiError.Code == code
}

func ratingListContains(ratings []models.CustomerRating, id uuid.UUID) bool {
	for _, rating := range ratings {
		if rating.ID == id {
			return true
		}
	}
	return false
}
