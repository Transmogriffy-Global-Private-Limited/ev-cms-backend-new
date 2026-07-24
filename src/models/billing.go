package models

import (
	"time"

	"github.com/google/uuid"
)

type CPOBillingAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"cpo_id"`
	LegalName      string    `gorm:"type:varchar(255);not null" json:"legal_name"`
	BillingEmail   string    `gorm:"type:varchar(320);not null" json:"billing_email"`
	TaxID          *string   `gorm:"type:varchar(50)" json:"tax_id,omitempty"`
	Currency       string    `gorm:"type:char(3);not null" json:"currency"`
	BillingAddress JSONB     `gorm:"type:jsonb;not null;default:'{}'" json:"billing_address"`
	CreatedBy      uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt      time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null" json:"updated_at"`
}

type PlatformInvoice struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvoiceNumber     string     `gorm:"type:varchar(80);not null;uniqueIndex" json:"invoice_number"`
	CPOID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	BillingAccountID  uuid.UUID  `gorm:"type:uuid;not null" json:"billing_account_id"`
	SubscriptionID    *uuid.UUID `gorm:"type:uuid" json:"subscription_id,omitempty"`
	Currency          string     `gorm:"type:char(3);not null" json:"currency"`
	Status            string     `gorm:"type:varchar(30);not null;default:'DRAFT'" json:"status"`
	SubtotalMinor     int64      `gorm:"not null" json:"subtotal_minor"`
	TaxMinor          int64      `gorm:"not null" json:"tax_minor"`
	TotalMinor        int64      `gorm:"not null" json:"total_minor"`
	PaidMinor         int64      `gorm:"not null" json:"paid_minor"`
	DueMinor          int64      `gorm:"not null" json:"due_minor"`
	PeriodStartsAt    *time.Time `json:"period_starts_at,omitempty"`
	PeriodEndsAt      *time.Time `json:"period_ends_at,omitempty"`
	IssuedAt          *time.Time `json:"issued_at,omitempty"`
	DueAt             *time.Time `json:"due_at,omitempty"`
	VoidedAt          *time.Time `json:"voided_at,omitempty"`
	VoidReason        *string    `gorm:"type:varchar(500)" json:"void_reason,omitempty"`
	ExternalReference *string    `gorm:"type:varchar(255)" json:"external_reference,omitempty"`
	IdempotencyKey    string     `gorm:"type:varchar(120);not null" json:"idempotency_key"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

type PlatformInvoiceLine struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvoiceID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_invoice_line,priority:1" json:"invoice_id"`
	LineNumber      int       `gorm:"not null;uniqueIndex:uq_invoice_line,priority:2" json:"line_number"`
	Description     string    `gorm:"type:varchar(500);not null" json:"description"`
	Quantity        int64     `gorm:"not null" json:"quantity"`
	UnitAmountMinor int64     `gorm:"not null" json:"unit_amount_minor"`
	SubtotalMinor   int64     `gorm:"not null" json:"subtotal_minor"`
	TaxMinor        int64     `gorm:"not null" json:"tax_minor"`
	TotalMinor      int64     `gorm:"not null" json:"total_minor"`
	Metadata        JSONB     `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
}

type PlatformPayment struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PaymentReference  string     `gorm:"type:varchar(120);not null;uniqueIndex" json:"payment_reference"`
	CPOID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"cpo_id"`
	Currency          string     `gorm:"type:char(3);not null" json:"currency"`
	AmountMinor       int64      `gorm:"not null" json:"amount_minor"`
	AllocatedMinor    int64      `gorm:"not null" json:"allocated_minor"`
	Status            string     `gorm:"type:varchar(20);not null;default:'RECORDED'" json:"status"`
	VoidedAt          *time.Time `json:"voided_at,omitempty"`
	VoidReason        *string    `gorm:"type:varchar(500)" json:"void_reason,omitempty"`
	Method            string     `gorm:"type:varchar(50);not null" json:"method"`
	ExternalReference *string    `gorm:"type:varchar(255)" json:"external_reference,omitempty"`
	OccurredAt        time.Time  `gorm:"not null" json:"occurred_at"`
	Notes             string     `gorm:"type:varchar(1000);not null;default:''" json:"notes"`
	IdempotencyKey    string     `gorm:"type:varchar(120);not null" json:"idempotency_key"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

type PlatformPaymentAllocation struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PaymentID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_payment_allocation,priority:1" json:"payment_id"`
	InvoiceID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_payment_allocation,priority:2" json:"invoice_id"`
	AmountMinor int64     `gorm:"not null" json:"amount_minor"`
	CreatedBy   uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
}
