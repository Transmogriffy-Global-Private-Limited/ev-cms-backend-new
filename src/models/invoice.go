package models

import (
	"time"

	"github.com/google/uuid"
)

// ChargingSessionInvoice is a downstream immutable document record. Charging
// and settlement do not depend on its lifecycle.
type ChargingSessionInvoice struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	CPOID               uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"customer_id"`
	SessionID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"session_id"`
	InvoiceNumber       string     `gorm:"type:varchar(40);not null" json:"invoice_number"`
	FinancialYear       string     `gorm:"type:varchar(7);not null" json:"financial_year"`
	Serial              int64      `gorm:"not null" json:"serial"`
	IssuedAt            time.Time  `gorm:"not null" json:"issued_at"`
	SnapshotVersion     int        `gorm:"not null;default:1" json:"snapshot_version"`
	RendererVersion     string     `gorm:"type:varchar(40);not null" json:"renderer_version"`
	Snapshot            JSONB      `gorm:"type:jsonb;not null" json:"-"`
	GenerationStatus    string     `gorm:"type:varchar(20);not null;index" json:"generation_status"`
	GenerationAttempts  int        `gorm:"not null;default:0" json:"generation_attempts"`
	AvailableAt         time.Time  `gorm:"not null;index" json:"available_at"`
	GenerationLockedAt  *time.Time `gorm:"type:timestamptz" json:"-"`
	LastGenerationError *string    `gorm:"type:varchar(500)" json:"-"`
	StoragePath         *string    `gorm:"type:varchar(500)" json:"-"`
	MIMEType            *string    `gorm:"type:varchar(100)" json:"mime_type,omitempty"`
	FileSize            *int64     `gorm:"type:bigint" json:"file_size,omitempty"`
	SHA256              *string    `gorm:"type:char(64)" json:"sha256,omitempty"`
	ReadyAt             *time.Time `gorm:"type:timestamptz" json:"ready_at,omitempty"`
	CreatedAt           time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"not null" json:"updated_at"`
}

func (ChargingSessionInvoice) TableName() string { return "charging_session_invoices" }

// InvoiceNumberSequence serializes visible invoice numbers per CPO and Indian
// financial year. next_serial is allocated atomically, never derived from MAX.
type InvoiceNumberSequence struct {
	CPOID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	FinancialYear string    `gorm:"type:varchar(7);primaryKey" json:"-"`
	NextSerial    int64     `gorm:"not null" json:"-"`
	UpdatedAt     time.Time `gorm:"not null" json:"-"`
}

func (InvoiceNumberSequence) TableName() string { return "invoice_number_sequences" }

// InvoiceRollout controls automatic first delivery separately from artifact
// eligibility. The single durable row is initialized when the migration runs.
type InvoiceRollout struct {
	ID                 int       `gorm:"primaryKey" json:"-"`
	AutomaticEmailFrom time.Time `gorm:"not null" json:"-"`
	CreatedAt          time.Time `gorm:"not null" json:"-"`
	UpdatedAt          time.Time `gorm:"not null" json:"-"`
}

func (InvoiceRollout) TableName() string { return "invoice_rollout" }

// InvoiceAsset is an immutable, content-addressed issuance-time branding
// input. It is deliberately separate from mutable CPO settings and never
// served directly. Its hash primary key deduplicates identical logo bytes.
type InvoiceAsset struct {
	SHA256    string    `gorm:"type:char(64);primaryKey" json:"-"`
	MIMEType  string    `gorm:"type:varchar(100);not null" json:"-"`
	Content   []byte    `gorm:"type:bytea;not null" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"-"`
}

func (InvoiceAsset) TableName() string { return "invoice_assets" }

// InvoiceAssetReference binds the immutable input used at issuance to one
// invoice without duplicating the asset bytes.
type InvoiceAssetReference struct {
	InvoiceID uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	SHA256    string    `gorm:"type:char(64);not null" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"-"`
}

func (InvoiceAssetReference) TableName() string { return "invoice_asset_references" }

// InvoiceDelivery records the external SMTP boundary independently of the PDF
// artifact. SENDING recovery becomes AMBIGUOUS, never a blind resend.
type InvoiceDelivery struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	InvoiceID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"invoice_id"`
	Recipient   string     `gorm:"type:varchar(320);not null" json:"-"`
	Status      string     `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts int        `gorm:"not null;default:8" json:"max_attempts"`
	AvailableAt time.Time  `gorm:"not null;index" json:"available_at"`
	LockedAt    *time.Time `gorm:"type:timestamptz" json:"-"`
	LastError   *string    `gorm:"type:varchar(500)" json:"-"`
	SentAt      *time.Time `gorm:"type:timestamptz" json:"sent_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"not null" json:"updated_at"`
}

func (InvoiceDelivery) TableName() string { return "invoice_deliveries" }
