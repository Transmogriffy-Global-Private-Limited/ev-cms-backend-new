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

func TestRenderSemanticResetKeepsChallengeOnlyInActionURL(t *testing.T) {
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
			_, body, html, err := renderSemanticMessage(template, payload, config.Mail{DisplayLocation: time.FixedZone("IST", 5*60*60+30*60), Frontend: config.FrontendLinks{AdminPasswordResetTemplate: "https://cms.example/reset#challenge_id={challenge_id}", CustomerPasswordResetTemplate: "https://app.example/reset#challenge_id={challenge_id}"}})
			if err != nil {
				t.Fatalf("render reset mail: %v", err)
			}
			if !strings.Contains(body, payload.Code) || !strings.Contains(body, "03 Aug 2026, 6:00 PM IST") {
				t.Fatalf("reset mail does not contain code/local expiry: %q", body)
			}
			visibleBody := strings.ReplaceAll(body, "challenge_id="+payload.ChallengeID, "challenge_id=<opaque>")
			if strings.Contains(visibleBody, "Recovery ID:") || strings.Contains(visibleBody, "Challenge ID:") || strings.Contains(visibleBody, payload.ChallengeID) {
				t.Fatalf("reset mail visibly exposed challenge context: %q", body)
			}
			if !strings.Contains(body, "challenge_id="+payload.ChallengeID) || strings.Contains(body, payload.Code+"&") {
				t.Fatalf("reset action URL does not carry only challenge context: %q", body)
			}
			if !strings.Contains(html, "href=") {
				t.Fatalf("semantic reset HTML lacks action link: %q", html)
			}
		})
	}
}

func TestRenderSemanticCPOStaffProcedures(t *testing.T) {
	t.Parallel()
	cfg := config.Mail{DisplayLocation: time.UTC}
	actionURL := "https://cms.example/login#cpo_id=opaque-cpo"
	newIdentity := MessagePayload{RecipientName: "<Alex>", CPOName: "Example <CPO>", Role: "OPERATOR", TemporaryPassword: "Temporary!Password", ActionURL: actionURL}
	_, newText, newHTML, err := renderSemanticMessage("CPO_STAFF_NEW_IDENTITY", newIdentity, cfg)
	if err != nil || !strings.Contains(newText, "Operator") || !strings.Contains(newText, actionURL) || !strings.Contains(newHTML, "&lt;Alex&gt;") || strings.Contains(newHTML, "<Alex>") {
		t.Fatalf("new identity semantic rendering = text %q html %q err %v", newText, newHTML, err)
	}
	existing := MessagePayload{RecipientName: "Viewer", CPOName: "Example CPO", Role: "VIEWER", ActionURL: actionURL}
	_, existingText, _, err := renderSemanticMessage("CPO_STAFF_EXISTING_IDENTITY", existing, cfg)
	if err != nil || !strings.Contains(existingText, "Viewer") || strings.Contains(existingText, "Temporary password") {
		t.Fatalf("existing identity procedure = %q, %v", existingText, err)
	}
	_, resendText, _, err := renderSemanticMessage("CPO_ONBOARDING_RESENT", MessagePayload{CPOName: "Example CPO", ActionURL: actionURL}, cfg)
	if err != nil || strings.Contains(resendText, "Temporary password") || !strings.Contains(resendText, "never resent") {
		t.Fatalf("resend procedure = %q, %v", resendText, err)
	}
	for _, template := range []string{"CPO_STAFF_SUSPENDED", "CPO_STAFF_REVOKED"} {
		_, text, _, renderErr := renderSemanticMessage(template, MessagePayload{CPOName: "Example CPO", ActionURL: actionURL}, cfg)
		if renderErr != nil || !strings.Contains(text, "only this CPO membership") || strings.Contains(strings.ToLower(text), "account has been") {
			t.Fatalf("%s procedure = %q, %v", template, text, renderErr)
		}
	}
}

func TestEverySemanticTemplateHasSubjectTextAndHTML(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 28, 10, 45, 0, 0, time.UTC)
	cfg := config.Mail{DisplayLocation: time.UTC, Frontend: config.FrontendLinks{
		AdminLoginVerifyTemplate:      "https://cms.example/login#challenge_id={challenge_id}",
		AdminPasswordResetTemplate:    "https://cms.example/reset#challenge_id={challenge_id}",
		CustomerLoginVerifyTemplate:   "https://app.example/login#challenge_id={challenge_id}",
		CustomerSignupVerifyTemplate:  "https://app.example/signup#challenge_id={challenge_id}",
		CustomerPasswordResetTemplate: "https://app.example/reset#challenge_id={challenge_id}",
		CPOSubscriptionURL:            "https://cms.example/subscription",
	}}
	base := MessagePayload{RecipientName: "Recipient", Code: "123456", ChallengeID: "5cef4c95-a1da-448e-bd7c-19d570cd4497", ExpiresAt: now, OccurredAt: now, TemporaryPassword: "temporary", CPOName: "Example CPO", Role: "OPERATOR", ActionURL: "https://cms.example/login#cpo_id=opaque", SupportSubject: "Ticket", SupportStatus: "OPEN"}
	for template := range semanticSubjects {
		t.Run(template, func(t *testing.T) {
			subject, text, html, err := renderSemanticMessage(template, base, cfg)
			if err != nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(text) == "" || strings.TrimSpace(html) == "" {
				t.Fatalf("semantic template %s = subject %q text %q html %q err %v", template, subject, text, html, err)
			}
		})
	}
}

