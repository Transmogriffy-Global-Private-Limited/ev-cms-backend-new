package customerauth

import (
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func TestSubscriptionBlocksNewCustomerCommandsAtExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		status string
		end    time.Time
		want   bool
	}{
		{name: "active before period end", status: "ACTIVE", end: now.Add(time.Second), want: false},
		{name: "active at period end", status: "ACTIVE", end: now, want: true},
		{name: "past-due after period end", status: "PAST_DUE", end: now.Add(-time.Second), want: true},
		{name: "expired record", status: "EXPIRED", end: now.Add(time.Hour), want: true},
		{name: "cancelled record is not the expiry gate", status: "CANCELLED", end: now.Add(-time.Hour), want: false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := subscriptionBlocksCustomerCommands(models.CPOSubscription{Status: test.status, CurrentPeriodEndsAt: test.end}, now)
			if got != test.want {
				t.Fatalf("subscriptionBlocksCustomerCommands(%s, %s) = %v, want %v", test.status, test.end, got, test.want)
			}
		})
	}
}

func TestCurrentRenewalTakesPrecedenceOverExpiredSubscriptionHistory(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	expired := models.CPOSubscription{Status: "EXPIRED", CurrentPeriodEndsAt: now.Add(-24 * time.Hour)}
	renewed := models.CPOSubscription{Status: "ACTIVE", CurrentPeriodEndsAt: now.Add(30 * 24 * time.Hour)}

	if customerSubscriptionAdmissionBlocked(&renewed, &expired, now) {
		t.Fatal("a current active renewal must admit customer commands despite an expired historical row")
	}
	if !customerSubscriptionAdmissionBlocked(nil, &expired, now) {
		t.Fatal("an expired subscription must still block when no current subscription exists")
	}
}
