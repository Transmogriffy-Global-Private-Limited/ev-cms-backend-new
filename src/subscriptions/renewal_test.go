package subscriptions

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func TestRenewSubscriptionPeriodReactivatesExpiredRecordAtCurrentTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	ended := now.Add(-time.Hour)
	trialEnd := now.Add(-24 * time.Hour)
	record := models.CPOSubscription{Status: "EXPIRED", EndedAt: &ended, TrialEndsAt: &trialEnd}
	version := models.SubscriptionPlanVersion{BillingInterval: "MONTHLY", IntervalCount: 1}

	renewSubscriptionPeriod(&record, now.Add(-48*time.Hour), now, version)

	if record.Status != "ACTIVE" || record.EndedAt != nil || record.TrialEndsAt != nil {
		t.Fatalf("expired renewal did not reactivate terminal state: %+v", record)
	}
	if !record.CurrentPeriodStartsAt.Equal(now) || !record.CurrentPeriodEndsAt.Equal(now.AddDate(0, 1, 0)) {
		t.Fatalf("expired renewal period=%s..%s, want %s..%s", record.CurrentPeriodStartsAt, record.CurrentPeriodEndsAt, now, now.AddDate(0, 1, 0))
	}
}
