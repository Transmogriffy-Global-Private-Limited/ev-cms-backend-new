package models

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestJSONBScanAndValue(t *testing.T) {
	t.Parallel()

	var value JSONB
	if err := value.Scan(`{"enabled":true}`); err != nil {
		t.Fatalf("scan JSON string: %v", err)
	}
	if !reflect.DeepEqual(value, JSONB{"enabled": true}) {
		t.Fatalf("unexpected scanned JSON: %#v", value)
	}

	driverValue, err := value.Value()
	if err != nil {
		t.Fatalf("encode JSON value: %v", err)
	}
	if string(driverValue.([]byte)) != `{"enabled":true}` {
		t.Fatalf("unexpected encoded JSON: %s", driverValue)
	}
}

func TestJSONBScanRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	var value JSONB
	if err := value.Scan(42); err == nil {
		t.Fatal("expected unsupported JSONB source type to fail")
	}
}

func TestGORMModelsParse(t *testing.T) {
	t.Parallel()

	models := []any{
		&User{},
		&UserSetting{},
		&PlatformAdmin{},
		&CPO{},
		&CPOMembership{},
		&UserGroup{},
		&Customer{},
		&Hub{},
		&Charger{},
		&Connector{},
		&UserGroupHub{},
		&UserGroupCharger{},
		&CustomerFavoriteHub{},
		&CustomerFavoriteCharger{},
		&GST{},
		&Tariff{},
		&Wallet{},
		&ChargingSession{},
		&WalletTransaction{},
		&Payment{},
		&AuditLog{},
		&AuthChallenge{},
		&AuthSession{},
		&AuthRefreshToken{},
		&MailOutbox{},
		&AuthRateLimit{},
		&CustomerSignupChallenge{},
		&CPOIntegration{},
		&PlatformEvent{},
		&OperationalEvent{},
		&WorkerInstance{},
		&PlatformAnnouncement{},
		&PlatformNotification{},
		&SubscriptionPlan{},
		&SubscriptionPlanVersion{},
		&CPOSubscription{},
		&CPOSubscriptionHistory{},
		&HALChargerRuntime{},
		&HALConnectorRuntime{},
	}

	cache := &sync.Map{}
	for _, model := range models {
		if _, err := schema.Parse(model, cache, schema.NamingStrategy{}); err != nil {
			t.Errorf("parse GORM model %T: %v", model, err)
		}
	}
}

func TestHALRuntimeTableNamesMatchMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model any
		want  string
	}{
		{model: &HALChargerRuntime{}, want: "hal_charger_runtime"},
		{model: &HALConnectorRuntime{}, want: "hal_connector_runtime"},
	}

	for _, test := range tests {
		parsed, err := schema.Parse(test.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse GORM model %T: %v", test.model, err)
		}
		if parsed.Table != test.want {
			t.Errorf("model %T maps to table %q, want %q", test.model, parsed.Table, test.want)
		}
	}
}

func TestGORMColumnMappingsMatchMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model    any
		field    string
		database string
	}{
		{model: &User{}, field: "MFAEnabled", database: "mfa_enabled"},
		{model: &CPO{}, field: "GSTIN", database: "gstin"},
		{model: &CPO{}, field: "AppID", database: "app_id"},
		{model: &CPO{}, field: "AppIDMode", database: "app_id_mode"},
		{model: &CPO{}, field: "AppIDUpdatedAt", database: "app_id_updated_at"},
		{model: &Hub{}, field: "Open24Hours", database: "open_24_hours"},
		{model: &Charger{}, field: "OCPPIdentity", database: "ocpp_identity"},
		{model: &Charger{}, field: "MaxPowerKW", database: "max_power_kw"},
		{model: &Charger{}, field: "OCPPVersion", database: "ocpp_version"},
		{model: &GST{}, field: "SGSTRate", database: "sgst_rate"},
		{model: &GST{}, field: "CGSTRate", database: "cgst_rate"},
		{model: &GST{}, field: "IGSTRate", database: "igst_rate"},
		{model: &Tariff{}, field: "PricePerUnit", database: "price_per_unit"},
		{model: &Tariff{}, field: "AssignedTo", database: "assigned_to"},
		{model: &ChargingSession{}, field: "StartIntentID", database: "start_intent_id"},
		{model: &ChargingSession{}, field: "HALTransactionID", database: "hal_transaction_id"},
		{model: &ChargingSession{}, field: "MeterStartWh", database: "meter_start_wh"},
		{model: &ChargingSession{}, field: "MeterStopWh", database: "meter_stop_wh"},
		{model: &ChargingSession{}, field: "LatestMeterWh", database: "latest_meter_wh"},
		{model: &ChargingSession{}, field: "TotalKWh", database: "total_kwh"},
		{model: &HALCommandRecord{}, field: "CMSCommandID", database: "cms_command_id"},
		{model: &HALCommandRecord{}, field: "HALCommandID", database: "hal_command_id"},
		{model: &HALFactReceipt{}, field: "FactID", database: "fact_id"},
		{model: &HALChargerMapping{}, field: "CMSChargerID", database: "cms_charger_id"},
		{model: &HALChargerMapping{}, field: "ChargerOCPPIdentity", database: "charger_ocpp_identity"},
		{model: &HALChargerRuntime{}, field: "CMSChargerID", database: "cms_charger_id"},
		{model: &HALConnectorRuntime{}, field: "OCPPConnectorStatus", database: "ocpp_connector_status"},
		{model: &AuthChallenge{}, field: "RequestIP", database: "request_ip"},
		{model: &AuthSession{}, field: "IPAddress", database: "ip_address"},
		{model: &CPOIntegration{}, field: "EncryptionKeyID", database: "encryption_key_id"},
		{model: &WalletRechargeOrder{}, field: "ProviderOrderID", database: "provider_order_id"},
		{model: &WalletRechargePayment{}, field: "ProviderPaymentID", database: "provider_payment_id"},
		{model: &WalletRechargeRefund{}, field: "ProviderRefundID", database: "provider_refund_id"},
	}

	for _, test := range tests {
		parsed, err := schema.Parse(test.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse GORM model %T: %v", test.model, err)
		}
		field := parsed.LookUpField(test.field)
		if field == nil {
			t.Fatalf("model %T has no field %s", test.model, test.field)
		}
		if field.DBName != test.database {
			t.Errorf(
				"model %T field %s maps to %q, want %q",
				test.model,
				test.field,
				field.DBName,
				test.database,
			)
		}
	}
}

type dryRunConn struct{}

func (dryRunConn) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("dry-run connection must not prepare statements")
}

func (dryRunConn) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("dry-run connection must not execute statements")
}

func (dryRunConn) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("dry-run connection must not query")
}

func (dryRunConn) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func TestChargingSessionPersistenceUsesMigrationTotalKWhColumn(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: dryRunConn{}}), &gorm.Config{
		DryRun:                 true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open dry-run postgres GORM database: %v", err)
	}

	tx := db.Create(&ChargingSession{})
	if tx.Error != nil {
		t.Fatalf("build ChargingSession insert: %v", tx.Error)
	}
	statement := tx.Statement.SQL.String()
	if !strings.Contains(statement, `"total_kwh"`) {
		t.Fatalf("ChargingSession insert does not target total_kwh: %s", statement)
	}
	if strings.Contains(statement, `"total_k_wh"`) {
		t.Fatalf("ChargingSession insert targets nonexistent total_k_wh: %s", statement)
	}
}
