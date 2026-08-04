package subscriptions

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

func TestValidatePlanRejectsInvalidCommercialTerms(t *testing.T) {
	t.Parallel()

	terms := normalizeTerms(PlanTermsInput{
		Currency:        "inr",
		PriceMinor:      -1,
		BillingInterval: "monthly",
	})
	if err := validatePlan("starter", "Starter", "", terms); err == nil {
		t.Fatal("expected invalid commercial terms to fail")
	}
}

func TestSubscriptionServiceRejectsNonPlatformPrincipal(t *testing.T) {
	t.Parallel()

	_, err := NewService(nil, nil).ListPlans(
		t.Context(),
		auth.Principal{Scope: constants.AuthScopeCPO},
	)
	if err == nil {
		t.Fatal("expected CPO principal to be rejected")
	}
}
