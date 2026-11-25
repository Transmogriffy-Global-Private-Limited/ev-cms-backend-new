package subscriptions

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
)

// Requires an explicitly selected disposable TEST_DATABASE_URL; does not run
// against the developer's existing database by default.
func TestLifecycleQueueRecoversAfterWorkerCrashAndContinuousArrivals(t *testing.T) {
	database := repairTestDatabase(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, clock)
	cpo := repairTestCPO(t, database, clock)
	svc := NewService(database, nil)
	svc.now = func() time.Time { return clock }
	version := repairTestPlan(t, svc, principal)
	issued, err := svc.Issue(ctx, principal, cpo.ID, IssueRequest{
		PlanVersionID: version, Reason: "Externally confirmed", IdempotencyKey: "queue-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var target models.CPOSubscription
	if err := database.First(&target, "id = ?", issued.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	clock = target.CurrentPeriodEndsAt.Add(time.Second)
	first, err := svc.reserveLifecycleWork(ctx, clock)
	if err != nil {
		t.Fatal(err)
	}
	var oldClaim *uuid.UUID
	for _, job := range first {
		if job.SubscriptionID == target.ID {
			oldClaim = job.ClaimToken
		}
	}
	if oldClaim == nil {
		t.Fatal("target was not durably reserved before cursor advance")
	}
	// Simulate process termination: nobody processes or acknowledges first.
	// New subscriptions arrive beyond the scan cursor in an otherwise busy CMS.
	const newcomers = 525
	cpos := make([]models.CPO, 0, newcomers)
	subs := make([]models.CPOSubscription, 0, newcomers)
	for i := 0; i < newcomers; i++ {
		newCPO := models.CPO{ID: uuid.New(), Slug: "retry-" + uuid.NewString()[:8],
			BusinessName: fmt.Sprintf("Retry Arrival %d", i), CompanyType: constants.CPOCompanyTypeCompany,
			GSTIN: testsupport.ValidGSTIN("19"), Address: "Test Road", City: "Kolkata", State: "West Bengal",
			Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Fixture",
			StatusChangedAt: clock, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy,
			AppIDUpdatedAt: clock, CreatedAt: clock, UpdatedAt: clock}
		cpos = append(cpos, newCPO)
		subs = append(subs, models.CPOSubscription{ID: uuid.New(), CPOID: newCPO.ID, PlanVersionID: version,
			Status: "ACTIVE", StartsAt: clock.Add(-time.Hour), CurrentPeriodStartsAt: clock.Add(-time.Hour),
			CurrentPeriodEndsAt: clock.Add(time.Hour), CurrentPeriodSequence: 1,
			CreatedBy: principal.UserID, CreatedAt: clock, UpdatedAt: clock})
	}
	if err := database.CreateInBatches(cpos, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInBatches(subs, 100).Error; err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(lifecycleWorkLease + time.Second)
	if err := svc.processLifecycle(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&target, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if target.Status != "EXPIRED" {
		t.Fatalf("crashed work starved under continuous arrivals: status=%s", target.Status)
	}
	var remaining int64
	if err := database.Model(&lifecyclePendingWork{}).Where("subscription_id = ?", target.ID).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("processed target still queued: %d", remaining)
	}
}

func TestLifecycleQueueFailureRetriesAndFencesExpiredLease(t *testing.T) {
	database := repairTestDatabase(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, clock)
	cpo := repairTestCPO(t, database, clock)
	svc := NewService(database, nil)
	svc.now = func() time.Time { return clock }
	version := repairTestPlan(t, svc, principal)
	issued, err := svc.Issue(ctx, principal, cpo.ID, IssueRequest{PlanVersionID: version,
		Reason: "External payment", IdempotencyKey: "queue-fail-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	var target models.CPOSubscription
	if err := database.First(&target, "id = ?", issued.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	clock = target.CurrentPeriodEndsAt.Add(time.Second)
	// Inject a real, transaction-local-to-the-target failure instead of a
	// duplicate receipt: duplicate transition receipts are intentionally
	// idempotent and must NOT cause a worker error.
	faultName := "transev_test_expiry_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	faultTrigger := faultName + "_trigger"
	if err := database.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'injected lifecycle expiry failure';
END $$`, faultName)).Error; err != nil {
		t.Fatalf("create test expiry fault: %v", err)
	}
	if err := database.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE OF status ON cpo_subscriptions
FOR EACH ROW WHEN (OLD.id = '%s'::uuid AND NEW.status = 'EXPIRED') EXECUTE FUNCTION %s()`,
		faultTrigger, target.ID, faultName)).Error; err != nil {
		_ = database.Exec("DROP FUNCTION IF EXISTS " + faultName + "()").Error
		t.Fatalf("activate test expiry fault: %v", err)
	}
	removeFault := func() error {
		if err := database.Exec("DROP TRIGGER IF EXISTS " + faultTrigger + " ON cpo_subscriptions").Error; err != nil {
			return err
		}
		return database.Exec("DROP FUNCTION IF EXISTS " + faultName + "()").Error
	}
	t.Cleanup(func() {
		if err := removeFault(); err != nil {
			t.Errorf("cleanup injected expiry fault: %v", err)
		}
	})
	// Prove the trigger actually intercepts the target subscription, not
	// merely that the CREATE TRIGGER statement succeeded.
	if err := svc.processSubscriptionLifecycle(ctx, target.ID, clock); err == nil {
		t.Fatal("injected expiry trigger did not reject the target transition")
	}
	// This test exercises the retry queue (not scanner discovery). Ensure
	// this unique job is already due without affecting any other subscription.
	if err := database.Exec(`INSERT INTO subscription_lifecycle_pending_work
		(subscription_id, next_attempt_at, attempts, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)
		ON CONFLICT (subscription_id) DO UPDATE
		SET next_attempt_at = EXCLUDED.next_attempt_at,
		    claim_token = NULL,
		    updated_at = EXCLUDED.updated_at`,
		target.ID, clock.Add(-time.Hour), clock, clock).Error; err != nil {
		t.Fatalf("seed due retry test work: %v", err)
	}
	if err := svc.processLifecycle(ctx); err == nil {
		t.Fatal("expected injected expiry failure")
	}
	var queued lifecyclePendingWork
	if err := database.First(&queued, "subscription_id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queued.ClaimToken != nil {
		t.Fatal("failed work still carries exclusive claim")
	}
	if queued.Attempts < 1 || !queued.NextAttemptAt.After(clock) {
		t.Fatal("failed work has no bounded retry")
	}
	if err := removeFault(); err != nil {
		t.Fatal(err)
	}
	clock = queued.NextAttemptAt.Add(time.Second)
	if err := svc.processLifecycle(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&target, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if target.Status != "EXPIRED" {
		t.Fatalf("repaired obstruction not retried: %s", target.Status)
	}
}

func TestLifecycleQueueOldClaimCannotDeleteNewClaim(t *testing.T) {
	database := repairTestDatabase(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, clock)
	cpo := repairTestCPO(t, database, clock)
	svc := NewService(database, nil)
	svc.now = func() time.Time { return clock }
	version := repairTestPlan(t, svc, principal)
	issued, err := svc.Issue(ctx, principal, cpo.ID, IssueRequest{PlanVersionID: version,
		Reason: "External payment", IdempotencyKey: "queue-claim-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	var target models.CPOSubscription
	if err := database.First(&target, "id = ?", issued.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	clock = target.CurrentPeriodEndsAt.Add(time.Second)
	// This suite shares an isolated database among integration tests. Put
	// this case's work at the queue head so previous tests cannot mask the
	// claim-token fencing behavior via the bounded discovery cursor.
	if err := database.Exec(`INSERT INTO subscription_lifecycle_pending_work
		(subscription_id, next_attempt_at, attempts, created_at, updated_at)
		VALUES (?, ?, 0, ?, ?)
		ON CONFLICT (subscription_id) DO UPDATE
		SET next_attempt_at = EXCLUDED.next_attempt_at`,
		target.ID, time.Unix(0, 0).UTC(), clock, clock).Error; err != nil {
		t.Fatalf("seed due claim for fencing test: %v", err)
	}
	first, err := svc.reserveLifecycleWork(ctx, clock)
	if err != nil {
		t.Fatal(err)
	}
	var old lifecyclePendingWork
	for _, job := range first {
		if job.SubscriptionID == target.ID {
			old = job
		}
	}
	if old.ClaimToken == nil {
		t.Fatal("no first claim")
	}
	clock = clock.Add(lifecycleWorkLease + time.Second)
	second, err := svc.reserveLifecycleWork(ctx, clock)
	if err != nil {
		t.Fatal(err)
	}
	var newer lifecyclePendingWork
	for _, job := range second {
		if job.SubscriptionID == target.ID {
			newer = job
		}
	}
	if newer.ClaimToken == nil || *newer.ClaimToken == *old.ClaimToken {
		t.Fatal("no fresh fenced claim")
	}
	if err := svc.completeLifecycleWork(ctx, old); err != nil {
		t.Fatal(err)
	}
	var persisted lifecyclePendingWork
	if err := database.First(&persisted, "subscription_id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ClaimToken == nil || *persisted.ClaimToken != *newer.ClaimToken {
		t.Fatal("stale acknowledgement destroyed new claim")
	}
	if err := svc.completeLifecycleWork(ctx, newer); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Model(&lifecyclePendingWork{}).Where("subscription_id = ?", target.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("acknowledged queue row remains")
	}
}
