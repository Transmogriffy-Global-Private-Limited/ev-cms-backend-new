package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/message.html.tmpl templates/message.txt.tmpl templates/semantic.html.tmpl templates/semantic.txt.tmpl
var messageTemplates embed.FS

type renderedMessage struct {
	RecipientName string
	Body          string
}

type semanticMessage struct {
	RecipientName     string
	Code              string
	ExpiresAt         string
	OccurredAt        string
	TemporaryPassword string
	CPOName           string
	Role              string
	ActionURL         string
	SupportSubject    string
	SupportStatus     string
}

// durableTemplateCatalog is the single application source for template names
// that may be persisted in mail_outbox. The database CHECK in migration 000058
// deliberately carries the same catalogue for the durable boundary.
var durableTemplateCatalog = []string{
	"LOGIN_OTP",
	"PASSWORD_RESET_OTP",
	"CUSTOMER_LOGIN_OTP",
	"CUSTOMER_SIGNUP_OTP",
	"CUSTOMER_PASSWORD_RESET_OTP",
	"CPO_ADMIN_WELCOME",
	"CPO_MEMBERSHIP_ASSIGNED",
	"PASSWORD_CHANGE_REMINDER",
	"PLATFORM_ADMIN_INVITE",
	"PLATFORM_ADMIN_GRANTED",
	"CPO_STAFF_NEW_IDENTITY",
	"CPO_STAFF_EXISTING_IDENTITY",
	"CPO_ONBOARDING_RESENT",
	"CPO_STAFF_ROLE_CHANGED",
	"CPO_STAFF_SUSPENDED",
	"CPO_STAFF_REACTIVATED",
	"CPO_STAFF_REVOKED",
	"CPO_SUBSCRIPTION_EXPIRY_WARNING",
	"CPO_SUBSCRIPTION_EXPIRED",
	"CPO_SUPPORT_TICKET_CREATED",
	"CPO_SUPPORT_TICKET_PLATFORM_REPLY",
	"CPO_SUPPORT_TICKET_RESOLVED",
	"CPO_SUPPORT_TICKET_CLOSED",
	"CPO_SUPPORT_TICKET_REOPENED",
}

var legacyTemplateNames = map[string]struct{}{
	"CPO_ADMIN_WELCOME":        {},
	"CPO_MEMBERSHIP_ASSIGNED":  {},
	"PASSWORD_CHANGE_REMINDER": {},
	"PLATFORM_ADMIN_INVITE":    {},
	"PLATFORM_ADMIN_GRANTED":   {},
}

var durableTemplateNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(durableTemplateCatalog))
	for _, name := range durableTemplateCatalog {
		names[name] = struct{}{}
	}
	return names
}()

// SupportedDurableTemplateNames returns a copy so callers cannot mutate the
// authoritative application catalogue.
func SupportedDurableTemplateNames() []string {
	names := make([]string, len(durableTemplateCatalog))
	copy(names, durableTemplateCatalog)
	return names
}

func isSupportedDurableTemplate(templateName string) bool {
	_, ok := durableTemplateNames[templateName]
	return ok
}

var semanticSubjects = map[string]string{
	"LOGIN_OTP": "Your TransEV CMS sign-in code", "PASSWORD_RESET_OTP": "Reset your TransEV CMS password",
	"CUSTOMER_LOGIN_OTP": "Your charging app sign-in code", "CUSTOMER_SIGNUP_OTP": "Verify your charging account", "CUSTOMER_PASSWORD_RESET_OTP": "Reset your charging account password",
	"CPO_STAFF_NEW_IDENTITY": "Your CPO access", "CPO_STAFF_EXISTING_IDENTITY": "You have CPO access", "CPO_ONBOARDING_RESENT": "Your CPO access reminder",
	"CPO_STAFF_ROLE_CHANGED": "Your CPO role has changed", "CPO_STAFF_SUSPENDED": "Your CPO access has been suspended", "CPO_STAFF_REACTIVATED": "Your CPO access has been restored", "CPO_STAFF_REVOKED": "Your CPO access has been removed",
	"CPO_SUBSCRIPTION_EXPIRY_WARNING": "Your CPO subscription is ending soon", "CPO_SUBSCRIPTION_EXPIRED": "Your CPO subscription has expired",
	"CPO_SUPPORT_TICKET_CREATED": "Support ticket received", "CPO_SUPPORT_TICKET_PLATFORM_REPLY": "Support replied to your ticket", "CPO_SUPPORT_TICKET_RESOLVED": "Support ticket resolved", "CPO_SUPPORT_TICKET_CLOSED": "Support ticket closed", "CPO_SUPPORT_TICKET_REOPENED": "Support ticket reopened",
}

func renderSemanticTemplates(templateName string, data semanticMessage) (string, string, string, error) {
	subject, ok := semanticSubjects[templateName]
	if !ok {
		return "", "", "", fmt.Errorf("render semantic mail template: unknown template %q", templateName)
	}
	text, err := texttemplate.ParseFS(messageTemplates, "templates/semantic.txt.tmpl")
	if err != nil {
		return "", "", "", fmt.Errorf("parse semantic text mail template: %w", err)
	}
	html, err := template.ParseFS(messageTemplates, "templates/semantic.html.tmpl")
	if err != nil {
		return "", "", "", fmt.Errorf("parse semantic HTML mail template: %w", err)
	}
	var textBody, htmlBody bytes.Buffer
	if err := text.ExecuteTemplate(&textBody, templateName, data); err != nil {
		return "", "", "", fmt.Errorf("render semantic text mail template: %w", err)
	}
	if err := html.ExecuteTemplate(&htmlBody, templateName, data); err != nil {
		return "", "", "", fmt.Errorf("render semantic HTML mail template: %w", err)
	}
	return subject, textBody.String(), htmlBody.String(), nil
}

func renderMessageTemplates(recipientName, body string) (string, string, error) {
	if strings.TrimSpace(recipientName) == "" {
		recipientName = "there"
	}
	data := renderedMessage{RecipientName: recipientName, Body: body}
	text, err := texttemplate.ParseFS(messageTemplates, "templates/message.txt.tmpl")
	if err != nil {
		return "", "", fmt.Errorf("parse text mail template: %w", err)
	}
	html, err := template.ParseFS(messageTemplates, "templates/message.html.tmpl")
	if err != nil {
		return "", "", fmt.Errorf("parse HTML mail template: %w", err)
	}
	var textBody, htmlBody bytes.Buffer
	if err := text.Execute(&textBody, data); err != nil {
		return "", "", fmt.Errorf("render text mail template: %w", err)
	}
	if err := html.Execute(&htmlBody, data); err != nil {
		return "", "", fmt.Errorf("render HTML mail template: %w", err)
	}
	return textBody.String(), htmlBody.String(), nil
}
