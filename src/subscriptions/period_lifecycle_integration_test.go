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
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
)

func TestRepeatedSubscriptionPeriodExpiryWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	clock := time.Now().UTC().Truncate(time.Microsecond)
	actor := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", PasswordHash: "integration-test-only",
		FullName: "Period Identity Tester", IsActive: true, IsVerified: true,
		PasswordChangedAt: clock, CreatedAt: clock, UpdatedAt: clock}
	if err := database.Create(&actor).Error; err != nil {
		t.Fatal(err)
	}
	cpo := models.CPO{ID: uuid.New(), Slug: "test-" + uuid.NewString()[:8],
		BusinessName: "Period Test CPO", CompanyType: constants.CPOCompanyTypeCompany,
		GSTIN: testsupport.ValidGSTIN("19"), Address: "1 Test Road", City: "Kolkata", State: "West Bengal",
		Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Test",
		StatusChangedAt: clock, AppID: "cpo_dummy_" + uuid.NewString(),
		AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: clock,
		CreatedAt: clock, UpdatedAt: clock}
	if err := database.Create(&cpo).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(database, nil)
	service.now = func() time.Time { return clock }
	principal := auth.Principal{UserID: actor.ID, Scope: constants.AuthScopePlatform}
	plan, err := service.CreatePlan(ctx, principal, CreatePlanRequest{Code: "period_" + uuid.NewString()[:8], Name: "Period Test",
		Terms: PlanTermsInput{Currency: "INR", PriceMinor: 1200, BillingInterval: "MONTHLY", IntervalCount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = service.PublishPlan(ctx, principal, plan.Plan.ID)
	if err != nil || len(plan.Published) != 1 {
		t.Fatalf("publish: %v, %+v", err, plan)
	}
	issued, err := service.Issue(ctx, principal, cpo.ID, IssueRequest{PlanVersionID: plan.Published[0].ID,
		Reason: "Initial manual subscription", IdempotencyKey: "initial-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Subscription.CurrentPeriodSequence != 1 {
		t.Fatalf("initial sequence %d, want 1", issued.Subscription.CurrentPeriodSequence)
	}
	for period := int64(1); period <= 3; period++ {
		var current models.CPOSubscription
		if err := database.First(&current, "id = ?", issued.Subscription.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.CurrentPeriodSequence != period {
			t.Fatalf("period %d has sequence %d", period, current.CurrentPeriodSequence)
		}
		clock = current.CurrentPeriodEndsAt.Add(time.Second)
		if err := service.processSubscriptionLifecycle(ctx, current.ID, clock); err != nil {
			t.Fatalf("expire period %d: %v", period, err)
		}
		if err := service.processSubscriptionLifecycle(ctx, current.ID, clock); err != nil {
			t.Fatalf("repeat expiry %d: %v", period, err)
		}
		if err := database.First(&current, "id = ?", current.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.Status != "EXPIRED" {
			t.Fatalf("period %d status %s, want EXPIRED", period, current.Status)
		}
		if period < 3 {
			if _, err := service.Renew(ctx, principal, cpo.ID, RenewRequest{
				Reason: "Externally confirmed renewal", IdempotencyKey: "renew-" + uuid.NewString()}); err != nil {
				t.Fatalf("renew after period %d: %v", period, err)
			}
		}
	}
	var events []models.CPOSubscriptionLifecycleEvent
	if err := database.Where("subscription_id = ? AND kind = ?", issued.Subscription.ID, "EXPIRED").
		Order("period_sequence ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d expiry events, want 3", len(events))
	}
	for i, e := range events {
		if e.PeriodSequence == nil || *e.PeriodSequence != int64(i+1) {
			t.Fatalf("expiry event %d belongs to wrong period: %+v", i, e)
		}
	}
	var systemHistory int64
	if err := database.Model(&models.CPOSubscriptionHistory{}).
		Where("subscription_id = ? AND actor_user_id IS NULL", issued.Subscription.ID).
		Count(&systemHistory).Error; err != nil {
		t.Fatal(err)
	}
	if systemHistory != 3 {
		t.Fatalf("got %d system expiry history records, want 3", systemHistory)
	}
}
