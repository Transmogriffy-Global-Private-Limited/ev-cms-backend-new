package models

import (
	"time"

	"github.com/google/uuid"
)

// Vehicle represents a customer's electric vehicle registered in the CMS.
type Vehicle struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"customer_id"`
	VehicleNumber     string     `gorm:"type:varchar(50);not null" json:"vehicle_number"`
	Type              *string    `gorm:"type:varchar(50)" json:"type,omitempty"`
	Make              *string    `gorm:"type:varchar(100)" json:"make,omitempty"`
	Model             *string    `gorm:"type:varchar(100)" json:"model,omitempty"`
	BatteryCapacityWh *int64     `gorm:"type:bigint" json:"battery_capacity_wh,omitempty"`
	ConnectorTypes    *[]string  `gorm:"type:jsonb;serializer:json" json:"connector_types,omitempty"`
	SupportsAC        *bool      `gorm:"type:boolean" json:"supports_ac,omitempty"`
	SupportsDC        *bool      `gorm:"type:boolean" json:"supports_dc,omitempty"`
	MaxACPowerW       *int64     `gorm:"type:bigint" json:"max_ac_power_w,omitempty"`
	MaxDCPowerW       *int64     `gorm:"type:bigint" json:"max_dc_power_w,omitempty"`
	Version           int64      `gorm:"type:bigint;not null;default:1" json:"version"`
	LastCharged       *time.Time `gorm:"type:timestamptz" json:"last_charged,omitempty"`
	DateAdded         time.Time  `gorm:"type:timestamptz;not null;default:now()" json:"date_added"`

	// Relations
	CPO      CPO      `gorm:"foreignKey:CPOID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"cpo,omitempty"`
	Customer Customer `gorm:"foreignKey:CPOID,CustomerID;references:CPOID,ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"customer,omitempty"`

	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

// TableName sets the table name explicitly (plural "vehicles").
func (Vehicle) TableName() string {
	return "vehicles"
}
