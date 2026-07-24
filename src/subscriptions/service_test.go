package subscriptions

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

func TestValidatePlanRejectsDuplicateAndNegativeEntitlements(t *testing.T) {
	t.Parallel()

	negative := int64(-1)
	terms := normalizeTerms(PlanTermsInput{
		Currency:        "inr",
		PriceMinor:      100,
		BillingInterval: "monthly",
		Entitlements: []EntitlementInput{
			{FeatureKey: "chargers.manage", Enabled: true},
			{FeatureKey: "chargers.manage", Enabled: true, LimitValue: &negative},
		},
	})
	if err := validatePlan("starter", "Starter", "", terms); err == nil {
		t.Fatal("expected invalid entitlement set to fail")
	}
}

func TestSubscriptionServiceRejectsNonPlatformPrincipal(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, false, nil)
	_, err := service.ListPlans(
		t.Context(),
		auth.Principal{Scope: constants.AuthScopeCPO},
	)
	if err == nil {
		t.Fatal("expected CPO principal to be rejected")
	}
}
