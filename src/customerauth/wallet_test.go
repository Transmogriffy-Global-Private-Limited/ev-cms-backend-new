package customerauth

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCustomerWalletTransactionQueryValidation(t *testing.T) {
	t.Parallel()

	query := CustomerWalletTransactionQuery{}
	if err := validateCustomerWalletTransactionQuery(&query); err != nil {
		t.Fatalf("default wallet transaction query rejected: %v", err)
	}
	if query.Limit != customerWalletDefaultLimit {
		t.Fatalf("default wallet transaction limit=%d, want %d", query.Limit, customerWalletDefaultLimit)
	}
	query.Limit = customerWalletMaxLimit + 1
	if err := validateCustomerWalletTransactionQuery(&query); err == nil {
		t.Fatal("overlarge wallet transaction limit was accepted")
	}
}

func TestCustomerWalletViewProjectsCurrentCPOPolicy(t *testing.T) {
	t.Parallel()
	wallet := models.Wallet{ID: uuid.New(), Balance: decimal.NewFromInt(499), Currency: "INR", UpdatedAt: time.Now().UTC()}
	view := customerWalletView(wallet, models.Settings{WalletMinBalance: 500, WalletBufferMinBalance: 20})
	if view.Balance != "499.00" || view.UsableBalance != "479.00" || view.MinimumRechargeAmount != "1.00" || view.WalletMinBalance != 500 || view.WalletBufferMinBalance != 20 {
		t.Fatalf("wallet policy view=%+v, want balance 499, usable 479, recharge 1, minimum 500, buffer 20", view)
	}

	view = customerWalletView(models.Wallet{ID: uuid.New(), Balance: decimal.NewFromInt(10), Currency: "INR", UpdatedAt: time.Now().UTC()}, models.Settings{WalletMinBalance: 0, WalletBufferMinBalance: 20})
	if view.UsableBalance != "0.00" || view.MinimumRechargeAmount != "0.00" {
		t.Fatalf("buffer-only wallet policy view=%+v, want zero usable/recharge", view)
	}
}
