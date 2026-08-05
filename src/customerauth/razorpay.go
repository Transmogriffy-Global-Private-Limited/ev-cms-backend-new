package customerauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	IdempotencyKeyHeader         = "Idempotency-Key"
	walletRechargeProvider       = "RAZORPAY"
	walletRechargeCurrency       = "INR"
	maxWalletRechargeMinor int64 = 99999999999999
)

type RazorpayCredentials struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
}

type RazorpayCredentialResolver func(context.Context, uuid.UUID) (RazorpayCredentials, error)

type RazorpayClient interface {
	CreateOrder(map[string]interface{}) (map[string]interface{}, error)
	FetchPayment(string) (map[string]interface{}, error)
}

type RazorpayClientFactory func(RazorpayCredentials) RazorpayClient

type razorpaySDKClient struct {
	client *razorpay.Client
}

func newRazorpayClient(credentials RazorpayCredentials) RazorpayClient {
	return &razorpaySDKClient{client: razorpay.NewClient(credentials.KeyID, credentials.KeySecret)}
}

func (client *razorpaySDKClient) CreateOrder(data map[string]interface{}) (map[string]interface{}, error) {
	return client.client.Order.Create(data, nil)
}

func (client *razorpaySDKClient) FetchPayment(paymentID string) (map[string]interface{}, error) {
	return client.client.Payment.Fetch(paymentID, nil, nil)
}

type CustomerRechargeOrderRequest struct {
	Amount string `json:"amount"`
}

type CustomerRechargeVerifyRequest struct {
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	RazorpaySignature string `json:"razorpay_signature"`
}

