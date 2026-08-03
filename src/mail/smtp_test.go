package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
)

func TestNewSMTPSenderAcceptsHostingerImplicitSSL(t *testing.T) {
	t.Parallel()

	sender, err := NewSMTPSender(config.Mail{
		Host:        "smtp.hostinger.com",
		Port:        465,
		Username:    "team@transev.in",
		Password:    "test-only-password",
		FromAddress: "team@transev.in",
		FromName:    "TransEV CMS",
		UseSSL:      true,
	})
	if err != nil {
		t.Fatalf("create Hostinger implicit-SSL sender: %v", err)
	}
	if sender == nil {
		t.Fatal("expected SMTP sender")
	}
}

func TestRenderPasswordResetIncludesRecipientRecoveryInputs(t *testing.T) {
	t.Parallel()

	payload := MessagePayload{
		Code:        "846201",
		ChallengeID: "5cef4c95-a1da-448e-bd7c-19d570cd4497",
		ExpiresAt:   time.Date(2026, time.August, 3, 12, 30, 0, 0, time.UTC),
	}
	for _, template := range []string{
		"PASSWORD_RESET_OTP",
		"CUSTOMER_PASSWORD_RESET_OTP",
	} {
		t.Run(template, func(t *testing.T) {
			t.Parallel()
			_, body, err := renderMessageContent(template, payload)
			if err != nil {
				t.Fatalf("render reset mail: %v", err)
			}
			for _, value := range []string{payload.Code, payload.ChallengeID, "03 Aug 2026 12:30 UTC"} {
				if !strings.Contains(body, value) {
					t.Fatalf("reset mail body %q does not contain %q", body, value)
				}
			}
		})
	}
}

func TestRenderNewCPOAdminWelcomeIncludesTemporaryPassword(t *testing.T) {
	t.Parallel()

	payload := MessagePayload{
		TemporaryPassword: "Temporary!Password123",
		CPOName:           "Example CPO",
		CPOID:             "c821a013-5041-42f7-80c8-aa153cf9d455",
		CPOAppID:          "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
	}
	_, body, err := renderMessageContent("CPO_ADMIN_WELCOME", payload)
	if err != nil {
		t.Fatalf("render CPO admin welcome: %v", err)
	}
	for _, value := range []string{
		payload.TemporaryPassword,
		payload.CPOName,
		payload.CPOID,
		payload.CPOAppID,
	} {
		if !strings.Contains(body, value) {
			t.Fatalf("welcome body %q does not contain %q", body, value)
		}
	}
}

func TestNewSMTPSenderRejectsAmbiguousTransport(t *testing.T) {
	t.Parallel()

	for _, cfg := range []config.Mail{
		{Host: "smtp.example.com", Port: 465},
		{Host: "smtp.example.com", Port: 465, UseTLS: true, UseSSL: true},
	} {
		if _, err := NewSMTPSender(cfg); err == nil ||
			!strings.Contains(err.Error(), "exactly one SMTP transport") {
			t.Fatalf("got error %v, want ambiguous transport rejection", err)
		}
	}
}
