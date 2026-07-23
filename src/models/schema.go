package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type JSONB map[string]any

func (value JSONB) Value() (driver.Value, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

func (value *JSONB) Scan(source any) error {
	var raw []byte
	switch typed := source.(type) {
	case nil:
		*value = JSONB{}
		return nil
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("scan JSONB from unsupported type %T", source)
	}

	if len(raw) == 0 {
		*value = JSONB{}
		return nil
	}
	return json.Unmarshal(raw, value)
}

// User is a login identity. Platform, CPO staff, and CPO customer authority are
// separate relationships.
type User struct {
	ID                  uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email               string          `gorm:"type:varchar(320);not null" json:"email"`
	PasswordHash        string          `gorm:"type:varchar(255);not null" json:"-"`
	FullName            string          `gorm:"type:varchar(255);not null" json:"full_name"`
	Phone               *string         `gorm:"type:varchar(32)" json:"phone,omitempty"`
	IsActive            bool            `gorm:"not null;default:true" json:"is_active"`
	IsVerified          bool            `gorm:"not null;default:false" json:"is_verified"`
	MFAEnabled          bool            `gorm:"not null;default:false" json:"mfa_enabled"`
	MustChangePassword  bool            `gorm:"not null;default:false" json:"must_change_password"`
	PasswordChangedAt   time.Time       `gorm:"not null" json:"password_changed_at"`
	FailedLoginAttempts int             `gorm:"not null;default:0" json:"-"`
	LockedUntil         *time.Time      `json:"-"`
	LastLoginAt         *time.Time      `json:"last_login_at,omitempty"`
	Settings            *UserSetting    `gorm:"foreignKey:UserID" json:"settings,omitempty"`
	PlatformAdmin       *PlatformAdmin  `gorm:"foreignKey:UserID" json:"platform_admin,omitempty"`
	CPOMemberships      []CPOMembership `gorm:"foreignKey:UserID" json:"cpo_memberships,omitempty"`
	Customers           []Customer      `gorm:"foreignKey:UserID" json:"customers,omitempty"`
	CreatedAt           time.Time       `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time       `gorm:"not null" json:"updated_at"`
}

type UserSetting struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Settings  JSONB     `gorm:"type:jsonb;not null;default:'{}'" json:"settings"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

// PlatformAdmin explicitly grants platform-superadmin authority.
type PlatformAdmin struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user,omitempty"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

// CPO is the tenant organization and data boundary. It carries
// the business-profile fields that were previously modeled as CPOProfile.
type CPO struct {
	ID             uuid.UUID                `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug           string                   `gorm:"type:varchar(80);not null" json:"slug"`
	BusinessName   string                   `gorm:"type:varchar(255);not null" json:"business_name"`
	CompanyType    constants.CPOCompanyType `gorm:"type:varchar(20);not null" json:"company_type"`
	GSTIN          *string                  `gorm:"type:varchar(15)" json:"gstin,omitempty"`
	Address        string                   `gorm:"type:text;not null;default:''" json:"address"`
	City           string                   `gorm:"type:varchar(100);not null;default:''" json:"city"`
	State          string                   `gorm:"type:varchar(100);not null;default:''" json:"state"`
	Pincode        string                   `gorm:"type:varchar(10);not null;default:''" json:"pincode"`
	Status         constants.CPOStatus      `gorm:"type:varchar(20);not null;default:'PENDING'" json:"status"`
	AppID          string                   `gorm:"type:varchar(100);not null;uniqueIndex" json:"app_id"`
	AppIDMode      constants.CPOAppIDMode   `gorm:"type:varchar(20);not null;default:'DUMMY'" json:"app_id_mode"`
	AppIDUpdatedAt time.Time                `gorm:"not null" json:"app_id_updated_at"`
	Memberships    []CPOMembership          `gorm:"foreignKey:CPOID" json:"memberships,omitempty"`
	UserGroups     []UserGroup              `gorm:"foreignKey:CPOID" json:"user_groups,omitempty"`
	Customers      []Customer               `gorm:"foreignKey:CPOID" json:"customers,omitempty"`
	Hubs           []Hub                    `gorm:"foreignKey:CPOID" json:"hubs,omitempty"`
	GSTProfiles    []GST                    `gorm:"foreignKey:CPOID" json:"gst_profiles,omitempty"`
	Tariffs        []Tariff                 `gorm:"foreignKey:CPOID" json:"tariffs,omitempty"`
	CreatedAt      time.Time                `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time                `gorm:"not null" json:"updated_at"`
}

func (CPO) TableName() string {
	return "cpos"
}

// CPOMembership grants one fixed CPO-wide staff role to a user.
type CPOMembership struct {
	ID        uuid.UUID                  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID     uuid.UUID                  `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_membership,priority:1;index" json:"cpo_id"`
	CPO       CPO                        `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	UserID    uuid.UUID                  `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_membership,priority:2;index" json:"user_id"`
	User      User                       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user,omitempty"`
	Role      constants.CPORole          `gorm:"type:varchar(20);not null" json:"role"`
	Status    constants.MembershipStatus `gorm:"type:varchar(20);not null;default:'ACTIVE'" json:"status"`
	CreatedAt time.Time                  `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time                  `gorm:"not null" json:"updated_at"`
}

type UserGroup struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID        uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Name         string    `gorm:"type:varchar(100);not null" json:"name"`
	Description  string    `gorm:"type:text;not null;default:''" json:"description"`
	IsActive     bool      `gorm:"not null;default:true" json:"is_active"`
	Customers    []Customer
	Tariffs      []Tariff
	HubLinks     []UserGroupHub     `gorm:"foreignKey:UserGroupID" json:"hub_links,omitempty"`
	ChargerLinks []UserGroupCharger `gorm:"foreignKey:UserGroupID" json:"charger_links,omitempty"`
	CreatedAt    time.Time          `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time          `gorm:"not null" json:"updated_at"`
}

// Customer is the relationship between a login identity and a CPO's charging
// business. It replaces the global APP_USER role while preserving app-user data.
type Customer struct {
	ID               uuid.UUID                 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID            uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_customer,priority:1;index" json:"cpo_id"`
	CPO              CPO                       `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	UserID           uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_customer,priority:2;index" json:"user_id"`
	User             User                      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user,omitempty"`
	UserGroupID      *uuid.UUID                `gorm:"type:uuid;index" json:"user_group_id,omitempty"`
	UserGroup        *UserGroup                `gorm:"foreignKey:UserGroupID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_group,omitempty"`
	Status           constants.CustomerStatus  `gorm:"type:varchar(20);not null;default:'ACTIVE'" json:"status"`
	Wallet           *Wallet                   `gorm:"foreignKey:CustomerID" json:"wallet,omitempty"`
	FavoriteHubs     []CustomerFavoriteHub     `gorm:"foreignKey:CustomerID" json:"favorite_hubs,omitempty"`
	FavoriteChargers []CustomerFavoriteCharger `gorm:"foreignKey:CustomerID" json:"favorite_chargers,omitempty"`
	CreatedAt        time.Time                 `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time                 `gorm:"not null" json:"updated_at"`
}

type Hub struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID       uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CPO         CPO       `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Address     string    `gorm:"type:text;not null" json:"address"`
	Latitude    float64   `gorm:"type:numeric(10,8);not null" json:"latitude"`
	Longitude   float64   `gorm:"type:numeric(11,8);not null" json:"longitude"`
	Open24Hours bool      `gorm:"not null;default:true" json:"open_24_hours"`
	Chargers    []Charger `gorm:"foreignKey:HubID" json:"chargers,omitempty"`
	Tariffs     []Tariff  `gorm:"foreignKey:HubID" json:"tariffs,omitempty"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

type Charger struct {
	ID           uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID        uuid.UUID               `gorm:"type:uuid;not null;index" json:"cpo_id"`
	HubID        uuid.UUID               `gorm:"type:uuid;not null;index" json:"hub_id"`
	Hub          Hub                     `gorm:"foreignKey:HubID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"hub,omitempty"`
	ChargerID    string                  `gorm:"type:varchar(6);not null" json:"charger_id"`
	OCPPIdentity string                  `gorm:"type:varchar(255);not null" json:"ocpp_identity"`
	Vendor       string                  `gorm:"type:varchar(100);not null;default:''" json:"vendor"`
	Model        string                  `gorm:"type:varchar(100);not null;default:''" json:"model"`
	SerialNumber string                  `gorm:"type:varchar(100);not null;default:''" json:"serial_number"`
	MaxPowerKW   float64                 `gorm:"type:numeric(8,2);not null;default:0" json:"max_power_kw"`
	Status       constants.ChargerStatus `gorm:"type:varchar(30);not null;default:'OFFLINE'" json:"status"`
	OCPPVersion  string                  `gorm:"type:varchar(20);not null;default:'1.6J'" json:"ocpp_version"`
	LastSeenAt   *time.Time              `json:"last_seen_at,omitempty"`
	Connectors   []Connector             `gorm:"foreignKey:ChargerID" json:"connectors,omitempty"`
	Tariffs      []Tariff                `gorm:"foreignKey:ChargerID" json:"tariffs,omitempty"`
	CreatedAt    time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time               `gorm:"not null" json:"updated_at"`
}

type Connector struct {
	ID              uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID           uuid.UUID               `gorm:"type:uuid;not null;index" json:"cpo_id"`
	ChargerID       uuid.UUID               `gorm:"type:uuid;not null;index" json:"charger_id"`
	Charger         Charger                 `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`
	ConnectorNumber int                     `gorm:"not null" json:"connector_number"`
	ConnectorType   string                  `gorm:"type:varchar(50);not null" json:"connector_type"`
	MaxCurrent      float64                 `gorm:"type:numeric(8,2);not null;default:0" json:"max_current"`
	MaxVoltage      float64                 `gorm:"type:numeric(8,2);not null;default:0" json:"max_voltage"`
	Status          constants.ChargerStatus `gorm:"type:varchar(30);not null;default:'AVAILABLE'" json:"status"`
	CreatedAt       time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time               `gorm:"not null" json:"updated_at"`
}

type UserGroupHub struct {
	CPOID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"cpo_id"`
	UserGroupID uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_group_id"`
	HubID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"hub_id"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}

type UserGroupCharger struct {
	CPOID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"cpo_id"`
	UserGroupID uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_group_id"`
	ChargerID   uuid.UUID `gorm:"type:uuid;primaryKey" json:"charger_id"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}

type CustomerFavoriteHub struct {
	CPOID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"cpo_id"`
	CustomerID uuid.UUID `gorm:"type:uuid;primaryKey" json:"customer_id"`
	HubID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"hub_id"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
}

type CustomerFavoriteCharger struct {
	CPOID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"cpo_id"`
	CustomerID uuid.UUID `gorm:"type:uuid;primaryKey" json:"customer_id"`
	ChargerID  uuid.UUID `gorm:"type:uuid;primaryKey" json:"charger_id"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
}

type GST struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID     uuid.UUID       `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Name      string          `gorm:"type:varchar(100);not null" json:"name"`
	SGSTRate  decimal.Decimal `gorm:"type:numeric(5,2);not null;default:9.00" json:"sgst_rate"`
	CGSTRate  decimal.Decimal `gorm:"type:numeric(5,2);not null;default:9.00" json:"cgst_rate"`
	IGSTRate  decimal.Decimal `gorm:"type:numeric(5,2);not null;default:18.00" json:"igst_rate"`
	IsActive  bool            `gorm:"not null;default:true" json:"is_active"`
	Tariffs   []Tariff        `gorm:"foreignKey:GSTID" json:"tariffs,omitempty"`
	CreatedAt time.Time       `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time       `gorm:"not null" json:"updated_at"`
}

func (GST) TableName() string {
	return "gsts"
}

type Tariff struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"cpo_id"`
	HubID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"hub_id"`
	Hub           Hub             `gorm:"foreignKey:HubID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"hub,omitempty"`
	ChargerID     *uuid.UUID      `gorm:"type:uuid;index" json:"charger_id,omitempty"`
	Charger       *Charger        `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`
	GSTID         *uuid.UUID      `gorm:"type:uuid;index" json:"gst_id,omitempty"`
	GST           *GST            `gorm:"foreignKey:GSTID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"gst,omitempty"`
	UserGroupID   *uuid.UUID      `gorm:"type:uuid;index" json:"user_group_id,omitempty"`
	UserGroup     *UserGroup      `gorm:"foreignKey:UserGroupID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_group,omitempty"`
	PricePerKWh   decimal.Decimal `gorm:"type:numeric(12,4);not null" json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal `gorm:"type:numeric(12,4);not null;default:0" json:"idle_fee_per_min"`
	Currency      string          `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	IsActive      bool            `gorm:"not null;default:true" json:"is_active"`
	CreatedAt     time.Time       `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"not null" json:"updated_at"`
}

