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

// User is an administrative login identity for platform and CPO staff.
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
	UserID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"user_id"`
	User              User       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user,omitempty"`
	IsActive          bool       `gorm:"not null;default:true" json:"is_active"`
	StatusReason      string     `gorm:"type:varchar(500);not null" json:"status_reason"`
	StatusChangedAt   time.Time  `gorm:"not null" json:"status_changed_at"`
	StatusChangedByID *uuid.UUID `gorm:"column:status_changed_by_user_id;type:uuid;index" json:"status_changed_by_user_id,omitempty"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

// CPO is the tenant organization and data boundary. It carries
// the business-profile fields that were previously modeled as CPOProfile.
type CPO struct {
	ID                    uuid.UUID                `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug                  string                   `gorm:"type:varchar(80);not null" json:"slug"`
	BusinessName          string                   `gorm:"type:varchar(255);not null" json:"business_name"`
	CompanyType           constants.CPOCompanyType `gorm:"type:varchar(20);not null" json:"company_type"`
	GSTIN                 string                   `gorm:"type:varchar(15);not null" json:"gstin"`
	Address               string                   `gorm:"type:text;not null" json:"address"`
	City                  string                   `gorm:"type:varchar(100);not null" json:"city"`
	State                 string                   `gorm:"type:varchar(100);not null" json:"state"`
	Pincode               string                   `gorm:"type:varchar(10);not null" json:"pincode"`
	Status                constants.CPOStatus      `gorm:"type:varchar(20);not null;default:'PENDING'" json:"status"`
	StatusReason          string                   `gorm:"type:varchar(500);not null" json:"status_reason"`
	StatusChangedAt       time.Time                `gorm:"not null" json:"status_changed_at"`
	StatusChangedByUserID *uuid.UUID               `gorm:"type:uuid;index" json:"status_changed_by_user_id,omitempty"`
	AppID                 string                   `gorm:"type:varchar(100);not null;uniqueIndex" json:"app_id"`
	AppIDMode             constants.CPOAppIDMode   `gorm:"type:varchar(20);not null;default:'DUMMY'" json:"app_id_mode"`
	AppIDUpdatedAt        time.Time                `gorm:"not null" json:"app_id_updated_at"`
	Memberships           []CPOMembership          `gorm:"foreignKey:CPOID" json:"memberships,omitempty"`
	UserGroups            []UserGroup              `gorm:"foreignKey:CPOID" json:"user_groups,omitempty"`
	Customers             []Customer               `gorm:"foreignKey:CPOID" json:"customers,omitempty"`
	Hubs                  []Hub                    `gorm:"foreignKey:CPOID" json:"hubs,omitempty"`
	GSTProfiles           []GST                    `gorm:"foreignKey:CPOID" json:"gst_profiles,omitempty"`
	Tariffs               []Tariff                 `gorm:"foreignKey:CPOID" json:"tariffs,omitempty"`
	CreatedAt             time.Time                `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time                `gorm:"not null" json:"updated_at"`
}

func (CPO) TableName() string {
	return "cpos"
}

// CPOMembership grants one fixed CPO-wide staff role to a user.
type CPOMembership struct {
	ID             uuid.UUID                  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID          uuid.UUID                  `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_membership,priority:1;index" json:"cpo_id"`
	CPO            CPO                        `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	UserID         uuid.UUID                  `gorm:"type:uuid;not null;uniqueIndex:uq_cpo_membership,priority:2;index" json:"user_id"`
	User           User                       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user,omitempty"`
	Role           constants.CPORole          `gorm:"type:varchar(20);not null" json:"role"`
	Status         constants.MembershipStatus `gorm:"type:varchar(20);not null;default:'ACTIVE'" json:"status"`
	IsPrimaryAdmin bool                       `gorm:"not null;default:false" json:"is_primary_admin"`
	CreatedAt      time.Time                  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time                  `gorm:"not null" json:"updated_at"`
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

