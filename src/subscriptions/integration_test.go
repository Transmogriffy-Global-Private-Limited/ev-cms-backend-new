package subscriptions

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestManualSubscriptionLifecycleWithPostgreSQL(t *testing.T) {
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
	actor := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", PasswordHash: "integration-test-only", FullName: "Subscription Tester", IsActive: true, IsVerified: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	cpo := models.CPO{ID: uuid.New(), Slug: "test-" + uuid.NewString()[:8], BusinessName: "Subscription Test CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: "27" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:10]) + "1Z5", Address: "1 Test Road", City: "Kolkata", State: "West Bengal", Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Integration test", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := gormDB.Create(&cpo).Error; err != nil {
		t.Fatalf("create CPO: %v", err)
	}

	service := NewService(gormDB, nil)
	service.now = func() time.Time { return now }
	principal := auth.Principal{UserID: actor.ID, Scope: constants.AuthScopePlatform}
	plan, err := service.CreatePlan(ctx, principal, CreatePlanRequest{Code: "test_" + uuid.NewString()[:8], Name: "Manual Lifecycle Test", Terms: PlanTermsInput{Currency: "INR", PriceMinor: 12500, BillingInterval: "MONTHLY", IntervalCount: 1, TrialDays: 1}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	plan, err = service.PublishPlan(ctx, principal, plan.Plan.ID)
	if err != nil || len(plan.Published) != 1 {
		t.Fatalf("publish plan = %#v, %v", plan, err)
	}

	key := "issue-" + uuid.NewString()
	issued, err := service.Issue(ctx, principal, cpo.ID, IssueRequest{PlanVersionID: plan.Published[0].ID, Reason: "Manual initial issue", IdempotencyKey: key})
	if err != nil || issued.Subscription.Status != "TRIAL" {
		t.Fatalf("issue = %#v, %v", issued, err)
	}
	replayed, err := service.Issue(ctx, principal, cpo.ID, IssueRequest{PlanVersionID: plan.Published[0].ID, Reason: "Manual initial issue", IdempotencyKey: key})
	if err != nil || replayed.Subscription.ID != issued.Subscription.ID {
		t.Fatalf("idempotent issue = %#v, %v", replayed, err)
	}

	service.now = func() time.Time { return now.AddDate(0, 0, 2) }
	stillTrial, err := service.GetCurrent(ctx, principal, cpo.ID)
	if err != nil || stillTrial.Subscription.Status != "TRIAL" {
		t.Fatalf("trial changed without a command = %#v, %v", stillTrial, err)
	}
	if _, err := service.Activate(ctx, principal, cpo.ID, TransitionRequest{Reason: "Manual trial approval", IdempotencyKey: "activate-" + uuid.NewString()}); err != nil {
		t.Fatalf("activate trial: %v", err)
	}
	if _, err := service.Renew(ctx, principal, cpo.ID, RenewRequest{Reason: "Manual renewal", IdempotencyKey: "renew-" + uuid.NewString()}); err != nil {
		t.Fatalf("renew: %v", err)
	}

	updated, err := service.UpdateDraft(ctx, principal, plan.Plan.ID, UpdateDraftRequest{Name: "Manual Lifecycle Test", Terms: PlanTermsInput{Currency: "INR", PriceMinor: 25000, BillingInterval: "MONTHLY", IntervalCount: 1}})
	if err != nil {
		t.Fatalf("create next draft: %v", err)
	}
	updated, err = service.PublishPlan(ctx, principal, updated.Plan.ID)
	if err != nil || len(updated.Published) < 2 {
		t.Fatalf("publish next version = %#v, %v", updated, err)
	}
	nextVersion := updated.Published[0]
	if nextVersion.Version != 2 {
		t.Fatalf("latest version = %d, want 2", nextVersion.Version)
	}
	changed, err := service.ChangePlan(ctx, principal, cpo.ID, ChangePlanRequest{PlanVersionID: nextVersion.ID, Reason: "Manual change", IdempotencyKey: "change-" + uuid.NewString()})
	if err != nil || changed.Subscription.PlanVersionID != nextVersion.ID || changed.Subscription.PendingPlanVersionID != nil {
		t.Fatalf("manual plan change = %#v, %v", changed, err)
	}

	if _, err := service.Pause(ctx, principal, cpo.ID, TransitionRequest{Reason: "Manual pause", IdempotencyKey: "pause-" + uuid.NewString()}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := service.Resume(ctx, principal, cpo.ID, TransitionRequest{Reason: "Manual resume", IdempotencyKey: "resume-" + uuid.NewString()}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, err := service.Cancel(ctx, principal, cpo.ID, TransitionRequest{Reason: "Manual cancellation", IdempotencyKey: "cancel-" + uuid.NewString()}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := service.GetCurrent(ctx, principal, cpo.ID); err == nil {
		t.Fatal("terminal subscription remained current")
	}
}
