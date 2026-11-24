package subscriptions

import (
	"context"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLegacyLifecycleWindowBoundaries(t *testing.T) {
	start := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	subscription := models.CPOSubscription{CurrentPeriodStartsAt: start, CurrentPeriodEndsAt: end}
	for _, kind := range []string{"EXPIRY_WARNING_7D", "EXPIRY_WARNING_3D", "EXPIRY_WARNING_1D"} {
		lower, upper := legacyLifecycleWindow(subscription, kind)
		if !lower.Equal(start) || upper == nil || !upper.Equal(end) {
			t.Fatalf("warning %s window = %v..%v, want %v..%v", kind, lower, upper, start, end)
		}
	}
	lower, upper := legacyLifecycleWindow(subscription, "EXPIRED")
	if !lower.Equal(end) || upper != nil {
		t.Fatalf("expired window = %v..%v, want from %v without upper bound", lower, upper, end)
	}
}

func TestLegacyLifecycleUpgradeSuppressesSamePeriodWarningAndPreservesRenewals(t *testing.T) {
	database := repairTestDatabase(t)
	ctx := context.Background()
	clock := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, clock)
	cpo := repairTestCPO(t, database, clock)
	service := NewService(database, nil)
	service.now = func() time.Time { return clock }
	planVersion := repairTestPlan(t, service, principal)
	issued, err := service.Issue(ctx, principal, cpo.ID, IssueRequest{
		PlanVersionID: planVersion, Reason: "Initial manually confirmed subscription", IdempotencyKey: "legacy-upgrade-issue-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var sub models.CPOSubscription
	if err := database.First(&sub, "id = ?", issued.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sub.CurrentPeriodSequence != 1 {
		t.Fatalf("initial period identity %d, expected 1", sub.CurrentPeriodSequence)
	}

	warningAt := sub.CurrentPeriodEndsAt.Add(-6 * 24 * time.Hour)
	legacy := models.CPOSubscriptionLifecycleEvent{
		ID: uuid.New(), SubscriptionID: sub.ID, Kind: "EXPIRY_WARNING_7D", PeriodSequence: nil,
		EffectiveAt: warningAt, CreatedAt: warningAt,
	}
	if err := database.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return service.recordLifecycleEvent(tx, sub, legacy.Kind, warningAt.Add(time.Hour), nil)
	}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Model(&models.CPOSubscriptionLifecycleEvent{}).
		Where("subscription_id = ? AND kind = ?", sub.ID, legacy.Kind).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate legacy same-period warning: got %d events, want 1", count)
	}

	// After an upgrade, historical old-period warnings can coexist with the
	// current period (the migration does not invent an identity for them).
	// A legacy warning emitted BEFORE the current start must not suppress the
	// current period's newly owed warning even when sequence starts at 1.
	olderKind := "EXPIRY_WARNING_3D"
	legacyPrior := models.CPOSubscriptionLifecycleEvent{
		ID: uuid.New(), SubscriptionID: sub.ID, Kind: olderKind,
		EffectiveAt: sub.CurrentPeriodStartsAt.Add(-time.Hour),
		CreatedAt:   sub.CurrentPeriodStartsAt.Add(-time.Hour),
	}
	if err := database.Create(&legacyPrior).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return service.recordLifecycleEvent(tx, sub, olderKind, sub.CurrentPeriodEndsAt.Add(-2*time.Hour), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.CPOSubscriptionLifecycleEvent{}).
		Where("subscription_id = ? AND kind = ?", sub.ID, olderKind).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("legacy prior-period warning suppressed current warning; got %d events, want 2", count)
	}

	// A previous-period legacy warning must not suppress a new period's event.
	clock = sub.CurrentPeriodEndsAt.Add(time.Hour)
	if _, err := service.Renew(ctx, principal, cpo.ID, RenewRequest{
		Reason: "Externally confirmed renewal", IdempotencyKey: "legacy-upgrade-renew-" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&sub, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sub.CurrentPeriodSequence != 2 {
		t.Fatalf("renewal sequence = %d, expected 2", sub.CurrentPeriodSequence)
	}
	nextWarningAt := sub.CurrentPeriodEndsAt.Add(-6 * 24 * time.Hour)
	if err := database.Transaction(func(tx *gorm.DB) error {
		return service.recordLifecycleEvent(tx, sub, legacy.Kind, nextWarningAt, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.CPOSubscriptionLifecycleEvent{}).
		Where("subscription_id = ? AND kind = ?", sub.ID, legacy.Kind).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("renewal warning suppressed or duplicated: got %d events, want 2", count)
	}
}

func TestLegacyLifecycleExpiredEventReconcilesActiveStateWithoutSecondNotification(t *testing.T) {
	database := repairTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	principal := repairTestActor(t, database, now)
	cpo := repairTestCPO(t, database, now)
	service := NewService(database, nil)
	service.now = func() time.Time { return now }
	version := repairTestPlan(t, service, principal)
	issued, err := service.Issue(ctx, principal, cpo.ID, IssueRequest{
		PlanVersionID: version, Reason: "Initial externally confirmed issue", IdempotencyKey: "legacy-expire-issue-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var sub models.CPOSubscription
	if err := database.First(&sub, "id = ?", issued.Subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	expiryAt := sub.CurrentPeriodEndsAt.Add(time.Second)
	if err := database.Create(&models.CPOSubscriptionLifecycleEvent{
		ID: uuid.New(), SubscriptionID: sub.ID, Kind: "EXPIRED", PeriodSequence: nil,
		EffectiveAt: expiryAt, CreatedAt: expiryAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.processSubscriptionLifecycle(ctx, sub.ID, expiryAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&sub, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sub.Status != "EXPIRED" {
		t.Fatalf("legacy expiry left status %q", sub.Status)
	}
	var count int64
	if err := database.Model(&models.CPOSubscriptionLifecycleEvent{}).
		Where("subscription_id = ? AND kind = ?", sub.ID, "EXPIRED").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate EXPIRED notification/event: %d", count)
	}
	if err := database.Model(&models.CPOSubscriptionHistory{}).
		Where("subscription_id = ? AND actor_user_id IS NULL", sub.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("system expiry history count = %d, want 1", count)
	}
}
