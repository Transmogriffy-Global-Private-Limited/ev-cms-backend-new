package constants

import "testing"

func TestStatusValidation(t *testing.T) {
	t.Parallel()

	if !CPOCompanyTypeCompany.Valid() || CPOCompanyType("PARTNERSHIP").Valid() {
		t.Fatal("unexpected CPO company type validation result")
	}
	if !CPOStatusActive.Valid() || CPOStatus("DELETED").Valid() {
		t.Fatal("unexpected CPO status validation result")
	}
	if !MembershipStatusRevoked.Valid() || MembershipStatus("PENDING").Valid() {
		t.Fatal("unexpected membership status validation result")
	}
	if !CustomerStatusBlocked.Valid() || CustomerStatus("SUSPENDED").Valid() {
		t.Fatal("unexpected customer status validation result")
	}
	if !ChargerStatusSuspendedEVSE.Valid() || ChargerStatus("BROKEN").Valid() {
		t.Fatal("unexpected charger status validation result")
	}
	if !SessionStatusStopPending.Valid() || SessionStatus("STOPPED").Valid() {
		t.Fatal("unexpected charging session status validation result")
	}
	if !WalletTransactionTypeDebit.Valid() || WalletTransactionType("PAYMENT").Valid() {
		t.Fatal("unexpected wallet transaction type validation result")
	}
	if !FinancialStatusRefunded.Valid() || FinancialStatus("CANCELLED").Valid() {
		t.Fatal("unexpected financial status validation result")
	}
}
