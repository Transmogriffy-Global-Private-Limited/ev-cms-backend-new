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
