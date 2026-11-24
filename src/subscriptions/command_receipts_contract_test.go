package subscriptions

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// These unit tests execute with TEST_DATABASE_URL unset. They cover the
// command-identity boundary without requiring another PostgreSQL cluster.
func TestSubscriptionMutationFingerprintUsesExecutedFields(t *testing.T) {
	cpo := uuid.New()
	version := uuid.New()
	started := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		operation      string
		withWhitespace any
		canonical      any
	}{
		{"renew", "RENEW",
			RenewRequest{StartsAt: &started, Reason: "  manually paid  ", IdempotencyKey: " key-1 "},
			RenewRequest{StartsAt: &started, Reason: "manually paid", IdempotencyKey: "key-1"}},
		{"plan change", "CHANGE_PLAN",
			ChangePlanRequest{PlanVersionID: version, Reason: " plan change ", IdempotencyKey: " key-2 "},
			ChangePlanRequest{PlanVersionID: version, Reason: "plan change", IdempotencyKey: "key-2"}},
		{"pause", "PAUSE",
			TransitionRequest{Reason: "  paused ", IdempotencyKey: " key-3 "},
			TransitionRequest{Reason: "paused", IdempotencyKey: "key-3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalized, err := canonicalSubscriptionMutationRequest(tc.withWhitespace,
				map[string]string{"renew": "manually paid", "plan change": "plan change", "pause": "paused"}[tc.name],
				map[string]string{"renew": "key-1", "plan change": "key-2", "pause": "key-3"}[tc.name])
			if err != nil {
				t.Fatal(err)
			}
			got, err := subscriptionCommandDigest(tc.operation, cpo, normalized)
			if err != nil {
				t.Fatal(err)
			}
			want, err := subscriptionCommandDigest(tc.operation, cpo, tc.canonical)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("fingerprint differed after API normalization: %q vs %q", got, want)
			}
			if got == "" {
				t.Fatal("empty digest")
			}
		})
	}
}

func TestSubscriptionMutationDigestPreservesMaterialDifferences(t *testing.T) {
	cpo := uuid.New()
	base := RenewRequest{Reason: "manual renewal", IdempotencyKey: "receipt"}
	digest := func(op string, id uuid.UUID, request any) string {
		t.Helper()
		value, err := subscriptionCommandDigest(op, id, request)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	reference := digest("RENEW", cpo, base)
	explicit := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if reference == digest("RENEW", cpo, RenewRequest{StartsAt: &explicit, Reason: base.Reason, IdempotencyKey: base.IdempotencyKey}) {
		t.Fatal("omitted and explicitly supplied starts_at must stay distinct")
	}
	if reference == digest("RENEW", uuid.New(), base) {
		t.Fatal("cross-CPO digest collision")
	}
	if reference == digest("PAUSE", cpo, base) {
		t.Fatal("operation omitted from digest")
	}
	if reference == digest("RENEW", cpo, RenewRequest{Reason: "different payment", IdempotencyKey: base.IdempotencyKey}) {
		t.Fatal("different reason must be rejected for same key")
	}
	if _, err := canonicalSubscriptionMutationRequest(struct{}{}, "reason", "key"); err == nil {
		t.Fatal("unknown command type accepted")
	}
}

func TestSubscriptionCommandReceiptIdentity(t *testing.T) {
	cpoID, subscriptionID, versionID, planID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	history := models.CPOSubscriptionHistory{SubscriptionID: subscriptionID, CPOID: cpoID, NextStatus: "ACTIVE", NextPlanVersionID: versionID}
	response := SubscriptionView{
		Subscription: models.CPOSubscription{ID: subscriptionID, CPOID: cpoID, PlanVersionID: versionID, Status: "ACTIVE"},
		Version:      models.SubscriptionPlanVersion{ID: versionID, PlanID: planID},
		Plan:         models.SubscriptionPlan{ID: planID},
	}
	if !validSubscriptionCommandReceipt(history, response) {
		t.Fatal("valid frozen receipt rejected")
	}
	wrong := response
	wrong.Subscription.ID = uuid.New()
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("cross-subscription receipt accepted")
	}
	wrong = response
	wrong.Subscription.CPOID = uuid.New()
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("cross-tenant receipt accepted")
	}
	wrong = response
	wrong.Subscription.Status = "EXPIRED"
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("mutated-status receipt accepted")
	}
	wrong = response
	wrong.Subscription.PlanVersionID = uuid.New()
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("mismatched plan version accepted")
	}
	wrong = response
	wrong.Version.PlanID = uuid.New()
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("mismatched plan accepted")
	}
	wrong = response
	wrong.Plan.ID = uuid.Nil
	if validSubscriptionCommandReceipt(history, wrong) {
		t.Fatal("empty plan identity accepted")
	}
}

// Executed only with an explicitly selected disposable TEST_DATABASE_URL.
// Replay must return the initial frozen response, not apply renewal twice.
func TestSubscriptionTrimmedMutationReplayWithPostgreSQL(t *testing.T) {
	database := repairTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	cpo := repairTestCPO(t, database, now)
	service := NewService(database, nil)
	service.now = func() time.Time { return now }
	version := repairTestPlan(t, service, principal)
	if _, err := service.Issue(context.Background(), principal, cpo.ID,
		IssueRequest{PlanVersionID: version, Reason: "Initial subscription", IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	key := "renew-" + uuid.NewString()
	first, err := service.Renew(context.Background(), principal, cpo.ID,
		RenewRequest{Reason: "  payment confirmed  ", IdempotencyKey: " " + key + " "})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Renew(context.Background(), principal, cpo.ID,
		RenewRequest{Reason: "payment confirmed", IdempotencyKey: key})
	if err != nil {
		t.Fatalf("canonical retry failed: %v", err)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Fatal("retry did not return exact original subscription response")
	}
	if _, err := service.Renew(context.Background(), principal, cpo.ID,
		RenewRequest{Reason: "different external record", IdempotencyKey: key}); err == nil {
		t.Fatal("same key with a materially different reason unexpectedly succeeded")
	}
	var histories int64
	if err := database.Model(&models.CPOSubscriptionHistory{}).
		Where("actor_user_id = ? AND idempotency_key = ?", principal.UserID, key).
		Count(&histories).Error; err != nil {
		t.Fatal(err)
	}
	if histories != 1 {
		t.Fatalf("expected one immutable renewal receipt, found %d", histories)
	}
}
