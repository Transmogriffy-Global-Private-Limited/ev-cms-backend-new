package constants

type WalletTransactionType string

const (
	WalletTransactionTypeCredit WalletTransactionType = "CREDIT"
	WalletTransactionTypeDebit  WalletTransactionType = "DEBIT"
)

func (transactionType WalletTransactionType) Valid() bool {
	switch transactionType {
	case WalletTransactionTypeCredit, WalletTransactionTypeDebit:
		return true
	default:
		return false
	}
}

type FinancialStatus string

const (
	FinancialStatusPending   FinancialStatus = "PENDING"
	FinancialStatusCompleted FinancialStatus = "COMPLETED"
	FinancialStatusFailed    FinancialStatus = "FAILED"
	FinancialStatusReversed  FinancialStatus = "REVERSED"
	FinancialStatusRefunded  FinancialStatus = "REFUNDED"
)

func (status FinancialStatus) Valid() bool {
	switch status {
	case FinancialStatusPending,
		FinancialStatusCompleted,
		FinancialStatusFailed,
		FinancialStatusReversed,
		FinancialStatusRefunded:
		return true
	default:
		return false
	}
}

type WalletRechargeOrderStatus string

const (
	WalletRechargeOrderStatusProviderPending WalletRechargeOrderStatus = "PROVIDER_PENDING"
	WalletRechargeOrderStatusPaymentPending  WalletRechargeOrderStatus = "PAYMENT_PENDING"
	WalletRechargeOrderStatusPaid            WalletRechargeOrderStatus = "PAID"
	WalletRechargeOrderStatusFailed          WalletRechargeOrderStatus = "FAILED"
)

type WalletRechargePaymentStatus string

const (
	WalletRechargePaymentStatusAuthorized WalletRechargePaymentStatus = "AUTHORIZED"
	WalletRechargePaymentStatusCaptured   WalletRechargePaymentStatus = "CAPTURED"
	WalletRechargePaymentStatusFailed     WalletRechargePaymentStatus = "FAILED"
)

type WalletRechargeRefundStatus string

const (
	WalletRechargeRefundStatusPending   WalletRechargeRefundStatus = "PENDING"
	WalletRechargeRefundStatusProcessed WalletRechargeRefundStatus = "PROCESSED"
	WalletRechargeRefundStatusFailed    WalletRechargeRefundStatus = "FAILED"
)