func TestDurableTemplateCatalogueMatchesCurrentRenderers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 28, 10, 45, 0, 0, time.UTC)
	payload := MessagePayload{
		RecipientName:     "Recipient",
		Code:              "123456",
		ChallengeID:       "5cef4c95-a1da-448e-bd7c-19d570cd4497",
		ExpiresAt:         now,
		OccurredAt:        now,
		TemporaryPassword: "temporary-password",
		CPOName:           "Example CPO",
		CPOID:             "opaque-cpo",
		CPOAppID:          "opaque-app",
		Role:              "OPERATOR",
		ActionURL:         "https://cms.example.invalid/login#context=opaque",
		SupportSubject:    "Connector status",
		SupportStatus:     "OPEN",
	}
	cfg := config.Mail{DisplayLocation: time.UTC, Frontend: config.FrontendLinks{
		AdminLoginVerifyTemplate:      "https://cms.example.invalid/login#challenge_id={challenge_id}",
		AdminPasswordResetTemplate:    "https://cms.example.invalid/reset#challenge_id={challenge_id}",
		CustomerLoginVerifyTemplate:   "https://app.example.invalid/login#challenge_id={challenge_id}",
		CustomerSignupVerifyTemplate:  "https://app.example.invalid/signup#challenge_id={challenge_id}",
		CustomerPasswordResetTemplate: "https://app.example.invalid/reset#challenge_id={challenge_id}",
		CPOSubscriptionURL:            "https://cms.example.invalid/subscription",
	}}

	for _, template := range SupportedDurableTemplateNames() {
		t.Run(template, func(t *testing.T) {
			var subject, text, html string
			var err error
			if isLegacyTemplate(template) {
				subject, text, err = renderMessageContent(template, payload)
				if err == nil {
					text, html, err = renderMessageTemplates(payload.RecipientName, text)
				}
			} else {
				subject, text, html, err = renderSemanticMessage(template, payload, cfg)
			}
			if err != nil || strings.TrimSpace(subject) == "" || strings.TrimSpace(text) == "" || strings.TrimSpace(html) == "" {
				t.Fatalf("render durable template = subject %q text %q html %q err %v", subject, text, html, err)
			}
		})
	}

	for template := range semanticSubjects {
		if !isSupportedDurableTemplate(template) {
			t.Fatalf("semantic renderer template %q is missing from durable catalogue", template)
		}
	}
	for template := range legacyTemplateNames {
		if !isSupportedDurableTemplate(template) {
			t.Fatalf("legacy renderer template %q is missing from durable catalogue", template)
		}
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

func TestRenderSupportTicketNotificationsUseLocalizedPrivacySafeContent(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("load display location: %v", err)
	}
	payload := MessagePayload{
		RecipientName:  "CPO Admin",
		CPOName:        "Example CPO",
		SupportSubject: "Connector <status> review",
		SupportStatus:  "RESOLVED",
		OccurredAt:     time.Date(2026, time.August, 28, 10, 45, 0, 0, time.UTC),
		ActionURL:      "https://cms.example.invalid/support/tickets/opaque-ticket-context",
	}
	for _, template := range []string{
		"CPO_SUPPORT_TICKET_CREATED",
		"CPO_SUPPORT_TICKET_PLATFORM_REPLY",
		"CPO_SUPPORT_TICKET_RESOLVED",
		"CPO_SUPPORT_TICKET_CLOSED",
		"CPO_SUPPORT_TICKET_REOPENED",
	} {
		t.Run(template, func(t *testing.T) {
			t.Parallel()
			subject, text, html, renderErr := renderSemanticMessage(template, payload, config.Mail{DisplayLocation: location})
			if renderErr != nil {
				t.Fatalf("render %s: %v", template, renderErr)
			}
			if strings.TrimSpace(subject) == "" || !strings.Contains(text, "28 Aug 2026, 4:15 PM IST") {
				t.Fatalf("localized support message = subject %q text %q", subject, text)
			}
			if !strings.Contains(text, payload.ActionURL) || strings.Contains(text, "private reply body") {
				t.Fatalf("support text did not retain the safe action-only procedure: %q", text)
			}
			if !strings.Contains(html, "Connector &lt;status&gt; review") || strings.Contains(html, "<status>") {
				t.Fatalf("support HTML did not escape ticket subject: %q", html)
			}
		})
	}
}
