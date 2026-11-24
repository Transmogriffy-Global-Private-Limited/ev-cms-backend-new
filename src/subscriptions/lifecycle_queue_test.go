package subscriptions

import (
	"testing"
	"time"
)

func TestLifecycleRetryDelayIsBounded(t *testing.T) {
	cases := []struct {
		attempts int64
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 15 * time.Minute},
		{40, 15 * time.Minute},
	}
	for _, tc := range cases {
		if actual := lifecycleRetryDelay(tc.attempts); actual != tc.want {
			t.Fatalf("attempt %d: got %s, want %s", tc.attempts, actual, tc.want)
		}
	}
	if lifecycleWorkLease <= 0 || lifecycleWorkLease > 5*time.Minute {
		t.Fatal("lease is not bounded")
	}
}