// Customer is a CPO-local app-user account. It owns its credentials and must
// never share authentication state with a global administrative User.
type Customer struct {
	ID                  uuid.UUID                 `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID               uuid.UUID                 `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CPO                 CPO                       `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	Email               string                    `gorm:"type:varchar(320);not null" json:"email"`
	PasswordHash        string                    `gorm:"type:varchar(255);not null" json:"-"`
	FullName            string                    `gorm:"type:varchar(255);not null" json:"full_name"`
	Phone               *string                   `gorm:"type:varchar(32)" json:"phone,omitempty"`
	IsVerified          bool                      `gorm:"not null;default:false" json:"is_verified"`
	FailedLoginAttempts int                       `gorm:"not null;default:0" json:"-"`
	LockedUntil         *time.Time                `gorm:"type:timestamptz" json:"-"`
	PasswordChangedAt   time.Time                 `gorm:"type:timestamptz;not null" json:"password_changed_at"`
	LastLoginAt         *time.Time                `gorm:"type:timestamptz" json:"last_login_at,omitempty"`
	UserGroupID         *uuid.UUID                `gorm:"type:uuid;index" json:"user_group_id,omitempty"`
	UserGroup           *UserGroup                `gorm:"foreignKey:UserGroupID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_group,omitempty"`
	Status              constants.CustomerStatus  `gorm:"type:varchar(20);not null;default:'ACTIVE'" json:"status"`
	Wallet              *Wallet                   `gorm:"foreignKey:CustomerID" json:"wallet,omitempty"`
	FavoriteHubs        []CustomerFavoriteHub     `gorm:"foreignKey:CustomerID" json:"favorite_hubs,omitempty"`
	FavoriteChargers    []CustomerFavoriteCharger `gorm:"foreignKey:CustomerID" json:"favorite_chargers,omitempty"`
	CreatedAt           time.Time                 `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time                 `gorm:"not null" json:"updated_at"`
}

type Hub struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID           uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CPO             CPO       `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	Address         string    `gorm:"type:text;not null" json:"address"`
	State           string    `gorm:"not null;size:100"`
	Latitude        float64   `gorm:"type:numeric(10,8);not null" json:"latitude"`
	Longitude       float64   `gorm:"type:numeric(11,8);not null" json:"longitude"`
	Open24Hours     bool      `gorm:"column:open_24_hours;not null;default:true" json:"open_24_hours"`
	SanctionLoad    float64   `gorm:"type:numeric(10,2);not null;default:0" json:"sanction_load"`
	CustomerVisible bool      `gorm:"column:customer_visible;not null;default:false" json:"customer_visible"`
	Chargers        []Charger `gorm:"foreignKey:HubID" json:"chargers,omitempty"`
	Tariffs         []Tariff  `gorm:"foreignKey:HubID" json:"tariffs,omitempty"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

type Charger struct {
	ID                  uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID               uuid.UUID               `gorm:"type:uuid;not null;index" json:"cpo_id"`
	HubID               *uuid.UUID              `gorm:"type:uuid;index" json:"hub_id,omitempty"`
	Hub                 *Hub                    `gorm:"foreignKey:HubID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"hub,omitempty"`
	ChargerID           string                  `gorm:"type:varchar(6);not null" json:"charger_id"`
	OCPPIdentity        string                  `gorm:"type:varchar(255);not null" json:"ocpp_identity"`
	Vendor              *string                 `gorm:"type:varchar(100)" json:"vendor"`
	Model               *string                 `gorm:"type:varchar(100)" json:"model"`
	SerialNumber        string                  `gorm:"type:varchar(100);not null;default:''" json:"serial_number"`
	MaxPowerKW          float64                 `gorm:"type:numeric(8,2);not null;default:0" json:"max_power_kw"`
	Status              constants.ChargerStatus `gorm:"type:varchar(30);not null;default:'INACTIVE'" json:"status"`
	OCPPVersion         string                  `gorm:"type:varchar(20);not null;default:'1.6J'" json:"ocpp_version"`
	LastSeenAt          *time.Time              `json:"last_seen_at,omitempty"`
	ChargerName         string                  `gorm:"type:varchar(255);not null;default:''" json:"charger_name"`
	ChargerHostName     string                  `gorm:"type:varchar(255);not null;default:''" json:"charger_host_name"`
	ChargerHostPhoneNo  string                  `gorm:"type:varchar(20);not null;default:''" json:"charger_host_phone_no"`
	ChargerType         string                  `gorm:"type:varchar(100);not null;default:''" json:"charger_type"`
	Segment             string                  `gorm:"type:varchar(100);not null;default:''" json:"segment"`
	SubSegment          string                  `gorm:"type:varchar(100);not null;default:''" json:"sub_segment"`
	ChargerImage        string                  `gorm:"type:text;not null;default:''" json:"charger_image"`
	ChargerUseType      string                  `gorm:"type:varchar(100);not null;default:''" json:"charger_use_type"`
	NumberOfConnectors  int                     `gorm:"not null;default:1" json:"number_of_connectors"`
	Parking             string                  `gorm:"type:varchar(100);not null;default:''" json:"parking"`
	Protocol            string                  `gorm:"type:varchar(50);not null;default:'OCPP 1.6J'" json:"protocol"`
	TwentyFourSevenOpen bool                    `gorm:"column:twenty_four_seven_open_status;not null;default:false" json:"twenty_four_seven_open_status"`
	Connectors          []Connector             `gorm:"foreignKey:ChargerID" json:"connectors,omitempty"`
	Tariffs             []Tariff                `gorm:"foreignKey:ChargerID" json:"tariffs,omitempty"`
	CreatedAt           time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time               `gorm:"not null" json:"updated_at"`
}

