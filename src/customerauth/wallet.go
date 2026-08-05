package customerauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CustomerWalletView struct {
	ID        uuid.UUID `json:"id"`
	Balance   string    `json:"balance"`
	Currency  string    `json:"currency"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CustomerWalletTransactionView struct {
	ID              uuid.UUID  `json:"id"`
	Amount          string     `json:"amount"`
	TransactionType string     `json:"transaction_type"`
	Description     string     `json:"description"`
	SessionID       *uuid.UUID `json:"session_id,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
}

type CustomerWalletTransactionQuery struct {
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
}

type CustomerWalletResponse struct {
	Wallet CustomerWalletView `json:"wallet"`
}

type CustomerWalletTransactionListResponse struct {
	Wallet       CustomerWalletView              `json:"wallet"`
	Transactions []CustomerWalletTransactionView `json:"transactions"`
	NextBefore   *time.Time                      `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID                      `json:"next_before_id,omitempty"`
	HasMore      bool                            `json:"has_more"`
}

const (
	customerWalletDefaultLimit = 25
	customerWalletMaxLimit     = 100
)

func (service *Service) GetCustomerWallet(ctx context.Context, principal Principal) (CustomerWalletResponse, error) {
	wallet, err := service.loadCustomerWallet(ctx, principal)
	if err != nil {
		return CustomerWalletResponse{}, err
	}
	return CustomerWalletResponse{Wallet: customerWalletView(wallet)}, nil
}

func (service *Service) ListCustomerWalletTransactions(ctx context.Context, principal Principal, query CustomerWalletTransactionQuery) (CustomerWalletTransactionListResponse, error) {
	if err := validateCustomerWalletTransactionQuery(&query); err != nil {
		return CustomerWalletTransactionListResponse{}, err
	}
	wallet, err := service.loadCustomerWallet(ctx, principal)
	if err != nil {
		return CustomerWalletTransactionListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND wallet_id = ?", principal.CPOID, wallet.ID).
		Order("created_at DESC, id DESC")
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(created_at, id) < (?, ?)", *query.Before, *query.BeforeID)
	}
	var records []models.WalletTransaction
	if err := databaseQuery.Limit(query.Limit + 1).Find(&records).Error; err != nil {
		return CustomerWalletTransactionListResponse{}, fmt.Errorf("list customer wallet transactions: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	transactions := make([]CustomerWalletTransactionView, 0, len(records))
	for _, record := range records {
		transactions = append(transactions, CustomerWalletTransactionView{
			ID:              record.ID,
			Amount:          record.Amount.StringFixed(2),
			TransactionType: string(record.TransactionType),
			Description:     record.Description,
			SessionID:       record.SessionID,
			Status:          string(record.Status),
			CreatedAt:       record.CreatedAt,
		})
	}
	response := CustomerWalletTransactionListResponse{
		Wallet:       customerWalletView(wallet),
		Transactions: transactions,
		HasMore:      hasMore,
	}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		response.NextBefore = &last.CreatedAt
		response.NextBeforeID = &last.ID
	}
	return response, nil
}

func validateCustomerWalletTransactionQuery(query *CustomerWalletTransactionQuery) error {
	if query.Limit == 0 {
		query.Limit = customerWalletDefaultLimit
	}
	if query.Limit < 1 || query.Limit > customerWalletMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Both before and before_id are required together."}
	}
	return nil
}

func (service *Service) loadCustomerWallet(ctx context.Context, principal Principal) (models.Wallet, error) {
	var wallet models.Wallet
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ? AND customer_id = ?", principal.CPOID, principal.CustomerID).
		First(&wallet).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.Wallet{}, &APIError{http.StatusNotFound, "wallet_not_found", "The customer wallet was not found."}
		}
		return models.Wallet{}, fmt.Errorf("load customer wallet: %w", err)
	}
	return wallet, nil
}

func customerWalletView(wallet models.Wallet) CustomerWalletView {
	return CustomerWalletView{ID: wallet.ID, Balance: wallet.Balance.StringFixed(2), Currency: wallet.Currency, UpdatedAt: wallet.UpdatedAt}
}
