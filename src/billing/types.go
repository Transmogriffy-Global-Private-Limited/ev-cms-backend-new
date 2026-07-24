package billing

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

type BillingAccountRequest struct {
	LegalName      string       `json:"legal_name"`
	BillingEmail   string       `json:"billing_email"`
	TaxID          *string      `json:"tax_id"`
	Currency       string       `json:"currency"`
	BillingAddress models.JSONB `json:"billing_address"`
}

type InvoiceLineInput struct {
	Description     string       `json:"description"`
	Quantity        int64        `json:"quantity"`
	UnitAmountMinor int64        `json:"unit_amount_minor"`
	TaxMinor        int64        `json:"tax_minor"`
	Metadata        models.JSONB `json:"metadata"`
}

type CreateInvoiceRequest struct {
	InvoiceNumber     string             `json:"invoice_number"`
	SubscriptionID    *uuid.UUID         `json:"subscription_id"`
	PeriodStartsAt    *time.Time         `json:"period_starts_at"`
	PeriodEndsAt      *time.Time         `json:"period_ends_at"`
	ExternalReference *string            `json:"external_reference"`
	IdempotencyKey    string             `json:"idempotency_key"`
	Lines             []InvoiceLineInput `json:"lines"`
}

type IssueInvoiceRequest struct {
	DueAt  time.Time `json:"due_at"`
	Reason string    `json:"reason"`
}

type VoidInvoiceRequest struct {
	Reason string `json:"reason"`
}

type VoidPaymentRequest struct {
	Reason string `json:"reason"`
}

type AllocationInput struct {
	InvoiceID   uuid.UUID `json:"invoice_id"`
	AmountMinor int64     `json:"amount_minor"`
}

type RecordPaymentRequest struct {
	PaymentReference  string            `json:"payment_reference"`
	Currency          string            `json:"currency"`
	AmountMinor       int64             `json:"amount_minor"`
	Method            string            `json:"method"`
	ExternalReference *string           `json:"external_reference"`
	OccurredAt        time.Time         `json:"occurred_at"`
	Notes             string            `json:"notes"`
	IdempotencyKey    string            `json:"idempotency_key"`
	Allocations       []AllocationInput `json:"allocations"`
}

type InvoiceView struct {
	Invoice models.PlatformInvoice       `json:"invoice"`
	Lines   []models.PlatformInvoiceLine `json:"lines"`
}

type PaymentView struct {
	Payment     models.PlatformPayment             `json:"payment"`
	Allocations []models.PlatformPaymentAllocation `json:"allocations"`
}

type TimelineResponse struct {
	Account  *models.CPOBillingAccount `json:"account,omitempty"`
	Invoices []models.PlatformInvoice  `json:"invoices"`
	Payments []models.PlatformPayment  `json:"payments"`
}
