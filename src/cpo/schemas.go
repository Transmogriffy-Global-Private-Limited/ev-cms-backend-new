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
	GSTIN        *string                  `json:"gstin,omitempty"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        string                   `json:"state"`
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

type View struct {
	ID             uuid.UUID                `json:"id"`
	Slug           string                   `json:"slug"`
	BusinessName   string                   `json:"business_name"`
	CompanyType    constants.CPOCompanyType `json:"company_type"`
	GSTIN          *string                  `json:"gstin,omitempty"`
	Address        string                   `json:"address"`
	City           string                   `json:"city"`
	State          string                   `json:"state"`
	Pincode        string                   `json:"pincode"`
	Status         constants.CPOStatus      `json:"status"`
	AppID          string                   `json:"app_id"`
	AppIDMode      constants.CPOAppIDMode   `json:"app_id_mode"`
	AppIDUpdatedAt time.Time                `json:"app_id_updated_at"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

type InitialAdminView struct {
	UserID          uuid.UUID         `json:"user_id"`
	Email           string            `json:"email"`
	FullName        string            `json:"full_name"`
	Role            constants.CPORole `json:"role"`
	IdentityCreated bool              `json:"identity_created"`
}

type CreateResponse struct {
	CPO   View             `json:"cpo"`
	Admin InitialAdminView `json:"admin"`
}
type UpdateProfileRequest struct {
	BusinessName *string                   `json:"business_name,omitempty"`
	CompanyType  *constants.CPOCompanyType `json:"company_type,omitempty"`
	GSTIN        *string                   `json:"gstin,omitempty"`
	Address      *string                   `json:"address,omitempty"`
	City         *string                   `json:"city,omitempty"`
	State        *string                   `json:"state,omitempty"`
	Pincode      *string                   `json:"pincode,omitempty"`
}
type CreateProfileRequest struct {
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        string                   `json:"gstin"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        string                   `json:"state"`
	Pincode      string                   `json:"pincode"`
}

type CreateChargerRequest struct {
	HubID        uuid.UUID `json:"hub_id"`
	Vendor       string    `json:"vendor"`
	Model        string    `json:"model"`
	SerialNumber string    `json:"serial_number"`
	MaxPowerKW   float64   `json:"max_power_kw"`
}

type UpdateChargerRequest struct {
	HubID        *uuid.UUID `json:"hub_id,omitempty"`
	Vendor       *string    `json:"vendor,omitempty"`
	Model        *string    `json:"model,omitempty"`
	SerialNumber *string    `json:"serial_number,omitempty"`
	MaxPowerKW   *float64   `json:"max_power_kw,omitempty"`
}

type DeleteChargerRequest struct {
	ChargerID string `json:"charger_id"`
}

type ChargerView struct {
	ID           uuid.UUID               `json:"id"`
	CPOID        uuid.UUID               `json:"cpo_id"`
	HubID        uuid.UUID               `json:"hub_id"`
	ChargerID    string                  `json:"charger_id"`
	OCPPIdentity string                  `json:"ocpp_identity"`
	Vendor       string                  `json:"vendor"`
	Model        string                  `json:"model"`
	SerialNumber string                  `json:"serial_number"`
	MaxPowerKW   float64                 `json:"max_power_kw"`
	Status       constants.ChargerStatus `json:"status"`
	OCPPVersion  string                  `json:"ocpp_version"`
	LastSeenAt   *time.Time              `json:"last_seen_at,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

type CreateHubRequest struct {
	Name        string  `json:"name"`
	Address     string  `json:"address"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Open24Hours *bool   `json:"open_24_hours,omitempty"`
}

type UpdateHubRequest struct {
	Name        *string  `json:"name,omitempty"`
	Address     *string  `json:"address,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Open24Hours *bool    `json:"open_24_hours,omitempty"`
}

type HubView struct {
	ID          uuid.UUID `json:"id"`
	CPOID       uuid.UUID `json:"cpo_id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Open24Hours bool      `json:"open_24_hours"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateTariffRequest struct {
	HubID         uuid.UUID       `json:"hub_id"`
	ChargerID     *uuid.UUID      `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID      `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID      `json:"user_group_id,omitempty"`
	PricePerKWh   decimal.Decimal `json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal `json:"idle_fee_per_min"`
	Currency      string          `json:"currency"`
	IsActive      *bool           `json:"is_active,omitempty"`
}

type UpdateTariffRequest struct {
	HubID         *uuid.UUID       `json:"hub_id,omitempty"`
	ChargerID     *uuid.UUID       `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID       `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID       `json:"user_group_id,omitempty"`
	PricePerKWh   *decimal.Decimal `json:"price_per_kwh,omitempty"`
	IdleFeePerMin *decimal.Decimal `json:"idle_fee_per_min,omitempty"`
	Currency      *string          `json:"currency,omitempty"`
	IsActive      *bool            `json:"is_active,omitempty"`
}

type TariffView struct {
	ID            uuid.UUID       `json:"id"`
	CPOID         uuid.UUID       `json:"cpo_id"`
	HubID         uuid.UUID       `json:"hub_id"`
	ChargerID     *uuid.UUID      `json:"charger_id,omitempty"`
	GSTID         *uuid.UUID      `json:"gst_id,omitempty"`
	UserGroupID   *uuid.UUID      `json:"user_group_id,omitempty"`
	PricePerKWh   decimal.Decimal `json:"price_per_kwh"`
	IdleFeePerMin decimal.Decimal `json:"idle_fee_per_min"`
	Currency      string          `json:"currency"`
	IsActive      bool            `json:"is_active"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type CreateGSTRequest struct {
	Name     string          `json:"name"`
	SGSTRate decimal.Decimal `json:"sgst_rate"`
	CGSTRate decimal.Decimal `json:"cgst_rate"`
	IGSTRate decimal.Decimal `json:"igst_rate"`
	IsActive *bool           `json:"is_active,omitempty"`
}

type UpdateGSTRequest struct {
	Name     *string          `json:"name,omitempty"`
	SGSTRate *decimal.Decimal `json:"sgst_rate,omitempty"`
	CGSTRate *decimal.Decimal `json:"cgst_rate,omitempty"`
	IGSTRate *decimal.Decimal `json:"igst_rate,omitempty"`
	IsActive *bool            `json:"is_active,omitempty"`
}

type GSTView struct {
	ID        uuid.UUID       `json:"id"`
	CPOID     uuid.UUID       `json:"cpo_id"`
	Name      string          `json:"name"`
	SGSTRate  decimal.Decimal `json:"sgst_rate"`
	CGSTRate  decimal.Decimal `json:"cgst_rate"`
	IGSTRate  decimal.Decimal `json:"igst_rate"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
