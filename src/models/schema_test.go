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
		&CPOIntegration{},
	}

	cache := &sync.Map{}
	for _, model := range models {
		if _, err := schema.Parse(model, cache, schema.NamingStrategy{}); err != nil {
			t.Errorf("parse GORM model %T: %v", model, err)
		}
	}
}