type Wallet struct {
	ID           uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID   uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex" json:"customer_id"`
	Customer     Customer        `gorm:"foreignKey:CustomerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"customer,omitempty"`
	Balance      decimal.Decimal `gorm:"type:numeric(14,2);not null;default:0" json:"balance"`
	Currency     string          `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	Transactions []WalletTransaction
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

type ChargingSession struct {
	ID                 uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID              uuid.UUID               `gorm:"type:uuid;not null;index" json:"cpo_id"`
	TransactionID      int32                   `gorm:"type:integer;not null;index" json:"transaction_id"`
	CustomerID         uuid.UUID               `gorm:"type:uuid;not null;index" json:"customer_id"`
	Customer           Customer                `gorm:"foreignKey:CustomerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"customer,omitempty"`
	ChargerID          uuid.UUID               `gorm:"type:uuid;not null;index" json:"charger_id"`
	Charger            Charger                 `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`
	ConnectorID        uuid.UUID               `gorm:"type:uuid;not null;index" json:"connector_id"`
	Connector          Connector               `gorm:"foreignKey:ConnectorID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"connector,omitempty"`
	TariffID           uuid.UUID               `gorm:"type:uuid;not null;index" json:"tariff_id"`
	Tariff             Tariff                  `gorm:"foreignKey:TariffID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"tariff,omitempty"`
	StartTime          time.Time               `gorm:"not null" json:"start_time"`
	EndTime            *time.Time              `json:"end_time,omitempty"`
	MeterStartWh       int64                   `gorm:"not null" json:"meter_start_wh"`
	MeterStopWh        *int64                  `json:"meter_stop_wh,omitempty"`
	TotalKWh           decimal.Decimal         `gorm:"type:numeric(14,3);not null;default:0" json:"total_kwh"`
	TotalAmount        decimal.Decimal         `gorm:"type:numeric(14,2);not null;default:0" json:"total_amount"`
	Currency           string                  `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	StopReason         *string                 `gorm:"type:varchar(50)" json:"stop_reason,omitempty"`
	TariffSnapshot     JSONB                   `gorm:"type:jsonb;not null;default:'{}'" json:"tariff_snapshot"`
	TaxSnapshot        JSONB                   `gorm:"type:jsonb;not null;default:'{}'" json:"tax_snapshot"`
	Status             constants.SessionStatus `gorm:"type:varchar(30);not null;default:'ACTIVE'" json:"status"`
	WalletTransactions []WalletTransaction     `gorm:"foreignKey:SessionID" json:"wallet_transactions,omitempty"`
	Payment            *Payment                `gorm:"foreignKey:SessionID" json:"payment,omitempty"`
	CreatedAt          time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time               `gorm:"not null" json:"updated_at"`
}

type WalletTransaction struct {
	ID              uuid.UUID                       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID           uuid.UUID                       `gorm:"type:uuid;not null;index" json:"cpo_id"`
	WalletID        uuid.UUID                       `gorm:"type:uuid;not null;index" json:"wallet_id"`
	Wallet          Wallet                          `gorm:"foreignKey:WalletID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"wallet,omitempty"`
	SessionID       *uuid.UUID                      `gorm:"type:uuid;index" json:"session_id,omitempty"`
	Session         *ChargingSession                `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"session,omitempty"`
	Amount          decimal.Decimal                 `gorm:"type:numeric(14,2);not null" json:"amount"`
	TransactionType constants.WalletTransactionType `gorm:"type:varchar(20);not null" json:"transaction_type"`
	Description     string                          `gorm:"type:varchar(255);not null;default:''" json:"description"`
	IdempotencyKey  *string                         `gorm:"type:varchar(100)" json:"idempotency_key,omitempty"`
	Status          constants.FinancialStatus       `gorm:"type:varchar(20);not null;default:'PENDING'" json:"status"`
	CreatedAt       time.Time                       `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time                       `gorm:"not null" json:"updated_at"`
}

type Payment struct {
	ID                  uuid.UUID                 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID               uuid.UUID                 `gorm:"type:uuid;not null;index" json:"cpo_id"`
	SessionID           uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex" json:"session_id"`
	Session             ChargingSession           `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"session,omitempty"`
	WalletTransactionID uuid.UUID                 `gorm:"type:uuid;not null;uniqueIndex" json:"wallet_transaction_id"`
	WalletTransaction   WalletTransaction         `gorm:"foreignKey:WalletTransactionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"wallet_transaction,omitempty"`
	Amount              decimal.Decimal           `gorm:"type:numeric(14,2);not null" json:"amount"`
	PaymentMethod       string                    `gorm:"type:varchar(20);not null;default:'WALLET'" json:"payment_method"`
	Status              constants.FinancialStatus `gorm:"type:varchar(20);not null;default:'PENDING'" json:"status"`
	CreatedAt           time.Time                 `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time                 `gorm:"not null" json:"updated_at"`
}

type AuditLog struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID     *uuid.UUID `gorm:"type:uuid;index" json:"cpo_id,omitempty"`
	UserID    *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`
	Action    string     `gorm:"type:varchar(100);not null" json:"action"`
	Entity    string     `gorm:"type:varchar(100);not null" json:"entity"`
	EntityID  *uuid.UUID `gorm:"type:uuid" json:"entity_id,omitempty"`
	Details   JSONB      `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
}
