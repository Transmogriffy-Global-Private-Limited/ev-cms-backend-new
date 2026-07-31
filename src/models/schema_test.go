package models

import (
	"reflect"
	"sync"
	"testing"

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
		&WorkerInstance{},
	}

	cache := &sync.Map{}
	for _, model := range models {
		if _, err := schema.Parse(model, cache, schema.NamingStrategy{}); err != nil {
			t.Errorf("parse GORM model %T: %v", model, err)
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
		{model: &Hub{}, field: "Open24Hours", database: "open_24_hours"},
		{model: &Charger{}, field: "OCPPIdentity", database: "ocpp_identity"},
		{model: &Charger{}, field: "MaxPowerKW", database: "max_power_kw"},
		{model: &Charger{}, field: "OCPPVersion", database: "ocpp_version"},
		{model: &GST{}, field: "SGSTRate", database: "sgst_rate"},
		{model: &GST{}, field: "CGSTRate", database: "cgst_rate"},
		{model: &GST{}, field: "IGSTRate", database: "igst_rate"},
		{model: &Tariff{}, field: "PricePerKWh", database: "price_per_kwh"},
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