type Connector struct {
	ID                     uuid.UUID               `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID                  uuid.UUID               `gorm:"type:uuid;not null;index" json:"cpo_id"`
	ChargerID              uuid.UUID               `gorm:"type:uuid;not null;index" json:"charger_id"`
	Charger                Charger                 `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`
	ConnectorNumber        int                     `gorm:"not null" json:"connector_number"`
	ConnectorType          string                  `gorm:"type:varchar(50);not null" json:"connector_type"`
	ConnectorTotalCapacity float64                 `gorm:"type:numeric(10,2);not null;default:0" json:"connector_total_capacity"`
	Status                 constants.ChargerStatus `gorm:"type:varchar(30);not null;default:'INACTIVE'" json:"status"`
	CreatedAt              time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt              time.Time               `gorm:"not null" json:"updated_at"`
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
	ID        uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID     uuid.UUID        `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Name      string           `gorm:"type:varchar(100);not null" json:"name"`
	State     string           `gorm:"type:varchar(255)" json:"state"`
	SGSTRate  *decimal.Decimal `gorm:"type:numeric(5,2)" json:"sgst_rate"`
	CGSTRate  *decimal.Decimal `gorm:"type:numeric(5,2)" json:"cgst_rate"`
	IGSTRate  *decimal.Decimal `gorm:"type:numeric(5,2)" json:"igst_rate"`
	IsActive  bool             `gorm:"not null;default:true" json:"is_active"`
	Tariffs   []Tariff         `gorm:"foreignKey:GSTID" json:"tariffs,omitempty"`
	CreatedAt time.Time        `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time        `gorm:"not null" json:"updated_at"`
}

func (GST) TableName() string {
	return "gsts"
}

type Tariff struct {
	ID            uuid.UUID                    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID         uuid.UUID                    `gorm:"type:uuid;not null;index" json:"cpo_id"`
	HubID         uuid.UUID                    `gorm:"type:uuid;not null;index" json:"hub_id"`
	Hub           Hub                          `gorm:"foreignKey:HubID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"hub,omitempty"`
	Assigned      constants.TariffAssignedType `gorm:"column:assigned;type:enum('usergroup','hub','charger');not null"`
	ChargerID     *uuid.UUID                   `gorm:"type:uuid;index" json:"charger_id,omitempty"`
	Charger       *Charger                     `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`
	GSTID         *uuid.UUID                   `gorm:"type:uuid;index" json:"gst_id,omitempty"`
	GST           *GST                         `gorm:"foreignKey:GSTID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"gst,omitempty"`
	UserGroupID   *uuid.UUID                   `gorm:"type:uuid;index" json:"user_group_id,omitempty"`
	UserGroup     *UserGroup                   `gorm:"foreignKey:UserGroupID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_group,omitempty"`
	PricePerKWh   decimal.Decimal              `gorm:"column:price_per_kwh;type:numeric(12,4);not null" json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal              `gorm:"type:numeric(12,4);not null;default:0" json:"idle_fee_per_min"`
	Currency      string                       `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	IsActive      bool                         `gorm:"not null;default:true" json:"is_active"`
	StartDate     *time.Time                   `gorm:"type:timestamptz;index" json:"start_date,omitempty"`
	EndDate       *time.Time                   `gorm:"type:timestamptz;index" json:"end_date,omitempty"`
	TariffType    *constants.TariffType        `gorm:"type:tariff_type" json:"tariff_type,omitempty"`
	PriceType     *constants.PriceType         `gorm:"type:price_type" json:"price_type,omitempty"`
	Units         *constants.Unit              `gorm:"type:units" json:"units,omitempty"`
	CreatedAt     time.Time                    `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time                    `gorm:"not null" json:"updated_at"`
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
	StartIntentID      *uuid.UUID              `gorm:"type:uuid;uniqueIndex" json:"start_intent_id,omitempty"`
	HALTransactionID   *uuid.UUID              `gorm:"type:uuid;uniqueIndex" json:"hal_transaction_id,omitempty"`
	TransactionID      int64                   `gorm:"type:bigint;not null;index" json:"transaction_id"`
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
	LatestMeterWh      *int64                  `gorm:"type:bigint" json:"latest_meter_wh,omitempty"`
	MeterObservedAt    *time.Time              `gorm:"type:timestamptz" json:"meter_observed_at,omitempty"`
	MeterSequence      int64                   `gorm:"type:bigint;not null;default:0" json:"meter_sequence"`
	TotalKWh           decimal.Decimal         `gorm:"type:numeric(14,3);not null;default:0" json:"total_kwh"`
	TotalAmount        decimal.Decimal         `gorm:"type:numeric(14,2);not null;default:0" json:"total_amount"`
	Currency           string                  `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	StopReason         *string                 `gorm:"type:varchar(50)" json:"stop_reason,omitempty"`
	TariffSnapshot     JSONB                   `gorm:"type:jsonb;not null;default:'{}'" json:"tariff_snapshot"`
	TaxSnapshot        JSONB                   `gorm:"type:jsonb;not null;default:'{}'" json:"tax_snapshot"`
	Status             constants.SessionStatus `gorm:"type:varchar(30);not null;default:'ACTIVE'" json:"status"`
	SettlementStatus   string                  `gorm:"type:varchar(32);not null;default:'PENDING'" json:"settlement_status"`
	WalletTransactions []WalletTransaction     `gorm:"foreignKey:SessionID" json:"wallet_transactions,omitempty"`
	Payment            *Payment                `gorm:"foreignKey:SessionID" json:"payment,omitempty"`
	CreatedAt          time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time               `gorm:"not null" json:"updated_at"`
}

