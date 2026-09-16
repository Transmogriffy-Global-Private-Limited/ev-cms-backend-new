package models

import (
	"time"

	"github.com/google/uuid"
)

// CustomerRating captures one customer's post-charging feedback.
//
// It is CPO-scoped and points at the customer, charger, hub (charging station),
// and charging session that the feedback is about. Each of those parent
// entities exposes this as a one-to-many relation.
//
// Rating scale is 1..5 (enforced at the database level by CHECK constraints).
// StationRating and ChargerRating are optional because a user may only rate
// the overall experience, or may not have an associated hub/session.
type CustomerRating struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID      uuid.UUID `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID uuid.UUID `gorm:"type:uuid;not null;index" json:"customer_id"`
	Customer   Customer  `gorm:"foreignKey:CustomerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"customer,omitempty"`

	ChargerID uuid.UUID `gorm:"type:uuid;not null;index" json:"charger_id"`
	Charger   Charger   `gorm:"foreignKey:ChargerID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"charger,omitempty"`

	HubID *uuid.UUID `gorm:"type:uuid;index" json:"hub_id,omitempty"`
	Hub   *Hub       `gorm:"foreignKey:HubID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"hub,omitempty"`

	SessionID *uuid.UUID       `gorm:"type:uuid;index" json:"session_id,omitempty"`
	Session   *ChargingSession `gorm:"foreignKey:SessionID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"session,omitempty"`

	// Ratings (1..5). Only OverallRating is mandatory.
	OverallRating int     `gorm:"type:smallint;not null" json:"overall_rating"`
	StationRating *int    `gorm:"type:smallint" json:"station_rating,omitempty"` // hub / station rating
	ChargerRating *int    `gorm:"type:smallint" json:"charger_rating,omitempty"`
	Reason        *string `gorm:"type:varchar(1000)" json:"reason,omitempty"`

	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

func (CustomerRating) TableName() string { return "customer_ratings" }
