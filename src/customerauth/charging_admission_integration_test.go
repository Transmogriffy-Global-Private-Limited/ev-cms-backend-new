package customerauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestChargingStartAdmissionWithPostgreSQL(t *testing.T) {
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

	var halRequests atomic.Int32
	halServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/v1/mappings/chargers/"):
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/remote-commands/start":
			halRequests.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"hal_command_id": uuid.New(), "cms_command_id": uuid.New(), "kind": "START", "state": "ACCEPTED", "updated_at": time.Now().UTC(),
			})
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
	service.WithHALOperations(
		halops.New(gormDB, config.HAL{BaseURL: halServer.URL, CMSBearerToken: "test", MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute}),
		liveops.New(gormDB, config.HAL{MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute}),
		config.HAL{MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute},
	)

	setChargingAdmissionProjection(t, gormDB, fixture, fixture.connector.ID, "ONLINE", "Available", time.Now().UTC())
	first, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "admission-first")
	if err != nil || first.Status != constants.StartIntentStatusAcceptedForDelivery {
		t.Fatalf("available/fresh start response=%+v err=%v", first, err)
	}
	if halRequests.Load() != 1 {
		t.Fatalf("HAL start calls=%d, want 1", halRequests.Load())
	}
	var intent models.ChargingStartIntent
	if err := gormDB.First(&intent, "id = ?", first.StartIntentID).Error; err != nil {
		t.Fatalf("load start intent: %v", err)
	}
	if intent.TariffSnapshot["price_per_unit"] != "10" || intent.TariffSnapshot["tariff_type"] != "fixed" || intent.TariffSnapshot["price_type"] != "energy" || intent.TariffSnapshot["units"] != "kwh" || intent.TariffSnapshot["price_per_kwh"] != nil {
		t.Fatalf("start intent did not freeze canonical tariff semantics: %#v", intent.TariffSnapshot)
	}
	setChargingAdmissionProjection(t, gormDB, fixture, fixture.connector.ID, "ONLINE", "Preparing", time.Now().UTC())
	replay, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "admission-replay")
	if err != nil || replay.StartIntentID != first.StartIntentID {
		t.Fatalf("same-customer replay response=%+v err=%v, want intent %s", replay, err, first.StartIntentID)
	}
	otherCustomerResponse, otherCustomerErr := service.StartCharging(ctx, fixture.secondPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: fixture.connector.ID}, "admission-other-customer")
	assertAdmissionUnavailable(t, otherCustomerResponse, otherCustomerErr)

	for _, test := range []struct {
		name            string
		chargerState    string
		connectorStatus string
		observedAt      time.Time
	}{
		{name: "charging", chargerState: "ONLINE", connectorStatus: "Charging", observedAt: time.Now().UTC()},
		{name: "faulted", chargerState: "ONLINE", connectorStatus: "Faulted", observedAt: time.Now().UTC()},
		{name: "unavailable", chargerState: "ONLINE", connectorStatus: "Unavailable", observedAt: time.Now().UTC()},
		{name: "unknown", chargerState: "UNKNOWN", connectorStatus: "Available", observedAt: time.Now().UTC()},
		{name: "offline", chargerState: "OFFLINE", connectorStatus: "Available", observedAt: time.Now().UTC()},
		{name: "stale", chargerState: "ONLINE", connectorStatus: "Available", observedAt: time.Now().UTC().Add(-2 * time.Minute)},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			connector := fixture.newConnector(t)
			setChargingAdmissionProjection(t, gormDB, fixture, connector.ID, test.chargerState, test.connectorStatus, test.observedAt)
			before := chargingAdmissionSideEffects(t, gormDB, fixture.cpo.ID)
			response, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: connector.ID}, "admission-"+test.name)
			assertAdmissionUnavailable(t, response, err)
			if after := chargingAdmissionSideEffects(t, gormDB, fixture.cpo.ID); after != before {
				t.Fatalf("live admission failure created durable side effects: before=%v after=%v", before, after)
			}
			if halRequests.Load() != 1 {
				t.Fatalf("live admission failure called HAL: calls=%d", halRequests.Load())
			}
		})
	}

	racingConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, racingConnector.ID, "ONLINE", "Available", time.Now().UTC())
	var start sync.WaitGroup
	start.Add(2)
	results := make(chan error, 2)
	for _, principal := range []Principal{fixture.firstPrincipal, fixture.secondPrincipal} {
		principal := principal
		go func() {
			defer start.Done()
			_, err := service.StartCharging(ctx, principal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: racingConnector.ID}, "admission-race")
			results <- err
		}()
	}
	start.Wait()
	close(results)
	var successful, rejected int
	for result := range results {
		if result == nil {
			successful++
			continue
		}
		assertAdmissionUnavailable(t, ChargingStartResponse{}, result)
		rejected++
	}
	if successful != 1 || rejected != 1 {
		t.Fatalf("concurrent admission successful=%d rejected=%d, want 1/1", successful, rejected)
	}
	var intents int64
	if err := gormDB.Model(&models.ChargingStartIntent{}).Where("connector_id = ?", racingConnector.ID).Count(&intents).Error; err != nil || intents != 1 {
		t.Fatalf("concurrent admission intent count=%d err=%v, want 1", intents, err)
	}

	if err := gormDB.Create(&models.Settings{CPOID: fixture.cpo.ID, WalletMinBalance: 500, WalletBufferMinBalance: 20}).Error; err != nil {
		t.Fatalf("create CPO wallet policy: %v", err)
	}
	if err := gormDB.Model(&models.Wallet{}).Where("id = ?", fixture.firstPrincipal.Wallet.ID).Update("balance", decimal.NewFromInt(500)).Error; err != nil {
		t.Fatalf("set wallet balance at CPO threshold: %v", err)
	}
	policyConnector := fixture.newConnector(t)
	if err := gormDB.Model(&models.Connector{}).Where("id = ?", policyConnector.ID).Update("connector_total_capacity", 100).Error; err != nil {
		t.Fatalf("raise connector capacity for wallet-policy affordability: %v", err)
	}
	policyConnector.ConnectorTotalCapacity = 100
	setChargingAdmissionProjection(t, gormDB, fixture, policyConnector.ID, "ONLINE", "Available", time.Now().UTC())
	policyStart, err := service.StartCharging(ctx, fixture.firstPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: policyConnector.ID}, "admission-wallet-policy")
	if err != nil {
		t.Fatalf("wallet-policy start=%+v err=%v", policyStart, err)
	}
	var policyIntent models.ChargingStartIntent
	if err := gormDB.First(&policyIntent, "id = ?", policyStart.StartIntentID).Error; err != nil || policyIntent.EnergyLimitWh != 40677 {
		t.Fatalf("wallet-policy intent=%+v err=%v, want 40677 Wh limit", policyIntent, err)
	}
	var policyHold models.WalletHold
	if err := gormDB.First(&policyHold, "start_intent_id = ?", policyStart.StartIntentID).Error; err != nil {
		t.Fatalf("load wallet-policy hold: %v", err)
	}
	if !policyHold.Amount.Equal(decimal.RequireFromString("479.99")) {
		t.Fatalf("wallet-policy hold=%s, want 479.99 from the 480 usable balance", policyHold.Amount)
	}

	if err := gormDB.Model(&models.Wallet{}).Where("id = ?", fixture.secondPrincipal.Wallet.ID).Update("balance", decimal.NewFromInt(499)).Error; err != nil {
		t.Fatalf("set below-minimum wallet balance: %v", err)
	}
	minimumConnector := fixture.newConnector(t)
	setChargingAdmissionProjection(t, gormDB, fixture, minimumConnector.ID, "ONLINE", "Available", time.Now().UTC())
	_, err = service.StartCharging(ctx, fixture.secondPrincipal, ChargingStartRequest{ChargerID: fixture.charger.ChargerID, ConnectorID: minimumConnector.ID}, "admission-wallet-minimum")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Code != "wallet_minimum_balance_not_met" {
		t.Fatalf("below-minimum start error=%v, want wallet_minimum_balance_not_met", err)
	}
}

