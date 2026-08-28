package config

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildActionURL substitutes only named opaque identifiers into a prevalidated
// frontend template. Values are path/fragment escaped; OTPs are intentionally
// never an accepted placeholder.
func BuildActionURL(template string, values map[string]string, required ...string) (string, error) {
	for _, key := range required {
		value, ok := values[key]
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("missing action URL value %q", key)
		}
		placeholder := "{" + key + "}"
		if !strings.Contains(template, placeholder) {
			return "", fmt.Errorf("action URL template does not contain %s", placeholder)
		}
		template = strings.ReplaceAll(template, placeholder, url.QueryEscape(value))
	}
	if strings.Contains(template, "{") || strings.Contains(template, "}") {
		return "", fmt.Errorf("action URL template contains an unsupported placeholder")
	}
	parsed, err := url.Parse(template)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("invalid frontend action URL")
	}
	return parsed.String(), nil
}

func (links FrontendLinks) Validate() error {
	checks := []struct {
		name, template, key string
	}{
		{"ADMIN_LOGIN_VERIFY_URL_TEMPLATE", links.AdminLoginVerifyTemplate, "challenge_id"},
		{"ADMIN_PASSWORD_RESET_URL_TEMPLATE", links.AdminPasswordResetTemplate, "challenge_id"},
		{"CUSTOMER_LOGIN_VERIFY_URL_TEMPLATE", links.CustomerLoginVerifyTemplate, "challenge_id"},
		{"CUSTOMER_SIGNUP_VERIFY_URL_TEMPLATE", links.CustomerSignupVerifyTemplate, "challenge_id"},
		{"CUSTOMER_PASSWORD_RESET_URL_TEMPLATE", links.CustomerPasswordResetTemplate, "challenge_id"},
		{"CPO_ONBOARDING_URL_TEMPLATE", links.CPOOnboardingTemplate, "cpo_id"},
		{"CPO_SUPPORT_TICKET_URL_TEMPLATE", links.CPOSupportTicketTemplate, "ticket_id"},
	}
	for _, check := range checks {
		if _, err := BuildActionURL(check.template, map[string]string{check.key: "opaque-test-value"}, check.key); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
	}
	if _, err := BuildActionURL(links.CPOSubscriptionURL, nil); err != nil {
		return fmt.Errorf("CPO_SUBSCRIPTION_URL: %w", err)
	}
	return nil
}
