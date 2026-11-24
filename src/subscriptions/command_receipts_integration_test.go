package subscriptions

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func repairTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires explicitly selected disposable TEST_DATABASE_URL")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	return database
}

func repairTestActor(t *testing.T, database *gorm.DB, now time.Time) auth.Principal {
	t.Helper()
	actor := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", PasswordHash: "test-only", FullName: "Repair fixture", IsActive: true, IsVerified: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&actor).Error; err != nil {
		t.Fatal(err)
	}
	return auth.Principal{UserID: actor.ID, Scope: constants.AuthScopePlatform}
}

func repairTestCPO(t *testing.T, database *gorm.DB, now time.Time) models.CPO {
	t.Helper()
	cpo := models.CPO{ID: uuid.New(), Slug: "repair-" + uuid.NewString()[:8], BusinessName: "Repair CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: testsupport.ValidGSTIN("19"), Address: "Test Road", City: "Kolkata", State: "West Bengal", Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Fixture", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&cpo).Error; err != nil {
		t.Fatal(err)
	}
	return cpo
}

func repairTestPlan(t *testing.T, service *Service, principal auth.Principal) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	plan, err := service.CreatePlan(ctx, principal, CreatePlanRequest{Code: "repair_" + uuid.NewString()[:8], Name: "Repair plan", Terms: PlanTermsInput{Currency: "INR", PriceMinor: 1500, BillingInterval: "MONTHLY", IntervalCount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = service.PublishPlan(ctx, principal, plan.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Published[0].ID
}

func TestCommandReceiptFrozenReplayWithPostgreSQL(t *testing.T) {
	database := repairTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	cpo := repairTestCPO(t, database, now)
	service := NewService(database, nil)
	service.now = func() time.Time { return now }
	version := repairTestPlan(t, service, principal)
	original := IssueRequest{PlanVersionID: version, Reason: "Externally confirmed", IdempotencyKey: "issue-" + uuid.NewString()}
	first, err := service.Issue(context.Background(), principal, cpo.ID, original)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.AddDate(0, 0, 2) }
	if _, err := service.Renew(context.Background(), principal, cpo.ID, RenewRequest{Reason: "Second period", IdempotencyKey: "renew-" + uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	replay, err := service.Issue(context.Background(), principal, cpo.ID, original)
	if err != nil {
		t.Fatalf("same command replay: %v", err)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Fatal("replay result changed after renewal")
	}
	changed := original
	changed.Reason = "Different external record"
	if _, err := service.Issue(context.Background(), principal, cpo.ID, changed); err == nil {
		t.Fatal("key reused with different request was accepted")
	}
	view, err := service.Overview(context.Background(), principal, cpo.ID)
	if err != nil || view.State != "CURRENT" || view.Latest == nil {
		t.Fatalf("overview: %+v, %v", view, err)
	}
	if view.Latest.Subscription.CurrentPeriodSequence <= first.Subscription.CurrentPeriodSequence {
		t.Fatal("renewal did not advance period")
	}
}

func TestConcurrentSameKeyIssueReturnsOneFrozenResult(t *testing.T) {
	database := repairTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	service := NewService(database, nil)
	service.now = func() time.Time { return now }
	cpo := repairTestCPO(t, database, now)
	version := repairTestPlan(t, service, principal)
	request := IssueRequest{PlanVersionID: version, Reason: "Manual external collection confirmed", IdempotencyKey: "parallel-" + uuid.NewString()}
	start := make(chan struct{})
	type result struct {
		view SubscriptionView
		err  error
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			v, e := service.Issue(context.Background(), principal, cpo.ID, request)
			results <- result{v, e}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var first *SubscriptionView
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent issue: %v", r.err)
		}
		if first == nil {
			x := r.view
			first = &x
		} else if !reflect.DeepEqual(*first, r.view) {
			t.Fatal("concurrent retry returned differing results")
		}
	}
	var subscriptions int64
	if err := database.Model(&models.CPOSubscription{}).Where("cpo_id = ?", cpo.ID).Count(&subscriptions).Error; err != nil {
		t.Fatal(err)
	}
	if subscriptions != 1 {
		t.Fatalf("issue produced %d subscription rows", subscriptions)
	}
}

func TestLegacyCommandReceiptDoesNotFakeOriginalResult(t *testing.T) {
	database := repairTestDatabase(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	cpo := repairTestCPO(t, database, now)
	service := NewService(database, nil)
	service.now = func() time.Time { return now }
	version := repairTestPlan(t, service, principal)
	request := IssueRequest{PlanVersionID: version, Reason: "Legacy issue", IdempotencyKey: "legacy-" + uuid.NewString()}
	result, err := service.Issue(context.Background(), principal, cpo.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.CPOSubscriptionHistory{}).Where("actor_user_id = ? AND idempotency_key = ?", principal.UserID, request.IdempotencyKey).Update("metadata", models.JSONB{"operation": "ISSUE"}).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.Issue(context.Background(), principal, cpo.ID, request)
	if err == nil {
		t.Fatal("legacy replay incorrectly inferred mutable current state")
	}
	var count int64
	if err := database.Model(&models.CPOSubscription{}).Where("id = ?", result.Subscription.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("legacy retry mutated subscription")
	}
}