func TestUserAppTariffTargetPrecedenceWithPostgreSQL(t *testing.T) {
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
	now := time.Now().UTC()
	assertPrice := func(label, want string, response CustomerPriceResponse, err error) {
		t.Helper()
		if err != nil || response.Status != customerPriceAvailable || response.PricePerUnit != want {
			t.Fatalf("%s price=%+v err=%v, want AVAILABLE %s", label, response, err, want)
		}
	}

	hubPrice, err := service.GetCustomerHubPrice(ctx, fixture.firstPrincipal, *fixture.charger.HubID)
	assertPrice("hub", "10.0000", hubPrice, err)

	tariffType, priceType, units := energyTariffMetadata()
	chargerTariff := models.Tariff{ID: uuid.New(), CPOID: fixture.cpo.ID, AssignedTo: constants.TariffAssignedCharger, ChargerID: &fixture.charger.ID, PricePerUnit: decimal.RequireFromString("11.00"), IdleFeePerMin: decimal.Zero, Currency: "INR", IsActive: true, TariffType: &tariffType, PriceType: &priceType, Units: &units, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&chargerTariff).Error; err != nil {
		t.Fatalf("create charger tariff: %v", err)
	}
	chargerPrice, err := service.GetCustomerChargerPrice(ctx, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertPrice("charger", "11.0000", chargerPrice, err)

	group := models.UserGroup{ID: uuid.New(), CPOID: fixture.cpo.ID, Name: "Pricing group", IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&group).Error; err != nil {
		t.Fatalf("create user group: %v", err)
	}
	groupTariff := models.Tariff{ID: uuid.New(), CPOID: fixture.cpo.ID, AssignedTo: constants.TariffAssignedUserGroup, UserGroupID: &group.ID, PricePerUnit: decimal.RequireFromString("12.00"), IdleFeePerMin: decimal.Zero, Currency: "INR", IsActive: true, TariffType: &tariffType, PriceType: &priceType, Units: &units, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&groupTariff).Error; err != nil {
		t.Fatalf("create user-group tariff: %v", err)
	}
	if err := gormDB.Model(&models.Customer{}).Where("id = ?", fixture.firstPrincipal.CustomerID).Update("user_group_id", group.ID).Error; err != nil {
		t.Fatalf("assign customer user group: %v", err)
	}
	fixture.firstPrincipal.Customer.UserGroupID = &group.ID

	chargerPrice, err = service.GetCustomerChargerPrice(ctx, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertPrice("user-group charger", "12.0000", chargerPrice, err)
	hubPrice, err = service.GetCustomerHubPrice(ctx, fixture.firstPrincipal, *fixture.charger.HubID)
	assertPrice("user-group hub", "12.0000", hubPrice, err)

	selected, ok, err := service.effectiveChargingTariff(gormDB.WithContext(ctx), fixture.firstPrincipal, *fixture.charger.HubID, fixture.charger.ID, now)
	if err != nil || !ok || !selected.PricePerUnit.Equal(decimal.RequireFromString("12.00")) {
		t.Fatalf("charging tariff=%+v ok=%v err=%v, want user-group tariff", selected, ok, err)
	}
}

func TestCustomerPriceRejectsInvalidPersistedHubGSTWithPostgreSQL(t *testing.T) {
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
	available, err := service.GetCustomerHubPrice(ctx, fixture.firstPrincipal, *fixture.charger.HubID)
	if err != nil || available.Status != customerPriceAvailable {
		t.Fatalf("valid Hub GST price=%+v err=%v", available, err)
	}

	var hub models.Hub
	if err := gormDB.First(&hub, "id = ?", *fixture.charger.HubID).Error; err != nil || hub.GSTID == nil {
		t.Fatalf("load fixture Hub GST: hub=%+v err=%v", hub, err)
	}
	invalidIGST := decimal.NewFromInt(18)
	if err := gormDB.Model(&models.GST{}).Where("id = ?", *hub.GSTID).Update("igst_rate", invalidIGST).Error; err != nil {
		t.Fatalf("inject invalid persisted Hub GST: %v", err)
	}
	unavailable, err := service.GetCustomerHubPrice(ctx, fixture.firstPrincipal, *fixture.charger.HubID)
	if err != nil || unavailable.Status != customerPriceUnavailable || unavailable.UnavailableReason != "hub_gst_unavailable" {
		t.Fatalf("invalid Hub GST price=%+v err=%v, want unavailable", unavailable, err)
	}
}

type chargingAdmissionFixture struct {
	cpo             models.CPO
	charger         models.Charger
	connector       models.Connector
	firstPrincipal  Principal
	secondPrincipal Principal
	database        *gorm.DB
}

func newChargingAdmissionFixture(t *testing.T, database *gorm.DB) chargingAdmissionFixture {
	t.Helper()
	now := time.Now().UTC()
	cpo := createActiveTestCPO(t, database)
	hub := models.Hub{ID: uuid.New(), CPOID: cpo.ID, Name: "Admission hub", Address: "1 Test Road", State: constants.WestBengal, Latitude: 22.5726, Longitude: 88.3639, CustomerVisible: true, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&hub).Error; err != nil {
		t.Fatalf("create hub: %v", err)
	}
	charger := models.Charger{ID: uuid.New(), CPOID: cpo.ID, HubID: &hub.ID, ChargerID: "a1b2c3", OCPPIdentity: "admission-" + uuid.NewString(), Status: constants.ChargerStatusActive, ChargerName: "Admission charger", NumberOfConnectors: 8, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&charger).Error; err != nil {
		t.Fatalf("create charger: %v", err)
	}
	nine, zero := decimal.NewFromInt(9), decimal.Zero
	gst := models.GST{ID: uuid.New(), CPOID: cpo.ID, Name: "Admission GST", State: constants.WestBengal, SGSTRate: &nine, CGSTRate: &nine, IGSTRate: &zero, IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&gst).Error; err != nil {
		t.Fatalf("create hub GST: %v", err)
	}
	if err := database.Model(&models.Hub{}).Where("id = ?", hub.ID).Update("gst_id", gst.ID).Error; err != nil {
		t.Fatalf("assign hub GST: %v", err)
	}
	hub.GSTID = &gst.ID
	tariffType, priceType, units := energyTariffMetadata()
	tariff := models.Tariff{ID: uuid.New(), CPOID: cpo.ID, HubID: &hub.ID, AssignedTo: constants.TariffAssignedHub, PricePerUnit: decimal.RequireFromString("10.00"), IdleFeePerMin: decimal.Zero, Currency: "INR", IsActive: true, TariffType: &tariffType, PriceType: &priceType, Units: &units, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&tariff).Error; err != nil {
		t.Fatalf("create tariff: %v", err)
	}
	firstPrincipal := createChargingAdmissionCustomer(t, database, cpo.ID, now)
	secondPrincipal := createChargingAdmissionCustomer(t, database, cpo.ID, now)
	fixture := chargingAdmissionFixture{cpo: cpo, charger: charger, firstPrincipal: firstPrincipal, secondPrincipal: secondPrincipal, database: database}
	fixture.connector = fixture.newConnector(t)
	return fixture
}

func (fixture chargingAdmissionFixture) newConnector(t *testing.T) models.Connector {
	t.Helper()
	var count int64
	if err := fixture.database.Model(&models.Connector{}).Where("charger_id = ?", fixture.charger.ID).Count(&count).Error; err != nil {
		t.Fatalf("count connectors: %v", err)
	}
	now := time.Now().UTC()
	connector := models.Connector{ID: uuid.New(), CPOID: fixture.cpo.ID, ChargerID: fixture.charger.ID, ConnectorNumber: int(count) + 1, ConnectorType: "CCS2", ConnectorTotalCapacity: 7.4, Status: constants.ChargerStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := fixture.database.Create(&connector).Error; err != nil {
		t.Fatalf("create connector: %v", err)
	}
	return connector
}

func createChargingAdmissionCustomer(t *testing.T, database *gorm.DB, cpoID uuid.UUID, now time.Time) Principal {
	t.Helper()
	customer := models.Customer{ID: uuid.New(), CPOID: cpoID, Email: "admission-" + uuid.NewString() + "@example.com", PasswordHash: "test-password-hash", FullName: "Charging admission customer", IsVerified: true, PasswordChangedAt: now, Status: constants.CustomerStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	wallet := models.Wallet{ID: uuid.New(), CPOID: cpoID, CustomerID: customer.ID, Balance: decimal.RequireFromString("1000.00"), Currency: "INR", CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&wallet).Error; err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	return Principal{UserID: customer.ID, CustomerID: customer.ID, CPOID: cpoID, Customer: CustomerView{ID: customer.ID, Status: string(customer.Status)}, Wallet: WalletView{ID: wallet.ID, Balance: wallet.Balance.StringFixed(2), Currency: wallet.Currency}}
}

func setChargingAdmissionProjection(t *testing.T, database *gorm.DB, fixture chargingAdmissionFixture, connectorID uuid.UUID, chargerState, connectorStatus string, observedAt time.Time) {
	t.Helper()
	charger := models.HALChargerRuntime{CMSChargerID: fixture.charger.ID, CPOID: fixture.cpo.ID, ConnectionState: chargerState, ConnectionGeneration: 1, ConnectionSequence: time.Now().UnixNano(), ObservedAt: observedAt, UpdatedAt: time.Now().UTC()}
	if err := database.Save(&charger).Error; err != nil {
		t.Fatalf("store charger projection: %v", err)
	}
	connector := models.HALConnectorRuntime{CMSConnectorID: connectorID, CMSChargerID: fixture.charger.ID, CPOID: fixture.cpo.ID, OCPPConnectorStatus: connectorStatus, ConnectorStatusSequence: time.Now().UnixNano(), ObservedAt: observedAt, UpdatedAt: time.Now().UTC()}
	if err := database.Save(&connector).Error; err != nil {
		t.Fatalf("store connector projection: %v", err)
	}
}

type chargingAdmissionEffects struct{ intents, holds, commands int64 }

func chargingAdmissionSideEffects(t *testing.T, database *gorm.DB, cpoID uuid.UUID) chargingAdmissionEffects {
	t.Helper()
	var result chargingAdmissionEffects
	for _, target := range []struct {
		model any
		into  *int64
	}{{&models.ChargingStartIntent{}, &result.intents}, {&models.WalletHold{}, &result.holds}, {&models.HALCommandRecord{}, &result.commands}} {
		if err := database.Model(target.model).Where("cpo_id = ?", cpoID).Count(target.into).Error; err != nil {
			t.Fatalf("count charging side effects: %v", err)
		}
	}
	return result
}

func assertAdmissionUnavailable(t *testing.T, _ ChargingStartResponse, err error) {
	t.Helper()
	apiError, ok := err.(*APIError)
	if !ok || apiError.Status != http.StatusConflict || apiError.Code != "connector_not_available" {
		t.Fatalf("error=%v, want 409 connector_not_available", err)
	}
}
