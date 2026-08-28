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
		{
			name:     "new platform admin without temporary password",
			template: "PLATFORM_ADMIN_INVITE",
			payload:  MessagePayload{RecipientName: "Platform Admin"},
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

func TestValidateMessagePayloadAcceptsPlatformAdminGrantMail(t *testing.T) {
	t.Parallel()

	payload := MessagePayload{RecipientName: "Platform Admin", TemporaryPassword: "temporary-password"}
	for _, template := range []string{"PLATFORM_ADMIN_INVITE", "PLATFORM_ADMIN_GRANTED"} {
		if err := validateMessagePayload(template, payload); err != nil {
			t.Fatalf("complete %s payload must remain valid: %v", template, err)
		}
	}
}

func TestValidateMessagePayloadAcceptsCompletePasswordRecoveryMail(t *testing.T) {
	t.Parallel()

	payload := MessagePayload{
		RecipientName: "Recovery Recipient",
		ChallengeID:   "9c685277-7754-4a6b-a2c5-6d42329759ae",
		Code:          "123456",
		ExpiresAt:     time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC),
	}

	for _, template := range []string{"PASSWORD_RESET_OTP", "CUSTOMER_PASSWORD_RESET_OTP"} {
		if err := validateMessagePayload(template, payload); err != nil {
			t.Fatalf("complete %s payload must remain valid: %v", template, err)
		}
	}
}

func TestValidateSemanticCPOOnboardingRequiresActionURL(t *testing.T) {
	t.Parallel()
	payload := MessagePayload{RecipientName: "Operator", CPOName: "Example CPO", Role: "OPERATOR", TemporaryPassword: "temporary"}
	if err := validateMessagePayload("CPO_STAFF_NEW_IDENTITY", payload); err == nil || !strings.Contains(err.Error(), "action URL") {
		t.Fatalf("new staff without frontend action URL = %v", err)
	}
	payload.ActionURL = "https://cms.example/login#cpo_id=opaque"
	if err := validateMessagePayload("CPO_STAFF_NEW_IDENTITY", payload); err != nil {
		t.Fatalf("new staff with frontend action URL: %v", err)
	}
}
