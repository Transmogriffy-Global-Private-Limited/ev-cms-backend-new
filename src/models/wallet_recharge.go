package models

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

// WalletRechargeOrder is the CMS-owned payment intent for adding funds to one
// CPO-local customer wallet. Provider credentials never belong in this model.
type WalletRechargeOrder struct {
	ID                   uuid.UUID                           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID                uuid.UUID                           `gorm:"type:uuid;not null;index" json:"cpo_id"`
	CustomerID           uuid.UUID                           `gorm:"type:uuid;not null;index" json:"customer_id"`
	WalletID             uuid.UUID                           `gorm:"type:uuid;not null;index" json:"wallet_id"`
	Provider             string                              `gorm:"type:varchar(30);not null;default:'RAZORPAY'" json:"provider"`
	IdempotencyKey       string                              `gorm:"type:varchar(120);not null" json:"idempotency_key"`
	ProviderOrderID      *string                             `gorm:"type:varchar(100)" json:"provider_order_id,omitempty"`
	AmountMinor          int64                               `gorm:"not null" json:"amount_minor"`
	Currency             string                              `gorm:"type:char(3);not null;default:'INR'" json:"currency"`
	Receipt              string                              `gorm:"type:varchar(40);not null" json:"receipt"`
	Status               constants.WalletRechargeOrderStatus `gorm:"type:varchar(30);not null;default:'PROVIDER_PENDING'" json:"status"`
	ProviderOrderPayload JSONB                               `gorm:"type:jsonb;not null;default:'{}'" json:"provider_order_payload"`
	ProviderCreatedAt    *time.Time                          `json:"provider_created_at,omitempty"`
	CreatedAt            time.Time                           `gorm:"not null" json:"created_at"`
	UpdatedAt            time.Time                           `gorm:"not null" json:"updated_at"`
	Payments             []WalletRechargePayment             `gorm:"foreignKey:RechargeOrderID" json:"payments,omitempty"`
}

type WalletRechargePayment struct {
	ID                uuid.UUID                             `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID             uuid.UUID                             `gorm:"type:uuid;not null;index" json:"cpo_id"`
	RechargeOrderID   uuid.UUID                             `gorm:"type:uuid;not null;index" json:"recharge_order_id"`
	ProviderPaymentID string                                `gorm:"type:varchar(100);not null" json:"provider_payment_id"`
	ProviderOrderID   string                                `gorm:"type:varchar(100);not null" json:"provider_order_id"`
	AmountMinor       int64                                 `gorm:"not null" json:"amount_minor"`
	Currency          string                                `gorm:"type:char(3);not null" json:"currency"`
	Status            constants.WalletRechargePaymentStatus `gorm:"type:varchar(30);not null" json:"status"`
	PaymentMethod     string                                `gorm:"type:varchar(50)" json:"payment_method,omitempty"`
	ProviderFeeMinor  *int64                                `json:"provider_fee_minor,omitempty"`
	ProviderTaxMinor  *int64                                `json:"provider_tax_minor,omitempty"`
	ErrorCode         *string                               `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	ErrorDescription  *string                               `gorm:"type:varchar(500)" json:"error_description,omitempty"`
	PaymentSignature  *string                               `gorm:"type:varchar(128)" json:"payment_signature,omitempty"`
	SignatureVerified bool                                  `gorm:"not null;default:false" json:"signature_verified"`
	ProviderPayload   JSONB                                 `gorm:"type:jsonb;not null;default:'{}'" json:"provider_payload"`
	ProviderCreatedAt *time.Time                            `json:"provider_created_at,omitempty"`
	AuthorizedAt      *time.Time                            `json:"authorized_at,omitempty"`
	CapturedAt        *time.Time                            `json:"captured_at,omitempty"`
	CreatedAt         time.Time                             `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time                             `gorm:"not null" json:"updated_at"`
}

type WalletRechargeRefund struct {
	ID                uuid.UUID                            `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CPOID             uuid.UUID                            `gorm:"type:uuid;not null;index" json:"cpo_id"`
	RechargeOrderID   uuid.UUID                            `gorm:"type:uuid;not null;index" json:"recharge_order_id"`
	RechargePaymentID *uuid.UUID                           `gorm:"type:uuid;index" json:"recharge_payment_id,omitempty"`
	ProviderRefundID  *string                              `gorm:"type:varchar(100)" json:"provider_refund_id,omitempty"`
	ProviderPaymentID string                               `gorm:"type:varchar(100);not null" json:"provider_payment_id"`
	AmountMinor       int64                                `gorm:"not null" json:"amount_minor"`
	Currency          string                               `gorm:"type:char(3);not null" json:"currency"`
	Status            constants.WalletRechargeRefundStatus `gorm:"type:varchar(30);not null" json:"status"`
	Receipt           *string                              `gorm:"type:varchar(40)" json:"receipt,omitempty"`
	SpeedProcessed    *string                              `gorm:"type:varchar(30)" json:"speed_processed,omitempty"`
	ProviderPayload   JSONB                                `gorm:"type:jsonb;not null;default:'{}'" json:"provider_payload"`
	ProviderCreatedAt *time.Time                           `json:"provider_created_at,omitempty"`
	CreatedAt         time.Time                            `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time                            `gorm:"not null" json:"updated_at"`
}
