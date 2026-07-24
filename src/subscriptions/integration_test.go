package subscriptions

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestSubscriptionLifecycleWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	actor := models.User{
		ID: uuid.New(), Email: uuid.NewString() + "@example.test",
		PasswordHash: "integration-test-only", FullName: "Subscription Tester",
		IsActive: true, IsVerified: true, PasswordChangedAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	cpo := models.CPO{
		ID: uuid.New(), Slug: "test-" + uuid.NewString()[:8],
		BusinessName:   "Subscription Test CPO",
		CompanyType:    constants.CPOCompanyTypeCompany,
		Status:         constants.CPOStatusActive,
		AppID:          "cpo_dummy_" + uuid.NewString(),
		AppIDMode:      constants.CPOAppIDModeDummy,
		AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&cpo).Error; err != nil {
		t.Fatalf("create CPO: %v", err)
	}

	service := NewService(gormDB, nil, false, nil)
	service.now = func() time.Time { return now }
	principal := auth.Principal{
		UserID: actor.ID,
		Scope:  constants.AuthScopePlatform,
	}
	limit := int64(10)
	plan, err := service.CreatePlan(ctx, principal, CreatePlanRequest{
		Code: "test_" + uuid.NewString()[:8],
		Name: "Lifecycle Test",
		Terms: PlanTermsInput{
			Currency: "INR", PriceMinor: 12500,
			BillingInterval: "MONTHLY", IntervalCount: 1,
			Entitlements: []EntitlementInput{
				{FeatureKey: "chargers.manage", Enabled: true, LimitValue: &limit},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	plan, err = service.PublishPlan(ctx, principal, plan.Plan.ID)
	if err != nil {
		t.Fatalf("publish plan: %v", err)
	}
	if len(plan.Published) != 1 {
		t.Fatalf("published versions = %d, want 1", len(plan.Published))
	}

	key := "assign-" + uuid.NewString()
	assigned, err := service.Assign(ctx, principal, cpo.ID, AssignRequest{
		PlanVersionID: plan.Published[0].ID,
		Reason:        "Initial test assignment", IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("assign subscription: %v", err)
	}
	replayed, err := service.Assign(ctx, principal, cpo.ID, AssignRequest{
		PlanVersionID: plan.Published[0].ID,
		Reason:        "Initial test assignment", IdempotencyKey: key,
	})
	if err != nil || replayed.Subscription.ID != assigned.Subscription.ID {
		t.Fatalf("idempotent assignment = %#v, %v", replayed, err)
	}
	if _, err := service.Pause(ctx, principal, cpo.ID, TransitionRequest{
		Reason: "Lifecycle test", IdempotencyKey: "pause-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("pause subscription: %v", err)
	}
	if _, err := service.Resume(ctx, principal, cpo.ID, TransitionRequest{
		Reason: "Lifecycle test", IdempotencyKey: "resume-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("resume subscription: %v", err)
	}

	overrideLimit := int64(25)
	if _, err := service.SetOverride(ctx, principal, cpo.ID, "chargers.manage", OverrideRequest{
		Enabled: true, LimitValue: &overrideLimit,
		Reason: "Temporary test expansion",
	}); err != nil {
		t.Fatalf("set entitlement override: %v", err)
	}
	effective, err := service.EffectiveEntitlements(ctx, principal, cpo.ID)
	if err != nil || len(effective.Entitlements) != 1 ||
		effective.Entitlements[0].Source != "OVERRIDE" {
		t.Fatalf("effective entitlements = %#v, %v", effective, err)
	}

	updated, err := service.UpdateDraft(ctx, principal, plan.Plan.ID, UpdateDraftRequest{
		Name: "Lifecycle Test",
		Terms: PlanTermsInput{
			Currency: "INR", PriceMinor: 25000,
			BillingInterval: "MONTHLY", IntervalCount: 1,
			Entitlements: []EntitlementInput{
				{FeatureKey: "chargers.manage", Enabled: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("create next draft: %v", err)
	}
	updated, err = service.PublishPlan(ctx, principal, updated.Plan.ID)
	if err != nil {
		t.Fatalf("publish next version: %v", err)
	}
	nextVersion := updated.Published[0]
	if nextVersion.Version != 2 {
		t.Fatalf("latest version = %d, want 2", nextVersion.Version)
	}
	scheduled, err := service.ChangePlan(ctx, principal, cpo.ID, ChangePlanRequest{
		PlanVersionID: nextVersion.ID, Effective: "PERIOD_END",
		Reason: "Scheduled upgrade", IdempotencyKey: "change-" + uuid.NewString(),
	})
	if err != nil || scheduled.Subscription.PendingPlanVersionID == nil {
		t.Fatalf("schedule plan change = %#v, %v", scheduled, err)
	}
	service.now = func() time.Time {
		return scheduled.Subscription.CurrentPeriodEndsAt.Add(time.Second)
	}
	if err := service.ReconcileDue(ctx); err != nil {
		t.Fatalf("reconcile plan change: %v", err)
	}
	current, err := service.GetCurrent(ctx, principal, cpo.ID)
	if err != nil || current.Subscription.PlanVersionID != nextVersion.ID {
		t.Fatalf("reconciled subscription = %#v, %v", current, err)
	}
	if _, err := service.Cancel(ctx, principal, cpo.ID, TransitionRequest{
		Reason: "End lifecycle test", IdempotencyKey: "cancel-" + uuid.NewString(),
	}); err != nil {
		t.Fatalf("cancel subscription: %v", err)
	}
	if _, err := service.GetCurrent(ctx, principal, cpo.ID); err == nil {
		t.Fatal("terminal subscription remained current")
	}
}
