package customerauth

import (
	"context"
	"os"
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
)

// TestCustomerChargeabilityProjectionWithPostgreSQL exercises the durable
// blockers that deliberately remain independent of an Available OCPP status.
// It is gated on the repository's explicit disposable test database setting.
func TestCustomerChargeabilityProjectionWithPostgreSQL(t *testing.T) {
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
	var tariff models.Tariff
	if err := database.Where("cpo_id = ?", fixture.cpo.ID).First(&tariff).Error; err != nil {
		t.Fatalf("load fixture tariff: %v", err)
	}
	if err := database.Model(&models.Charger{}).Where("id = ?", fixture.charger.ID).Update("customer_visibility", true).Error; err != nil {
		t.Fatalf("make charger customer-visible: %v", err)
	}
	if err := database.Create(&models.HALChargerMapping{CMSChargerID: fixture.charger.ID, CPOID: fixture.cpo.ID, ChargerOCPPIdentity: fixture.charger.OCPPIdentity, SyncState: "SYNCHRONIZED", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("create synchronized mapping: %v", err)
	}
	service, err := NewService(database, config.Auth{}, false, nil, nil)
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	halConfig := config.HAL{BaseURL: "http://hal.invalid", CMSBearerToken: "test", MeterStaleAfter: time.Minute, ConnectionStaleAfter: 15 * time.Minute}
	service.WithHALOperations(halops.New(database, halConfig), liveops.New(database, halConfig), halConfig)

	setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Available", time.Now().UTC())
	before := chargingAdmissionSideEffects(t, database, fixture.cpo.ID)
	response := requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, true, chargeabilityAvailable)
	if after := chargingAdmissionSideEffects(t, database, fixture.cpo.ID); after != before {
		t.Fatalf("read projection created charging side effects: before=%+v after=%+v", before, after)
	}

	setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Preparing", time.Now().UTC())
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, true, chargeabilityAvailable)

	for _, status := range []constants.SessionStatus{constants.SessionStatusActive, constants.SessionStatusStopPending, constants.SessionStatusReconciliationRequired} {
		session := models.ChargingSession{ID: uuid.New(), CPOID: fixture.cpo.ID, TransactionID: time.Now().UnixNano(), CustomerID: fixture.firstPrincipal.CustomerID, ChargerID: fixture.charger.ID, ConnectorID: fixture.connector.ID, TariffID: tariff.ID, StartTime: time.Now().UTC(), Currency: "INR", Status: status, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := database.Create(&session).Error; err != nil {
			t.Fatalf("create %s session: %v", status, err)
		}
		setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Available", time.Now().UTC())
		response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
		assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityConnectorOccupied)
		if err := database.Delete(&session).Error; err != nil {
			t.Fatalf("delete %s session: %v", status, err)
		}
	}
	intent := models.ChargingStartIntent{ID: uuid.New(), CPOID: fixture.cpo.ID, CustomerID: fixture.firstPrincipal.CustomerID, ChargerID: fixture.charger.ID, ConnectorID: fixture.connector.ID, WalletID: fixture.firstPrincipal.Wallet.ID, TariffID: tariff.ID, Status: constants.StartIntentStatusAcceptedForDelivery, CredentialHash: "0000000000000000000000000000000000000000000000000000000000000000", CredentialExpiresAt: time.Now().UTC().Add(time.Minute), CommandExpiresAt: time.Now().UTC().Add(time.Minute), TariffSnapshot: models.JSONB{}, TaxSnapshot: models.JSONB{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.Create(&intent).Error; err != nil {
		t.Fatalf("create unresolved start intent: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityStartInProgress)
	if err := database.Delete(&intent).Error; err != nil {
		t.Fatalf("delete unresolved start intent: %v", err)
	}

	if err := database.Model(&models.Connector{}).Where("id = ?", fixture.connector.ID).Update("status", constants.ChargerStatusInactive).Error; err != nil {
		t.Fatalf("set connector inactive: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityConnectorUnavailable)
	if err := database.Model(&models.Connector{}).Where("id = ?", fixture.connector.ID).Update("status", constants.ChargerStatusActive).Error; err != nil {
		t.Fatalf("restore connector active: %v", err)
	}

	setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Unavailable", time.Now().UTC())
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityConnectorUnavailable)
	setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Available", time.Now().UTC())

	if err := database.Model(&models.Wallet{}).Where("id = ?", fixture.firstPrincipal.Wallet.ID).Update("balance", decimal.Zero).Error; err != nil {
		t.Fatalf("empty customer wallet: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityWalletMinimumBalanceNotMet)
	if err := database.Model(&models.Wallet{}).Where("id = ?", fixture.firstPrincipal.Wallet.ID).Update("balance", decimal.NewFromInt(1000)).Error; err != nil {
		t.Fatalf("restore customer wallet: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, true, chargeabilityAvailable)

	if err := database.Model(&models.Tariff{}).Where("id = ?", tariff.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate tariff: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityNoEligibleTariff)
	if err := database.Model(&models.Tariff{}).Where("id = ?", tariff.ID).Update("is_active", true).Error; err != nil {
		t.Fatalf("restore tariff: %v", err)
	}

	second := fixture.newConnector(t)
	setChargingAdmissionProjection(t, database, fixture, fixture.connector.ID, "ONLINE", "Faulted", time.Now().UTC())
	setChargingAdmissionProjection(t, database, fixture, second.ID, "ONLINE", "Available", time.Now().UTC())
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	assertCustomerConnectorChargeability(t, response, fixture.connector.ID, false, chargeabilityConnectorFaulted)
	assertCustomerConnectorChargeability(t, response, second.ID, true, chargeabilityAvailable)
	if !response.Chargers[0].CanCharge || response.Chargers[0].ChargeabilityReason != chargeabilityAvailable {
		t.Fatalf("mixed connector charger result=%+v, want aggregate AVAILABLE", response.Chargers[0])
	}
	if err := database.Model(&models.Connector{}).Where("id = ?", second.ID).Update("status", constants.ChargerStatusInactive).Error; err != nil {
		t.Fatalf("set second connector inactive: %v", err)
	}
	response = requireCustomerChargeability(t, service, fixture.firstPrincipal, fixture.charger.ChargerID)
	if response.Chargers[0].CanCharge || response.Chargers[0].ChargeabilityReason != chargeabilityNoChargeableConnector {
		t.Fatalf("all-blocked charger result=%+v, want NO_CHARGEABLE_CONNECTOR", response.Chargers[0])
	}
	if err := database.Model(&models.Connector{}).Where("id = ?", second.ID).Update("status", constants.ChargerStatusActive).Error; err != nil {
		t.Fatalf("restore second connector active: %v", err)
	}

	visibleSecond := models.Charger{ID: uuid.New(), CPOID: fixture.cpo.ID, HubID: fixture.charger.HubID, ChargerID: "d4e5f6", OCPPIdentity: "chargeability-" + uuid.NewString(), ChargerName: "Second visible charger", NumberOfConnectors: 1, Status: constants.ChargerStatusActive, CustomerVisibility: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.Create(&visibleSecond).Error; err != nil {
		t.Fatalf("create second visible charger: %v", err)
	}
	visibleSecondConnector := models.Connector{ID: uuid.New(), CPOID: fixture.cpo.ID, ChargerID: visibleSecond.ID, ConnectorNumber: 1, ConnectorType: "CCS2", ConnectorTotalCapacity: 7.4, Status: constants.ChargerStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.Create(&visibleSecondConnector).Error; err != nil {
		t.Fatalf("create second visible connector: %v", err)
	}
	if err := database.Create(&models.HALChargerMapping{CMSChargerID: visibleSecond.ID, CPOID: fixture.cpo.ID, ChargerOCPPIdentity: visibleSecond.OCPPIdentity, SyncState: "SYNCHRONIZED", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("create second synchronized mapping: %v", err)
	}
	secondFixture := fixture
	secondFixture.charger = visibleSecond
	setChargingAdmissionProjection(t, database, secondFixture, visibleSecondConnector.ID, "ONLINE", "Available", time.Now().UTC())
	response, err = service.ListCustomerChargerChargeability(ctx, fixture.firstPrincipal, []string{visibleSecond.ChargerID, fixture.charger.ChargerID, visibleSecond.ChargerID})
	if err != nil || len(response.Chargers) != 2 || response.Chargers[0].ChargerID != visibleSecond.ChargerID || response.Chargers[1].ChargerID != fixture.charger.ChargerID {
		t.Fatalf("bounded multi-ID projection=%+v err=%v, want requested visible order without duplicate", response, err)
	}
	if err := database.Model(&models.Charger{}).Where("id = ?", visibleSecond.ID).Update("customer_visibility", false).Error; err != nil {
		t.Fatalf("hide same-CPO charger: %v", err)
	}

	otherCPO := createActiveTestCPO(t, database)
	other := models.Charger{ID: uuid.New(), CPOID: otherCPO.ID, ChargerID: "z9y8x7", ChargerName: "Hidden", Status: constants.ChargerStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.Create(&other).Error; err != nil {
		t.Fatalf("create other-CPO charger: %v", err)
	}
	response, err = service.ListCustomerChargerChargeability(ctx, fixture.firstPrincipal, []string{fixture.charger.ChargerID, visibleSecond.ChargerID, other.ChargerID})
	if err != nil || len(response.Chargers) != 1 || response.Chargers[0].ChargerID != fixture.charger.ChargerID {
		t.Fatalf("cross-CPO projection=%+v err=%v, want only visible charger", response, err)
	}
}

func requireCustomerChargeability(t *testing.T, service *Service, principal Principal, chargerID string) CustomerChargeabilityResponse {
	t.Helper()
	response, err := service.ListCustomerChargerChargeability(context.Background(), principal, []string{chargerID})
	if err != nil {
		t.Fatalf("get chargeability: %v", err)
	}
	if len(response.Chargers) != 1 {
		t.Fatalf("chargeability response=%+v, want one charger", response)
	}
	return response
}

func assertCustomerConnectorChargeability(t *testing.T, response CustomerChargeabilityResponse, connectorID uuid.UUID, wantCanCharge bool, wantReason string) {
	t.Helper()
	for _, connector := range response.Chargers[0].Connectors {
		if connector.ConnectorID == connectorID {
			if connector.CanCharge != wantCanCharge || connector.ChargeabilityReason != wantReason {
				t.Fatalf("connector chargeability=%+v, want can_charge=%t reason=%s", connector, wantCanCharge, wantReason)
			}
			return
		}
	}
	t.Fatalf("connector %s missing from chargeability response=%+v", connectorID, response)
}