type CustomerRechargeOrderResponse struct {
	RechargeOrderID uuid.UUID `json:"recharge_order_id"`
	Provider        string    `json:"provider"`
	ProviderOrderID string    `json:"provider_order_id"`
	Amount          string    `json:"amount"`
	AmountMinor     int64     `json:"amount_minor"`
	Currency        string    `json:"currency"`
	ProviderKeyID   string    `json:"provider_key_id,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

func (service *Service) CreateWalletRechargeOrder(
	ctx context.Context,
	principal Principal,
	idempotencyKey string,
	request CustomerRechargeOrderRequest,
) (CustomerRechargeOrderResponse, error) {
	idempotencyKey, err := validateRechargeIdempotencyKey(idempotencyKey)
	if err != nil {
		return CustomerRechargeOrderResponse{}, err
	}
	amountMinor, err := parseRechargeAmount(request.Amount)
	if err != nil {
		return CustomerRechargeOrderResponse{}, err
	}

	var existing models.WalletRechargeOrder
	result := service.database.WithContext(ctx).
		Where("cpo_id = ? AND customer_id = ? AND idempotency_key = ?", principal.CPOID, principal.CustomerID, idempotencyKey).
		First(&existing)
	if result.Error == nil {
		if existing.ProviderOrderID == nil {
			if existing.Status == constants.WalletRechargeOrderStatusFailed {
				return CustomerRechargeOrderResponse{}, &APIError{http.StatusBadGateway, "payment_provider_unavailable", "The payment provider could not create the recharge order. Retry with a new idempotency key."}
			}
			return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "recharge_order_pending", "This recharge request is still being prepared."}
		}
		if existing.AmountMinor != amountMinor {
			return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "idempotency_conflict", "The idempotency key was already used for a different amount."}
		}
		keyID := ""
		if existing.Status == constants.WalletRechargeOrderStatusPaymentPending {
			if service.razorpayResolver == nil {
				return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
			}
			credentials, resolveErr := service.razorpayResolver(ctx, principal.CPOID)
			if resolveErr != nil {
				return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
			}
			keyID = credentials.KeyID
		}
		return service.rechargeOrderResponse(existing, keyID), nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return CustomerRechargeOrderResponse{}, fmt.Errorf("find wallet recharge order: %w", result.Error)
	}

	if service.razorpayResolver == nil || service.razorpayFactory == nil {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
	}
	credentials, err := service.razorpayResolver(ctx, principal.CPOID)
	if err != nil {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
	}
	wallet, err := service.loadCustomerWallet(ctx, principal)
	if err != nil {
		return CustomerRechargeOrderResponse{}, err
	}
	if wallet.Currency != walletRechargeCurrency {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "unsupported_wallet_currency", "This wallet currency cannot be recharged."}
	}

	now := service.now()
	order := models.WalletRechargeOrder{
		ID:                   uuid.New(),
		CPOID:                principal.CPOID,
		CustomerID:           principal.CustomerID,
		WalletID:             wallet.ID,
		Provider:             walletRechargeProvider,
		IdempotencyKey:       idempotencyKey,
		AmountMinor:          amountMinor,
		Currency:             walletRechargeCurrency,
		Receipt:              "rcg_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Status:               constants.WalletRechargeOrderStatusProviderPending,
		ProviderOrderPayload: models.JSONB{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := service.database.WithContext(ctx).Create(&order).Error; err != nil {
		return CustomerRechargeOrderResponse{}, fmt.Errorf("create wallet recharge order: %w", err)
	}

	providerOrder, err := service.razorpayFactory(credentials).CreateOrder(map[string]interface{}{
		"amount":          amountMinor,
		"currency":        walletRechargeCurrency,
		"receipt":         order.Receipt,
		"partial_payment": false,
		"notes": map[string]interface{}{
			"cms_recharge_order_id": order.ID.String(),
		},
	})
	if err != nil {
		service.markRechargeOrderFailed(ctx, order.ID, principal.CPOID)
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusBadGateway, "payment_provider_unavailable", "The payment provider could not create the recharge order."}
	}
	providerOrderID := providerString(providerOrder, "id")
	if providerOrderID == "" {
		service.markRechargeOrderFailed(ctx, order.ID, principal.CPOID)
		return CustomerRechargeOrderResponse{}, fmt.Errorf("Razorpay order response did not contain an order ID")
	}
	if providerAmount, ok := providerInt64(providerOrder, "amount"); ok && providerAmount != amountMinor {
		service.markRechargeOrderFailed(ctx, order.ID, principal.CPOID)
		return CustomerRechargeOrderResponse{}, fmt.Errorf("Razorpay order amount did not match the requested amount")
	}
	if providerCurrency := providerString(providerOrder, "currency"); providerCurrency != "" && providerCurrency != walletRechargeCurrency {
		service.markRechargeOrderFailed(ctx, order.ID, principal.CPOID)
		return CustomerRechargeOrderResponse{}, fmt.Errorf("Razorpay order currency did not match the wallet currency")
	}
	providerCreatedAt := providerTimestamp(providerOrder, "created_at")
	updates := map[string]interface{}{
		"provider_order_id":      providerOrderID,
		"status":                 constants.WalletRechargeOrderStatusPaymentPending,
		"provider_order_payload": models.JSONB(providerSnapshot(providerOrder)),
		"provider_created_at":    providerCreatedAt,
		"updated_at":             service.now(),
	}
	if err := service.database.WithContext(ctx).Model(&models.WalletRechargeOrder{}).
		Where("id = ? AND cpo_id = ?", order.ID, principal.CPOID).Updates(updates).Error; err != nil {
		return CustomerRechargeOrderResponse{}, fmt.Errorf("store Razorpay order: %w", err)
	}
	order.ProviderOrderID = &providerOrderID
	order.Status = constants.WalletRechargeOrderStatusPaymentPending
	order.ProviderOrderPayload = models.JSONB(providerSnapshot(providerOrder))
	order.ProviderCreatedAt = providerCreatedAt
	order.UpdatedAt = updates["updated_at"].(time.Time)
	return service.rechargeOrderResponse(order, credentials.KeyID), nil
}

func (service *Service) VerifyWalletRecharge(
	ctx context.Context,
	principal Principal,
	request CustomerRechargeVerifyRequest,
) (CustomerRechargeOrderResponse, error) {
	request.RazorpayOrderID = strings.TrimSpace(request.RazorpayOrderID)
	request.RazorpayPaymentID = strings.TrimSpace(request.RazorpayPaymentID)
	request.RazorpaySignature = strings.TrimSpace(request.RazorpaySignature)
	if request.RazorpayOrderID == "" || request.RazorpayPaymentID == "" || request.RazorpaySignature == "" {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusBadRequest, "invalid_request", "Razorpay order ID, payment ID, and signature are required."}
	}
	if service.razorpayResolver == nil || service.razorpayFactory == nil {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
	}
	var order models.WalletRechargeOrder
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ? AND customer_id = ? AND provider = ? AND provider_order_id = ?", principal.CPOID, principal.CustomerID, walletRechargeProvider, request.RazorpayOrderID).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CustomerRechargeOrderResponse{}, &APIError{http.StatusNotFound, "recharge_order_not_found", "The recharge order was not found."}
		}
		return CustomerRechargeOrderResponse{}, fmt.Errorf("find wallet recharge order: %w", err)
	}
	if order.Status == constants.WalletRechargeOrderStatusPaid {
		var payment models.WalletRechargePayment
		if err := service.database.WithContext(ctx).
			Where("cpo_id = ? AND recharge_order_id = ? AND provider_payment_id = ?", principal.CPOID, order.ID, request.RazorpayPaymentID).
			First(&payment).Error; err == nil {
			return service.rechargeOrderResponse(order, ""), nil
		}
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "recharge_already_completed", "This recharge order has already been completed."}
	}
	credentials, err := service.razorpayResolver(ctx, principal.CPOID)
	if err != nil {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusServiceUnavailable, "payment_provider_not_configured", "Wallet recharge is temporarily unavailable."}
	}
	if !verifyRazorpayPaymentSignature(
		request.RazorpayOrderID,
		request.RazorpayPaymentID,
		request.RazorpaySignature,
		credentials.KeySecret,
	) {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusBadRequest, "invalid_payment_signature", "The Razorpay payment signature is invalid."}
	}
	providerPayment, err := service.razorpayFactory(credentials).FetchPayment(request.RazorpayPaymentID)
	if err != nil {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusBadGateway, "payment_provider_unavailable", "The payment provider could not verify the payment."}
	}
	if providerString(providerPayment, "id") != request.RazorpayPaymentID ||
		providerString(providerPayment, "order_id") != request.RazorpayOrderID {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "payment_order_mismatch", "The Razorpay payment does not belong to this recharge order."}
	}
	providerAmount, ok := providerInt64(providerPayment, "amount")
	if !ok || providerAmount != order.AmountMinor || providerString(providerPayment, "currency") != order.Currency {
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "payment_amount_mismatch", "The Razorpay payment does not match this recharge order."}
	}
	paymentStatus := rechargePaymentStatus(providerPayment)
	payment := buildRechargePayment(order, providerPayment, request.RazorpayPaymentID, request.RazorpaySignature, paymentStatus, true, service.now())
	if paymentStatus != constants.WalletRechargePaymentStatusCaptured {
		if err := service.storeUncapturedRechargePayment(ctx, order, payment); err != nil {
			return CustomerRechargeOrderResponse{}, err
		}
		return CustomerRechargeOrderResponse{}, &APIError{http.StatusConflict, "payment_not_captured", "The Razorpay payment has not been captured."}
	}
	if err := service.creditCapturedRecharge(ctx, principal, order, payment); err != nil {
		return CustomerRechargeOrderResponse{}, err
	}
	order.Status = constants.WalletRechargeOrderStatusPaid
	return service.rechargeOrderResponse(order, credentials.KeyID), nil
}

func verifyRazorpayPaymentSignature(orderID, paymentID, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(orderID + "|" + paymentID))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (service *Service) creditCapturedRecharge(
	ctx context.Context,
	principal Principal,
	order models.WalletRechargeOrder,
	payment models.WalletRechargePayment,
) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedOrder models.WalletRechargeOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND customer_id = ? AND id = ?", principal.CPOID, principal.CustomerID, order.ID).
			First(&lockedOrder).Error; err != nil {
			return fmt.Errorf("lock wallet recharge order: %w", err)
		}
		if lockedOrder.Status == constants.WalletRechargeOrderStatusPaid {
			return nil
		}
		var wallet models.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND customer_id = ? AND id = ?", principal.CPOID, principal.CustomerID, lockedOrder.WalletID).
			First(&wallet).Error; err != nil {
			return fmt.Errorf("lock customer wallet: %w", err)
		}
		var existingTransaction models.WalletTransaction
		if err := tx.Where("cpo_id = ? AND wallet_id = ? AND recharge_order_id = ?", principal.CPOID, wallet.ID, lockedOrder.ID).
			First(&existingTransaction).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find recharge wallet transaction: %w", err)
		}
		payment.CPOID = principal.CPOID
		payment.RechargeOrderID = lockedOrder.ID
		if err := upsertRechargePayment(tx, payment); err != nil {
			return err
		}
		amount := decimal.NewFromInt(payment.AmountMinor).Shift(-2)
		transaction := models.WalletTransaction{
			ID:              uuid.New(),
			CPOID:           principal.CPOID,
			WalletID:        wallet.ID,
			Amount:          amount,
			TransactionType: constants.WalletTransactionTypeCredit,
			Description:     "Razorpay wallet recharge",
			IdempotencyKey:  stringPtr("razorpay-recharge:" + lockedOrder.ID.String()),
			Status:          constants.FinancialStatusCompleted,
			CreatedAt:       service.now(),
			UpdatedAt:       service.now(),
			RechargeOrderID: &lockedOrder.ID,
		}
		if err := tx.Create(&transaction).Error; err != nil {
			return fmt.Errorf("create recharge wallet transaction: %w", err)
		}
		wallet.Balance = wallet.Balance.Add(amount)
		wallet.UpdatedAt = service.now()
		if err := tx.Model(&wallet).Select("balance", "updated_at").Updates(&wallet).Error; err != nil {
			return fmt.Errorf("credit customer wallet: %w", err)
		}
		return tx.Model(&lockedOrder).Updates(map[string]interface{}{
			"status":     constants.WalletRechargeOrderStatusPaid,
			"updated_at": service.now(),
		}).Error
	})
}

func (service *Service) storeUncapturedRechargePayment(
	ctx context.Context,
	order models.WalletRechargeOrder,
	payment models.WalletRechargePayment,
) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		payment.CPOID = order.CPOID
		payment.RechargeOrderID = order.ID
		if err := upsertRechargePayment(tx, payment); err != nil {
			return err
		}
		return tx.Model(&order).Updates(map[string]interface{}{
			"status":     constants.WalletRechargeOrderStatusPaymentPending,
			"updated_at": service.now(),
		}).Error
	})
}

func upsertRechargePayment(tx *gorm.DB, payment models.WalletRechargePayment) error {
	var existing models.WalletRechargePayment
	result := tx.Where("cpo_id = ? AND provider_payment_id = ?", payment.CPOID, payment.ProviderPaymentID).First(&existing)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := tx.Create(&payment).Error; err != nil {
			return fmt.Errorf("store Razorpay payment: %w", err)
		}
		return nil
	}
	if result.Error != nil {
		return fmt.Errorf("find Razorpay payment: %w", result.Error)
	}
	if existing.RechargeOrderID != payment.RechargeOrderID {
		return &APIError{http.StatusConflict, "payment_already_linked", "The Razorpay payment is linked to another recharge order."}
	}
	return tx.Model(&existing).Updates(map[string]interface{}{
		"status":              payment.Status,
		"amount_minor":        payment.AmountMinor,
		"currency":            payment.Currency,
		"provider_order_id":   payment.ProviderOrderID,
		"payment_method":      payment.PaymentMethod,
		"provider_fee_minor":  payment.ProviderFeeMinor,
		"provider_tax_minor":  payment.ProviderTaxMinor,
		"error_code":          payment.ErrorCode,
		"error_description":   payment.ErrorDescription,
		"payment_signature":   payment.PaymentSignature,
		"signature_verified":  payment.SignatureVerified,
		"provider_payload":    payment.ProviderPayload,
		"provider_created_at": payment.ProviderCreatedAt,
		"authorized_at":       payment.AuthorizedAt,
		"captured_at":         payment.CapturedAt,
		"updated_at":          payment.UpdatedAt,
	}).Error
}

func buildRechargePayment(
	order models.WalletRechargeOrder,
	data map[string]interface{},
	providerPaymentID string,
	paymentSignature string,
	status constants.WalletRechargePaymentStatus,
	signatureVerified bool,
	now time.Time,
) models.WalletRechargePayment {
	return models.WalletRechargePayment{
		ID:                uuid.New(),
		CPOID:             order.CPOID,
		RechargeOrderID:   order.ID,
		ProviderPaymentID: providerPaymentID,
		ProviderOrderID:   rechargeProviderOrderID(order),
		AmountMinor:       order.AmountMinor,
		Currency:          order.Currency,
		Status:            status,
		PaymentMethod:     providerString(data, "method"),
		ProviderFeeMinor:  providerInt64Ptr(data, "fee"),
		ProviderTaxMinor:  providerInt64Ptr(data, "tax"),
		ErrorCode:         stringValuePtr(data, "error_code"),
		ErrorDescription:  stringValuePtr(data, "error_description"),
		PaymentSignature:  stringPtr(paymentSignature),
		SignatureVerified: signatureVerified,
		ProviderPayload:   models.JSONB(providerSnapshot(data)),
		ProviderCreatedAt: providerTimestamp(data, "created_at"),
		AuthorizedAt:      providerTimestamp(data, "authorized_at"),
		CapturedAt:        providerTimestamp(data, "captured_at"),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func rechargeProviderOrderID(order models.WalletRechargeOrder) string {
	if order.ProviderOrderID == nil {
		return ""
	}
	return *order.ProviderOrderID
}

func rechargePaymentStatus(data map[string]interface{}) constants.WalletRechargePaymentStatus {
	if captured, ok := data["captured"].(bool); ok && captured {
		return constants.WalletRechargePaymentStatusCaptured
	}
	switch strings.ToLower(providerString(data, "status")) {
	case "captured":
		return constants.WalletRechargePaymentStatusCaptured
	case "authorized":
		return constants.WalletRechargePaymentStatusAuthorized
	default:
		return constants.WalletRechargePaymentStatusFailed
	}
}

func (service *Service) markRechargeOrderFailed(ctx context.Context, orderID, cpoID uuid.UUID) {
	_ = service.database.WithContext(ctx).Model(&models.WalletRechargeOrder{}).
		Where("id = ? AND cpo_id = ? AND provider_order_id IS NULL", orderID, cpoID).
		Updates(map[string]interface{}{"status": constants.WalletRechargeOrderStatusFailed, "updated_at": service.now()}).Error
}

func (service *Service) rechargeOrderResponse(order models.WalletRechargeOrder, keyID string) CustomerRechargeOrderResponse {
	return CustomerRechargeOrderResponse{
		RechargeOrderID: order.ID,
		Provider:        order.Provider,
		ProviderOrderID: rechargeProviderOrderID(order),
		Amount:          decimal.NewFromInt(order.AmountMinor).Shift(-2).StringFixed(2),
		AmountMinor:     order.AmountMinor,
		Currency:        order.Currency,
		ProviderKeyID:   keyID,
		Status:          string(order.Status),
		CreatedAt:       order.CreatedAt,
	}
}

func validateRechargeIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 120 || strings.ContainsAny(value, "\r\n\x00") {
		return "", &APIError{http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must contain 1 to 120 safe characters."}
	}
	return value, nil
}

func parseRechargeAmount(value string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || amount.Sign() <= 0 {
		return 0, &APIError{http.StatusBadRequest, "invalid_amount", "Amount must be a positive INR value with at most two decimal places."}
	}
	minor := amount.Shift(2)
	if !minor.IsInteger() || minor.IntPart() <= 0 || minor.IntPart() > maxWalletRechargeMinor {
		return 0, &APIError{http.StatusBadRequest, "invalid_amount", "Amount must be a positive INR value with at most two decimal places."}
	}
	return minor.IntPart(), nil
}

func providerString(data map[string]interface{}, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func providerInt64(data map[string]interface{}, key string) (int64, bool) {
	value, ok := data[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed < math.MinInt64 || typed > math.MaxInt64 || typed != math.Trunc(typed) {
			return 0, false
		}
		return int64(typed), true
	case string:
		var parsed int64
		if _, err := fmt.Sscan(typed, &parsed); err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func providerInt64Ptr(data map[string]interface{}, key string) *int64 {
	value, ok := providerInt64(data, key)
	if !ok {
		return nil
	}
	return &value
}

func providerTimestamp(data map[string]interface{}, key string) *time.Time {
	value, ok := providerInt64(data, key)
	if !ok || value <= 0 {
		return nil
	}
	timestamp := time.Unix(value, 0).UTC()
	return &timestamp
}

func stringValuePtr(data map[string]interface{}, key string) *string {
	value := providerString(data, key)
	if value == "" {
		return nil
	}
	return &value
}

func providerSnapshot(data map[string]interface{}) map[string]any {
	return sanitizeProviderMap(data)
}

func sanitizeProviderMap(data map[string]interface{}) map[string]any {
	result := make(map[string]any, len(data))
	for key, value := range data {
		if sensitiveProviderField(key) {
			continue
		}
		result[key] = sanitizeProviderValue(value)
	}
	return result
}

func sanitizeProviderValue(value any) any {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeProviderMap(typed)
	case []interface{}:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeProviderValue(item))
		}
		return result
	default:
		return value
	}
}

func sensitiveProviderField(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
	case "key", "secret", "key_secret", "webhook_secret", "token",
		"access_token", "authorization", "password", "cvv", "card_number",
		"number", "expiry_month", "expiry_year":
		return true
	default:
		return false
	}
}

func stringPtr(value string) *string {
	return &value
}