// ChargingStartIntent is the durable CMS decision and commercial reservation
// made before a command crosses the service boundary. It is not OCPP start
// truth and cannot materialize a session by itself.
type ChargingStartIntent struct {
	ID                    uuid.UUID                   `gorm:"type:uuid;primaryKey" json:"id"`
	CPOID                 uuid.UUID                   `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID            uuid.UUID                   `gorm:"type:uuid;not null;index" json:"customer_id"`
	ChargerID             uuid.UUID                   `gorm:"type:uuid;not null;index" json:"charger_id"`
	ConnectorID           uuid.UUID                   `gorm:"type:uuid;not null;index" json:"connector_id"`
	WalletID              uuid.UUID                   `gorm:"type:uuid;not null;index" json:"wallet_id"`
	TariffID              uuid.UUID                   `gorm:"type:uuid;not null;index" json:"tariff_id"`
	Status                constants.StartIntentStatus `gorm:"type:varchar(32);not null;index" json:"status"`
	CredentialHash        string                      `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	CredentialExpiresAt   time.Time                   `gorm:"type:timestamptz;not null" json:"credential_expires_at"`
	CommandExpiresAt      time.Time                   `gorm:"type:timestamptz;not null" json:"command_expires_at"`
	EnergyLimitWh         int64                       `gorm:"type:bigint;not null" json:"energy_limit_wh"`
	MaxDurationSeconds    int64                       `gorm:"type:bigint;not null" json:"max_duration_seconds"`
	TariffSnapshot        JSONB                       `gorm:"type:jsonb;not null" json:"tariff_snapshot"`
	TaxSnapshot           JSONB                       `gorm:"type:jsonb;not null" json:"tax_snapshot"`
	MaterializedSessionID *uuid.UUID                  `gorm:"type:uuid;uniqueIndex" json:"materialized_session_id,omitempty"`
	HALCommandID          *uuid.UUID                  `gorm:"type:uuid" json:"hal_command_id,omitempty"`
	CreatedAt             time.Time                   `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time                   `gorm:"not null" json:"updated_at"`
}

type WalletHold struct {
	ID            uuid.UUID                  `gorm:"type:uuid;primaryKey" json:"id"`
	CPOID         uuid.UUID                  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	WalletID      uuid.UUID                  `gorm:"type:uuid;not null;index" json:"wallet_id"`
	StartIntentID uuid.UUID                  `gorm:"type:uuid;not null;uniqueIndex" json:"start_intent_id"`
	Amount        decimal.Decimal            `gorm:"type:numeric(14,2);not null" json:"amount"`
	Currency      string                     `gorm:"type:char(3);not null" json:"currency"`
	Status        constants.WalletHoldStatus `gorm:"type:varchar(32);not null;index" json:"status"`
	CapturedAt    *time.Time                 `gorm:"type:timestamptz" json:"captured_at,omitempty"`
	ReleasedAt    *time.Time                 `gorm:"type:timestamptz" json:"released_at,omitempty"`
	CreatedAt     time.Time                  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time                  `gorm:"not null" json:"updated_at"`
}

type HALCommandRecord struct {
	CMSCommandID      uuid.UUID  `gorm:"type:uuid;primaryKey" json:"cms_command_id"`
	CPOID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Kind              string     `gorm:"type:varchar(8);not null" json:"kind"`
	StartIntentID     *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"start_intent_id,omitempty"`
	ChargingSessionID *uuid.UUID `gorm:"type:uuid;index" json:"charging_session_id,omitempty"`
	HALCommandID      *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"hal_command_id,omitempty"`
	State             string     `gorm:"type:varchar(32);not null;index" json:"state"`
	CommandExpiresAt  time.Time  `gorm:"type:timestamptz;not null" json:"command_expires_at"`
	LastErrorCategory string     `gorm:"type:varchar(64);not null;default:''" json:"last_error_category"`
	LastErrorDetail   string     `gorm:"type:varchar(500);not null;default:''" json:"last_error_detail"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

type HALFactReceipt struct {
	FactID      uuid.UUID `gorm:"type:uuid;primaryKey" json:"fact_id"`
	FactType    string    `gorm:"type:varchar(64);not null" json:"fact_type"`
	Digest      string    `gorm:"type:char(64);not null" json:"digest"`
	OccurredAt  time.Time `gorm:"type:timestamptz;not null" json:"occurred_at"`
	Payload     JSONB     `gorm:"type:jsonb;not null" json:"payload"`
	ProcessedAt time.Time `gorm:"type:timestamptz;not null" json:"processed_at"`
}

type HALChargerMapping struct {
	CMSChargerID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"cms_charger_id"`
	CPOID               uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	ChargerOCPPIdentity string     `gorm:"type:varchar(255);not null;uniqueIndex" json:"charger_ocpp_identity"`
	SyncState           string     `gorm:"type:varchar(32);not null;index" json:"sync_state"`
	LastSyncError       string     `gorm:"type:varchar(500);not null;default:''" json:"last_sync_error"`
	LastSynchronizedAt  *time.Time `gorm:"type:timestamptz" json:"last_synchronized_at,omitempty"`
	CreatedAt           time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"not null" json:"updated_at"`
}

type HALChargerRuntime struct {
	CMSChargerID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"cms_charger_id"`
	CPOID                uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	ConnectionState      string    `gorm:"type:varchar(16);not null" json:"connection_state"`
	ConnectionGeneration int64     `gorm:"type:bigint;not null" json:"connection_generation"`
	ConnectionSequence   int64     `gorm:"type:bigint;not null" json:"connection_sequence"`
	ObservedAt           time.Time `gorm:"type:timestamptz;not null" json:"observed_at"`
	UpdatedAt            time.Time `gorm:"not null" json:"updated_at"`
}

type HALConnectorRuntime struct {
	CMSConnectorID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"cms_connector_id"`
	CMSChargerID            uuid.UUID `gorm:"type:uuid;not null;index" json:"cms_charger_id"`
	CPOID                   uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	OCPPConnectorStatus     string    `gorm:"type:varchar(32);not null" json:"ocpp_connector_status"`
	ConnectorStatusSequence int64     `gorm:"type:bigint;not null" json:"connector_status_sequence"`
	ObservedAt              time.Time `gorm:"type:timestamptz;not null" json:"observed_at"`
	UpdatedAt               time.Time `gorm:"not null" json:"updated_at"`
}

type WalletTransaction struct {
	ID              uuid.UUID                       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID           uuid.UUID                       `gorm:"type:uuid;not null;index" json:"cpo_id"`
	WalletID        uuid.UUID                       `gorm:"type:uuid;not null;index" json:"wallet_id"`
	Wallet          Wallet                          `gorm:"foreignKey:WalletID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"wallet,omitempty"`
	SessionID       *uuid.UUID                      `gorm:"type:uuid;index" json:"session_id,omitempty"`
	Session         *ChargingSession                `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"session,omitempty"`
	RechargeOrderID *uuid.UUID                      `gorm:"type:uuid;index" json:"recharge_order_id,omitempty"`
	RechargeOrder   *WalletRechargeOrder            `gorm:"foreignKey:RechargeOrderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"recharge_order,omitempty"`
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
