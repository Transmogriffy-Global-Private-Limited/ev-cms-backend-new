package models

import (
	"time"

	"github.com/google/uuid"
)

type Settings struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID       uuid.UUID `gorm:"type:uuid;not null;unique" json:"cpo_id"`
	InvoiceLogo *string   `gorm:"type:varchar(255)" json:"invoice_logo"`
	InvoiceNote *string   `gorm:"type:text" json:"invoice_note"`
	CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}
