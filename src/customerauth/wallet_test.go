package customerauth

import "testing"

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
