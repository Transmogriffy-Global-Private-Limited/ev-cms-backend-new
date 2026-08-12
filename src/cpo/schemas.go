package cpo

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CreateRequest struct {
	Slug         string                   `json:"slug"`
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        string                   `json:"gstin"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        constants.IndianState    `json:"state"`
	Pincode      string                   `json:"pincode"`
	Admin        InitialAdminRequest      `json:"admin"`
}

type InitialAdminRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

type SetAppIDRequest struct {
	AppID string `json:"app_id"`
}

type UpdateProfileRequest struct {
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        string                   `json:"gstin"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        constants.IndianState    `json:"state"`
	Pincode      string                   `json:"pincode"`
}

type LifecycleRequest struct {
	Reason string `json:"reason"`
}

type PrimaryAdminRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Reason   string `json:"reason"`
}

type ReasonRequest struct {
	Reason string `json:"reason"`
}

type ListQuery struct {
	Search   string
	Status   *constants.CPOStatus
	AppMode  *constants.CPOAppIDMode
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

type SlugAvailabilityResponse struct {
	Slug      string `json:"slug"`
	Available bool   `json:"available"`
}

type View struct {
	ID                    uuid.UUID                `json:"id"`
	Slug                  string                   `json:"slug"`
	BusinessName          string                   `json:"business_name"`
	CompanyType           constants.CPOCompanyType `json:"company_type"`
	GSTIN                 string                   `json:"gstin"`
	Address               string                   `json:"address"`
	City                  string                   `json:"city"`
	State        constants.IndianState    `json:"state"`
	Pincode               string                   `json:"pincode"`
	Status                constants.CPOStatus      `json:"status"`
	StatusReason          string                   `json:"status_reason"`
	StatusChangedAt       time.Time                `json:"status_changed_at"`
	StatusChangedByUserID *uuid.UUID               `json:"status_changed_by_user_id,omitempty"`
	AppID                 string                   `json:"app_id"`
	AppIDMode             constants.CPOAppIDMode   `json:"app_id_mode"`
	AppIDUpdatedAt        time.Time                `json:"app_id_updated_at"`
	CreatedAt             time.Time                `json:"created_at"`
	UpdatedAt             time.Time                `json:"updated_at"`
}

type InitialAdminView struct {
	UserID          uuid.UUID         `json:"user_id"`
	Email           string            `json:"email"`
	FullName        string            `json:"full_name"`
	Role            constants.CPORole `json:"role"`
	IdentityCreated bool              `json:"identity_created"`
}

type PrimaryAdminView struct {
	UserID                   uuid.UUID                  `json:"user_id"`
	Email                    string                     `json:"email"`
	FullName                 string                     `json:"full_name"`
	Role                     constants.CPORole          `json:"role"`
	MembershipStatus         constants.MembershipStatus `json:"membership_status"`
	IdentityActive           bool                       `json:"identity_active"`
	IdentityVerified         bool                       `json:"identity_verified"`
	MustChangePassword       bool                       `json:"must_change_password"`
	LastLoginAt              *time.Time                 `json:"last_login_at,omitempty"`
	LatestOnboardingDelivery *OnboardingDeliveryView    `json:"latest_onboarding_delivery,omitempty"`
}

type OnboardingDeliveryView struct {
	JobID     uuid.UUID                  `json:"job_id"`
	Template  string                     `json:"template"`
	Status    constants.MailOutboxStatus `json:"status"`
	Attempts  int                        `json:"attempts"`
	SentAt    *time.Time                 `json:"sent_at,omitempty"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type CreateResponse struct {
	CPO   View             `json:"cpo"`
	Admin InitialAdminView `json:"admin"`
}

type ListResponse struct {
	CPOs         []View     `json:"cpos"`
	NextBefore   *time.Time `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID `json:"next_before_id,omitempty"`
	HasMore      bool       `json:"has_more"`
}

type SessionRevocationResponse struct {
	RevokedSessions      int64 `json:"revoked_sessions"`
	RevokedRefreshTokens int64 `json:"revoked_refresh_tokens"`
}

type UpdateAdminProfileRequest struct {
	FullName *string `json:"full_name,omitempty"`
	Phone    *string `json:"phone,omitempty"`
}

type AdminProfileView struct {
	UserID     uuid.UUID         `json:"user_id"`
	CPOID      uuid.UUID         `json:"cpo_id"`
	Email      string            `json:"email"`
	FullName   string            `json:"full_name"`
	Phone      *string           `json:"phone,omitempty"`
	Role       constants.CPORole `json:"role"`
	IsVerified bool              `json:"is_verified"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type CPOUserView struct {
	ID               uuid.UUID                   `json:"id"`
	CPOID            uuid.UUID                   `json:"cpo_id"`
	Email            string                      `json:"email"`
	FullName         string                      `json:"full_name"`
	Phone            *string                     `json:"phone,omitempty"`
	IsActive         bool                        `json:"is_active"`
	IsVerified       bool                        `json:"is_verified"`
	Role             *constants.CPORole          `json:"role,omitempty"`
	MembershipStatus *constants.MembershipStatus `json:"membership_status,omitempty"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
}

// OrganizationView is the tenant-safe, read-only projection of the CPO record.
// It intentionally omits platform actor IDs and the privileged lifecycle reason.
type OrganizationView struct {
	ID              uuid.UUID                `json:"id"`
	Slug            string                   `json:"slug"`
	BusinessName    string                   `json:"business_name"`
	CompanyType     constants.CPOCompanyType `json:"company_type"`
	GSTIN           string                   `json:"gstin"`
	Address         string                   `json:"address"`
	City            string                   `json:"city"`
	State        constants.IndianState    `json:"state"`
	Pincode         string                   `json:"pincode"`
	Status          constants.CPOStatus      `json:"status"`
	StatusChangedAt time.Time                `json:"status_changed_at"`
	AppID           string                   `json:"app_id"`
	AppIDMode       constants.CPOAppIDMode   `json:"app_id_mode"`
	AppIDUpdatedAt  time.Time                `json:"app_id_updated_at"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
}

type CreateChargerRequest struct {
	HubID               *uuid.UUID               `json:"hub_id,omitempty"`
	Vendor              string                   `json:"vendor,omitempty"`
	Model               string                   `json:"model,omitempty"`
	SerialNumber        string                   `json:"serial_number"`
	MaxPowerKW          float64                  `json:"max_power_kw"`
	ChargerName         string                   `json:"charger_name"`
	ChargerHostName     string                   `json:"charger_host_name"`
	ChargerHostPhoneNo  string                   `json:"charger_host_phone_no"`
	ChargerType         string                   `json:"charger_type"`
	Segment             string                   `json:"segment"`
	SubSegment          string                   `json:"sub_segment"`
	ChargerUseType      string                   `json:"charger_use_type"`
	NumberOfConnectors  int                      `json:"number_of_connectors"`
	Parking             string                   `json:"parking"`
	Protocol            string                   `json:"protocol"`
	TwentyFourSevenOpen bool                     `json:"twenty_four_seven_open_status"`
	Connectors          []CreateConnectorRequest `json:"connectors"`
}

type CreateConnectorRequest struct {
	ConnectorNumber        int     `json:"connector_number"`
	ConnectorType          string  `json:"connector_type"`
	ConnectorTotalCapacity float64 `json:"connector_total_capacity"`
}

type UpdateChargerRequest struct {
	HubID               *uuid.UUID                `json:"hub_id,omitempty"`
	Vendor              *string                   `json:"vendor,omitempty"`
	Model               *string                   `json:"model,omitempty"`
	SerialNumber        *string                   `json:"serial_number,omitempty"`
	MaxPowerKW          *float64                  `json:"max_power_kw,omitempty"`
	ChargerName         *string                   `json:"charger_name,omitempty"`
	ChargerHostName     *string                   `json:"charger_host_name,omitempty"`
	ChargerHostPhoneNo  *string                   `json:"charger_host_phone_no,omitempty"`
	ChargerType         *string                   `json:"charger_type,omitempty"`
	Segment             *string                   `json:"segment,omitempty"`
	SubSegment          *string                   `json:"sub_segment,omitempty"`
	ChargerUseType      *string                   `json:"charger_use_type,omitempty"`
	NumberOfConnectors  *int                      `json:"number_of_connectors,omitempty"`
	Parking             *string                   `json:"parking,omitempty"`
	Protocol            *string                   `json:"protocol,omitempty"`
	TwentyFourSevenOpen *bool                     `json:"twenty_four_seven_open_status,omitempty"`
	Connectors          *[]UpdateConnectorRequest `json:"connectors,omitempty"`
}

type UpdateConnectorRequest struct {
	ID                     uuid.UUID `json:"id"`
	ConnectorNumber        *int      `json:"connector_number,omitempty"`
	ConnectorType          *string   `json:"connector_type,omitempty"`
	ConnectorTotalCapacity *float64  `json:"connector_total_capacity,omitempty"`
}

type ChargerView struct {
	ID                      uuid.UUID               `json:"id"`
	CPOID                   uuid.UUID               `json:"cpo_id"`
	HubID                   *uuid.UUID              `json:"hub_id,omitempty"`
	HubName                 *string                 `json:"hub_name,omitempty"`
	ChargerID               string                  `json:"charger_id"`
	OCPPIdentity            string                  `json:"ocpp_identity"`
	Vendor                  *string                 `json:"vendor,omitempty"`
	Model                   *string                 `json:"model,omitempty"`
	SerialNumber            string                  `json:"serial_number"`
	MaxPowerKW              float64                 `json:"max_power_kw"`
	Status                  constants.ChargerStatus `json:"status"`
	OCPPVersion             string                  `json:"ocpp_version"`
	LastSeenAt              *time.Time              `json:"last_seen_at,omitempty"`
	ChargerName             string                  `json:"charger_name"`
	ChargerHostName         string                  `json:"charger_host_name"`
	ChargerHostPhoneNo      string                  `json:"charger_host_phone_no"`
	ChargerType             string                  `json:"charger_type"`
	Segment                 string                  `json:"segment"`
	SubSegment              string                  `json:"sub_segment"`
	ChargerImage            string                  `json:"charger_image"`
	ChargerUseType          string                  `json:"charger_use_type"`
	NumberOfConnectors      int                     `json:"number_of_connectors"`
	Parking                 string                  `json:"parking"`
	Protocol                string                  `json:"protocol"`
	TwentyFourSevenOpen     bool                    `json:"twenty_four_seven_open_status"`
	Connectors              []ConnectorView         `json:"connectors"`
	ChargerConnectionURLWS  string                  `json:"charger_connection_url_ws"`
	ChargerConnectionURLWSS string                  `json:"charger_connection_url_wss"`
	Assigned                bool                    `json:"assigned"`
	CreatedAt               time.Time               `json:"created_at"`
	UpdatedAt               time.Time               `json:"updated_at"`
}

type ChargerResponse struct {
	ChargerView
	Email string `json:"email,omitempty"`
}

type TenantListQuery struct {
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

type ChargerListResponse struct {
	Chargers     []ChargerResponse `json:"chargers"`
	NextBefore   *time.Time        `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID        `json:"next_before_id,omitempty"`
	HasMore      bool              `json:"has_more"`
}

type ConnectorView struct {
	ID                     uuid.UUID               `json:"id"`
	CPOID                  uuid.UUID               `json:"cpo_id"`
	ChargerID              uuid.UUID               `json:"charger_id"`
	ConnectorNumber        int                     `json:"connector_number"`
	ConnectorType          string                  `json:"connector_type"`
	ConnectorTotalCapacity float64                 `json:"connector_total_capacity"`
	Status                 constants.ChargerStatus `json:"status"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
}

type CreateHubRequest struct {
	Name            string      `json:"name"`
	Address         string      `json:"address"`
	State        constants.IndianState    `json:"state"`
	Latitude        *float64    `json:"latitude"`
	Longitude       *float64    `json:"longitude"`
	Open24Hours     *bool       `json:"open_24_hours,omitempty"`
	SanctionLoad    *float64    `json:"sanction_load,omitempty"`
	CustomerVisible *bool       `json:"customer_visible,omitempty"`
	ChargerIDs      []uuid.UUID `json:"charger_ids,omitempty"`
}

type UpdateHubRequest struct {
	Name            *string  `json:"name,omitempty"`
	Address         *string  `json:"address,omitempty"`
	State           *constants.IndianState  `json:"state,omitempty"`
	Latitude        *float64 `json:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty"`
	Open24Hours     *bool    `json:"open_24_hours,omitempty"`
	SanctionLoad    *float64 `json:"sanction_load,omitempty"`
	CustomerVisible *bool    `json:"customer_visible,omitempty"`
}

type AssignChargerRequest struct {
	ChargerID uuid.UUID `json:"charger_id"`
}

type HubView struct {
	ID              uuid.UUID `json:"id"`
	CPOID           uuid.UUID `json:"cpo_id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	State        constants.IndianState    `json:"state"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	Open24Hours     bool      `json:"open_24_hours"`
	SanctionLoad    float64   `json:"sanction_load"`
	CustomerVisible bool      `json:"customer_visible"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type HubListResponse struct {
	Hubs         []HubView  `json:"hubs"`
	NextBefore   *time.Time `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID `json:"next_before_id,omitempty"`
	HasMore      bool       `json:"has_more"`
}

// HubResponse represents the detailed view of a hub, including a paginated list of its chargers.
type HubResponse struct {
	ID              uuid.UUID            `json:"id"`
	CPOID           uuid.UUID            `json:"cpo_id"`
	Name            string               `json:"name"`
	Address         string               `json:"address"`
	State        constants.IndianState    `json:"state"`
	Latitude        float64              `json:"latitude"`
	Longitude       float64              `json:"longitude"`
	Open24Hours     bool                 `json:"open_24_hours"`
	SanctionLoad    float64              `json:"sanction_load"`
	CustomerVisible bool                 `json:"customer_visible"`
	Chargers        *ChargerListResponse `json:"chargers,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// CPOAdminCustomerView is the CPO administrator's view of a customer account.
type CPOAdminCustomerView struct {
	ID                uuid.UUID                `json:"id"`
	CPOID             uuid.UUID                `json:"cpo_id"`
	Email             string                   `json:"email"`
	FullName          string                   `json:"full_name"`
	Phone             *string                  `json:"phone,omitempty"`
	Status            constants.CustomerStatus `json:"status"`
	IsVerified        bool                     `json:"is_verified"`
	LastLoginAt       *time.Time               `json:"last_login_at,omitempty"`
	UsergroupAssigned bool                     `json:"usergroup_assigned"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

// CPOAdminCustomerListQuery defines the query parameters for listing customers.
type CPOAdminCustomerListQuery struct {
	Search   string
	Status   *constants.CustomerStatus
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

// CPOAdminCustomerListResponse is the paginated list of customer views.
type CPOAdminCustomerListResponse struct {
	Customers    []CPOAdminCustomerView `json:"customers"`
	NextBefore   *time.Time             `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID             `json:"next_before_id,omitempty"`
	HasMore      bool                   `json:"has_more"`
}

type CreateTariffRequest struct {
	HubID         uuid.UUID             `json:"hub_id"`
	ChargerID     *uuid.UUID            `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID            `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID            `json:"user_group_id,omitempty"`
	PricePerKWh   decimal.Decimal       `json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal       `json:"idle_fee_per_min"`
	Currency      string                `json:"currency"`
	IsActive      *bool                 `json:"is_active,omitempty"`
	StartDate     *time.Time            `json:"start_date,omitempty"`
	EndDate       *time.Time            `json:"end_date,omitempty"`
	TariffType    *constants.TariffType `json:"tariff_type,omitempty"`
	PriceType     *constants.PriceType  `json:"price_type,omitempty"`
	Units         *constants.Unit       `json:"units,omitempty"`
}

type UpdateTariffRequest struct {
	HubID         *uuid.UUID            `json:"hub_id,omitempty"`
	ChargerID     *uuid.UUID            `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID            `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID            `json:"user_group_id,omitempty"`
	PricePerKWh   *decimal.Decimal      `json:"price_per_kwh,omitempty"`
	IdleFeePerMin *decimal.Decimal      `json:"idle_fee_per_min,omitempty"`
	Currency      *string               `json:"currency,omitempty"`
	IsActive      *bool                 `json:"is_active,omitempty"`
	StartDate     *time.Time            `json:"start_date,omitempty"`
	EndDate       *time.Time            `json:"end_date,omitempty"`
	TariffType    *constants.TariffType `json:"tariff_type,omitempty"`
	PriceType     *constants.PriceType  `json:"price_type,omitempty"`
	Units         *constants.Unit       `json:"units,omitempty"`
}

type TariffView struct {
	ID            uuid.UUID             `json:"id"`
	CPOID         uuid.UUID             `json:"cpo_id"`
	HubID         uuid.UUID             `json:"hub_id"`
	ChargerID     *uuid.UUID            `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID            `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID            `json:"user_group_id,omitempty"`
	PricePerKWh   decimal.Decimal       `json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal       `json:"idle_fee_per_min"`
	Currency      string                `json:"currency"`
	IsActive      bool                  `json:"is_active"`
	StartDate     *time.Time            `json:"start_date,omitempty"`
	EndDate       *time.Time            `json:"end_date,omitempty"`
	TariffType    *constants.TariffType `json:"tariff_type,omitempty"`
	PriceType     *constants.PriceType  `json:"price_type,omitempty"`
	Units         *constants.Unit       `json:"units,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

type TariffListResponse struct {
	Tariffs      []TariffView `json:"tariffs"`
	NextBefore   *time.Time   `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID   `json:"next_before_id,omitempty"`
	HasMore      bool         `json:"has_more"`
}

type CreateGSTRequest struct {
	Name     string           `json:"name"`
	State        constants.IndianState    `json:"state"`
	SGSTRate *decimal.Decimal `json:"sgst_rate"`
	CGSTRate *decimal.Decimal `json:"cgst_rate"`
	IGSTRate *decimal.Decimal `json:"igst_rate"`
	IsActive *bool            `json:"is_active,omitempty"`
}

type UpdateGSTRequest struct {
	Name     *string                 `json:"name,omitempty"`
	State    *constants.IndianState  `json:"state,omitempty"` // ✅ correct type
	SGSTRate *decimal.Decimal        `json:"sgst_rate,omitempty"`
	CGSTRate *decimal.Decimal        `json:"cgst_rate,omitempty"`
	IGSTRate *decimal.Decimal        `json:"igst_rate,omitempty"`
	IsActive *bool                   `json:"is_active,omitempty"`
}

type GSTView struct {
	ID        uuid.UUID        `json:"id"`
	CPOID     uuid.UUID        `json:"cpo_id"`
	Name      string           `json:"name"`
	State        constants.IndianState    `json:"state"`
	SGSTRate  *decimal.Decimal `json:"sgst_rate,omitempty"`
	CGSTRate  *decimal.Decimal `json:"cgst_rate,omitempty"`
	IGSTRate  *decimal.Decimal `json:"igst_rate,omitempty"`
	IsActive  bool             `json:"is_active"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type GSTListResponse struct {
	GSTs         []GSTView  `json:"gsts"`
	NextBefore   *time.Time `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID `json:"next_before_id,omitempty"`
	HasMore      bool       `json:"has_more"`
}

type CreateUserGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

type UpdateUserGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

type AddMemberToUserGroupRequest struct {
	CustomerID uuid.UUID `json:"customer_id"`
}

type UserGroupView struct {
	ID          uuid.UUID              `json:"id"`
	CPOID       uuid.UUID              `json:"cpo_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Members     []CPOAdminCustomerView `json:"members,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type UserGroupListResponse struct {
	UserGroups   []UserGroupView `json:"user_groups"`
	NextBefore   *time.Time      `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID      `json:"next_before_id,omitempty"`
	HasMore      bool            `json:"has_more"`
}

type CPOSubscriptionPlanView struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Currency        string `json:"currency"`
	PriceMinor      int64  `json:"price_minor"`
	BillingInterval string `json:"billing_interval"`
	IntervalCount   int    `json:"interval_count"`
	TrialDays       int    `json:"trial_days"`
}

type CPOSubscriptionView struct {
	ID                    uuid.UUID               `json:"id"`
	Status                string                  `json:"status"`
	StartsAt              time.Time               `json:"starts_at"`
	TrialEndsAt           *time.Time              `json:"trial_ends_at,omitempty"`
	CurrentPeriodStartsAt time.Time               `json:"current_period_starts_at"`
	CurrentPeriodEndsAt   time.Time               `json:"current_period_ends_at"`
	CancelAtPeriodEnd     bool                    `json:"cancel_at_period_end"`
	CancelledAt           *time.Time              `json:"cancelled_at,omitempty"`
	EndedAt               *time.Time              `json:"ended_at,omitempty"`
	Plan                  CPOSubscriptionPlanView `json:"plan"`
}

type UpdateChargerStatusRequest struct {
	Status       constants.ChargerStatus `json:"status"`
	OCPPIdentity string                  `json:"ocpp_identity,omitempty"`
}

type UpdateHubCustomerVisibilityRequest struct {
	CustomerVisible bool `json:"customer_visible"`
}

type ChargerStatusResponse struct {
	ChargerID    uuid.UUID               `json:"charger_id"`
	OCPPIdentity string                  `json:"ocpp_identity"`
	Status       constants.ChargerStatus `json:"status"`
}

type SettingsView struct {
	InvoiceLogo *string `json:"invoice_logo,omitempty"`
	InvoiceNote *string `json:"invoice_note,omitempty"`
}
