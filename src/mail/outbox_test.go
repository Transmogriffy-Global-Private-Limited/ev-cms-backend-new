package mail

import (
	"testing"
	"time"
)

func TestRetryDelayIsBounded(t *testing.T) {
	t.Parallel()

	if got := retryDelay(1); got != time.Minute {
		t.Fatalf("attempt 1 delay = %s, want 1m", got)
	}
	if got := retryDelay(4); got != 8*time.Minute {
		t.Fatalf("attempt 4 delay = %s, want 8m", got)
	}
	if got := retryDelay(20); got != time.Hour {
		t.Fatalf("attempt 20 delay = %s, want 1h", got)
	}
}
