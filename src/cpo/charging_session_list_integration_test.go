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
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TestChargingSessionListRepositoryWithPostgreSQL verifies that the typed
// service query reaches PostgreSQL with the same boundary and cursor semantics
// advertised by the CPO endpoint. It requires an explicitly disposable DB.
func TestChargingSessionListRepositoryWithPostgreSQL(t *testing.T) {
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

	base := time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
	cpoID, customerID, chargerID, connectorID, tariffID := createChargingSessionListFixture(t, database, base)
	latest := int64(2_500)
	makeSession := func(transactionID int64, start time.Time, end *time.Time, created time.Time, status constants.SessionStatus, totalKWh, amount string, stopReason, settlement string, latestMeter *int64) models.ChargingSession {
		return models.ChargingSession{
			ID: uuid.New(), CPOID: cpoID, TransactionID: transactionID,
			CustomerID: customerID, ChargerID: chargerID, ConnectorID: connectorID, TariffID: tariffID,
			StartTime: start, EndTime: end, MeterStartWh: 1_000, LatestMeterWh: latestMeter,
			TotalKWh: decimal.RequireFromString(totalKWh), TotalAmount: decimal.RequireFromString(amount), Currency: "INR",
			StopReason: optionalChargingSessionListString(stopReason), TariffSnapshot: models.JSONB{}, TaxSnapshot: models.JSONB{},
			Status: status, SettlementStatus: settlement, CreatedAt: created, UpdatedAt: created,
		}
	}
	endOne, endTwo, endThree := base.Add(10*time.Minute), base.Add(25*time.Minute), base.Add(40*time.Minute)
	sessions := []models.ChargingSession{
		makeSession(1001, base, &endOne, base.Add(time.Minute), constants.SessionStatusCompleted, "1.000", "10.00", "Local", "COMPLETED", nil),
		makeSession(1002, base.Add(2*time.Minute), &endTwo, base.Add(2*time.Minute), constants.SessionStatusCompleted, "2.000", "20.00", "Remote", "PENDING", nil),
		makeSession(1003, base.Add(3*time.Minute), nil, base.Add(3*time.Minute), constants.SessionStatusActive, "0.000", "15.00", "", "PENDING", &latest),
		makeSession(1004, base.Add(4*time.Minute), &endThree, base.Add(4*time.Minute), constants.SessionStatusCompleted, "3.000", "30.00", "Local", "FAILED", nil),
		makeSession(1005, base.Add(5*time.Minute), &endThree, base.Add(4*time.Minute), constants.SessionStatusCompleted, "4.000", "40.00", "Local", "PENDING", nil),
	}
	for index := range sessions {
		if err := database.Create(&sessions[index]).Error; err != nil {
			t.Fatalf("create session %d: %v", index, err)
		}
	}

	service := &Service{repository: NewRepository(database), now: func() time.Time { return base.Add(time.Hour) }}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, CPOID: &cpoID}
	assertIDs := func(name string, query ChargingSessionListQuery, expected ...uuid.UUID) {
		t.Helper()
		response, err := service.ListChargingSessions(ctx, principal, query)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(response.Sessions) != len(expected) {
			t.Fatalf("%s count=%d, want %d: %+v", name, len(response.Sessions), len(expected), response.Sessions)
		}
		for index, want := range expected {
			if response.Sessions[index].ID != want {
				t.Fatalf("%s row %d=%s, want %s", name, index, response.Sessions[index].ID, want)
			}
		}
	}

	// Every range reaches a concrete SQL predicate. Strict bounds exclude exact
	// matches; inclusive bounds retain them. The active row's 1.5 kWh is derived
	// from latest_meter_wh and therefore participates in the same truth as the
	// returned total_kwh field.
	valueOne, valueTwo := decimal.NewFromInt(1), decimal.NewFromInt(2)
	amountTwenty := decimal.NewFromInt(20)
	assertIDs("start_time range", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", StartTimeFrom: timePointer(base.Add(2 * time.Minute)), StartTimeTo: timePointer(base.Add(4 * time.Minute))}, sessions[1].ID, sessions[2].ID)
	assertIDs("created_at range", ChargingSessionListQuery{SortBy: "created_at", SortOrder: "asc", CreatedAtFrom: timePointer(base.Add(2 * time.Minute)), CreatedAtTo: timePointer(base.Add(4 * time.Minute))}, sessions[1].ID, sessions[2].ID)
	assertIDs("end_time range excludes open", ChargingSessionListQuery{SortBy: "created_at", SortOrder: "asc", EndTimeFrom: timePointer(base.Add(20 * time.Minute)), EndTimeTo: timePointer(base.Add(30 * time.Minute))}, sessions[1].ID)
	assertIDs("usage min inclusive", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalKWhMin: &valueTwo}, sessions[1].ID, sessions[3].ID, sessions[4].ID)
	assertIDs("usage max inclusive", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalKWhMax: &valueTwo}, sessions[0].ID, sessions[1].ID, sessions[2].ID)
	assertIDs("usage strict greater", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalKWhGT: &valueOne}, sessions[1].ID, sessions[2].ID, sessions[3].ID, sessions[4].ID)
	assertIDs("usage strict less", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalKWhLT: &valueTwo}, sessions[0].ID, sessions[2].ID)
	assertIDs("amount min inclusive", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalAmountMin: &amountTwenty}, sessions[1].ID, sessions[3].ID, sessions[4].ID)
	assertIDs("amount max inclusive", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalAmountMax: &amountTwenty}, sessions[0].ID, sessions[1].ID, sessions[2].ID)
	assertIDs("amount strict greater", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalAmountGT: &amountTwenty}, sessions[3].ID, sessions[4].ID)
	assertIDs("amount strict less", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", TotalAmountLT: &amountTwenty}, sessions[0].ID, sessions[2].ID)
	assertIDs("duration range", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", DurationMin: int64Pointer(600), DurationMax: int64Pointer(1_500)}, sessions[0].ID, sessions[1].ID)
	assertIDs("equality filters", ChargingSessionListQuery{SortBy: "start_time", SortOrder: "asc", Status: sessionStatusPointer(constants.SessionStatusCompleted), ChargerID: &chargerID, CustomerID: &customerID, ConnectorID: &connectorID, TariffID: &tariffID, Currency: stringPointer("INR"), StopReason: stringPointer("Remote"), SettlementStatus: stringPointer("PENDING")}, sessions[1].ID)
	for name, query := range map[string]ChargingSessionListQuery{
		"status":            {Status: sessionStatusPointer(constants.SessionStatusFailed)},
		"charger":           {ChargerID: uuidPointer(uuid.New())},
		"customer":          {CustomerID: uuidPointer(uuid.New())},
		"connector":         {ConnectorID: uuidPointer(uuid.New())},
		"tariff":            {TariffID: uuidPointer(uuid.New())},
		"currency":          {Currency: stringPointer("USD")},
		"stop_reason":       {StopReason: stringPointer("PowerLoss")},
		"settlement_status": {SettlementStatus: stringPointer("REVERSED")},
	} {
		assertIDs("nonmatching "+name, query)
	}
	for _, sortBy := range []string{"created_at", "start_time", "end_time", "duration", "usage"} {
		for _, sortOrder := range []string{"asc", "desc"} {
			name := sortBy + " " + sortOrder
			query := ChargingSessionListQuery{SortBy: sortBy, SortOrder: sortOrder, Limit: 50}
			if sortBy == "duration" {
				query.AsOf = timePointer(base.Add(time.Hour))
			}
			reference, err := service.ListChargingSessions(ctx, principal, query)
			if err != nil {
				t.Fatalf("reference %s: %v", name, err)
			}
			pageQuery := query
			pageQuery.Limit = 1
			var traversed []uuid.UUID
			for {
				page, err := service.ListChargingSessions(ctx, principal, pageQuery)
				if err != nil {
					t.Fatalf("page %s: %v", name, err)
				}
				for _, session := range page.Sessions {
					traversed = append(traversed, session.ID)
				}
				if !page.HasMore {
					break
				}
				cursor, err := parseChargingSessionCursor(sortBy, *page.NextCursorValue, *page.NextCursorID)
				if err != nil {
					t.Fatalf("parse next cursor %s: %v", name, err)
				}
				pageQuery.Cursor = cursor
				pageQuery.AsOf = page.AsOf
			}
			if len(traversed) != len(reference.Sessions) {
				t.Fatalf("%s traversed %d rows, reference %d", name, len(traversed), len(reference.Sessions))
			}
			seen := map[uuid.UUID]bool{}
			for index, id := range traversed {
				if seen[id] || id != reference.Sessions[index].ID {
					t.Fatalf("%s traversal=%v reference=%+v", name, traversed, reference.Sessions)
				}
				seen[id] = true
			}
		}
	}
	for _, sortOrder := range []string{"asc", "desc"} {
		response, err := service.ListChargingSessions(ctx, principal, ChargingSessionListQuery{SortBy: "created_at", SortOrder: sortOrder, Limit: 50})
		if err != nil {
			t.Fatalf("equal created_at %s: %v", sortOrder, err)
		}
		var tied []uuid.UUID
		for _, row := range response.Sessions {
			if row.CreatedAt.Equal(base.Add(4 * time.Minute)) {
				tied = append(tied, row.ID)
			}
		}
		if len(tied) != 2 {
			t.Fatalf("equal created_at %s tied rows=%v", sortOrder, tied)
		}
		if (sortOrder == "asc" && tied[0].String() >= tied[1].String()) || (sortOrder == "desc" && tied[0].String() <= tied[1].String()) {
			t.Fatalf("equal created_at %s did not use UUID tie-breaker: %v", sortOrder, tied)
		}
	}

	first, err := service.ListChargingSessions(ctx, principal, ChargingSessionListQuery{Limit: 1})
	if err != nil || !first.HasMore || first.NextBefore == nil || first.NextBeforeID == nil {
		t.Fatalf("legacy first page=%+v err=%v", first, err)
	}
	second, err := service.ListChargingSessions(ctx, principal, ChargingSessionListQuery{Limit: 1, Before: first.NextBefore, BeforeID: first.NextBeforeID})
	if err != nil || len(second.Sessions) != 1 || second.Sessions[0].ID == first.Sessions[0].ID {
		t.Fatalf("legacy continuation=%+v err=%v", second, err)
	}
	timestampOnly, err := service.ListChargingSessions(ctx, principal, ChargingSessionListQuery{Limit: 50, Before: first.NextBefore})
	if err != nil {
		t.Fatalf("legacy timestamp-only continuation: %v", err)
	}
	for _, row := range timestampOnly.Sessions {
		if !row.CreatedAt.Before(*first.NextBefore) {
			t.Fatalf("legacy timestamp-only continuation retained %s at %s", row.ID, row.CreatedAt)
		}
	}
}

func createChargingSessionListFixture(t *testing.T, database *gorm.DB, now time.Time) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	cpo := models.CPO{ID: uuid.New(), Slug: "session-list-" + uuid.NewString()[:8], BusinessName: "Session List CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: uniqueCPOGSTIN(), Address: "1 Test Road", City: "Kolkata", State: constants.WestBengal, Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "test", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&cpo).Error; err != nil {
		t.Fatalf("create CPO: %v", err)
	}
	hub := models.Hub{ID: uuid.New(), CPOID: cpo.ID, Name: "Session List Hub", Address: "1 Test Road", State: constants.WestBengal, Latitude: 22.57, Longitude: 88.36, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&hub).Error; err != nil {
		t.Fatalf("create hub: %v", err)
	}
	charger := models.Charger{ID: uuid.New(), CPOID: cpo.ID, HubID: &hub.ID, ChargerID: "s1t001", OCPPIdentity: "session-list-" + uuid.NewString(), Status: constants.ChargerStatusActive, ChargerName: "Session list charger", NumberOfConnectors: 1, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&charger).Error; err != nil {
		t.Fatalf("create charger: %v", err)
	}
	connector := models.Connector{ID: uuid.New(), CPOID: cpo.ID, ChargerID: charger.ID, ConnectorNumber: 1, ConnectorType: "CCS2", ConnectorTotalCapacity: 7.4, Status: constants.ChargerStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&connector).Error; err != nil {
		t.Fatalf("create connector: %v", err)
	}
	tariffType, priceType, unit := constants.TariffTypeFixed, constants.PriceTypeEnergy, constants.UnitKWh
	tariff := models.Tariff{ID: uuid.New(), CPOID: cpo.ID, HubID: &hub.ID, AssignedTo: constants.TariffAssignedHub, PricePerUnit: decimal.NewFromInt(10), IdleFeePerMin: decimal.Zero, Currency: "INR", IsActive: true, TariffType: &tariffType, PriceType: &priceType, Units: &unit, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&tariff).Error; err != nil {
		t.Fatalf("create tariff: %v", err)
	}
	customer := models.Customer{ID: uuid.New(), CPOID: cpo.ID, Email: "session-list-" + uuid.NewString() + "@example.test", PasswordHash: "test-password-hash", FullName: "Session List Customer", IsVerified: true, PasswordChangedAt: now, Status: constants.CustomerStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	return cpo.ID, customer.ID, charger.ID, connector.ID, tariff.ID
}

func optionalChargingSessionListString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func timePointer(value time.Time) *time.Time { return &value }
func int64Pointer(value int64) *int64        { return &value }
func stringPointer(value string) *string     { return &value }
func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }
func sessionStatusPointer(value constants.SessionStatus) *constants.SessionStatus {
	return &value
}
