package mail

import (
	"strings"
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

func TestValidateMessagePayloadRejectsIncompleteCredentialMail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		payload  MessagePayload
		want     string
	}{
		{
			name:     "administrative reset without recovery ID",
			template: "PASSWORD_RESET_OTP",
			payload: MessagePayload{
				Code: "123456", ExpiresAt: time.Now().UTC().Add(time.Minute),
			},
			want: "recovery challenge ID is required",
		},
		{
			name:     "customer reset without recovery ID",
			template: "CUSTOMER_PASSWORD_RESET_OTP",
			payload: MessagePayload{
				Code: "123456", ExpiresAt: time.Now().UTC().Add(time.Minute),
			},
			want: "recovery challenge ID is required",
		},
		{
			name:     "new CPO admin without temporary password",
			template: "CPO_ADMIN_WELCOME",
			payload:  MessagePayload{CPOID: "cpo-id", CPOAppID: "app-id"},
			want:     "temporary password is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateMessagePayload(test.template, test.payload)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}
