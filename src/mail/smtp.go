package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	gomail "github.com/wneessen/go-mail"
)

type SMTPSender struct {
	client      *gomail.Client
	fromAddress string
	fromName    string
	mailConfig  config.Mail
}

func NewSMTPSender(cfg config.Mail) (*SMTPSender, error) {
	options := []gomail.Option{
		gomail.WithPort(cfg.Port),
	}
	switch {
	case cfg.UseSSL && !cfg.UseTLS:
		options = append(options, gomail.WithSSL())
	case cfg.UseTLS && !cfg.UseSSL:
		options = append(options, gomail.WithTLSPortPolicy(gomail.TLSMandatory))
	default:
		return nil, errors.New(
			"exactly one SMTP transport must be enabled: STARTTLS or implicit SSL",
		)
	}
	if cfg.Username != "" {
		options = append(
			options,
			gomail.WithUsername(cfg.Username),
			gomail.WithPassword(cfg.Password),
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
		)
	}
	client, err := gomail.NewClient(cfg.Host, options...)
	if err != nil {
		return nil, fmt.Errorf("create SMTP client: %w", err)
	}
	client.SetLogAuthData(false)
	return &SMTPSender{
		client:      client,
		fromAddress: cfg.FromAddress,
		fromName:    cfg.FromName,
		mailConfig:  cfg,
	}, nil
}

func (sender *SMTPSender) SendMessage(
	ctx context.Context,
	toEmail string,
	template string,
	payload MessagePayload,
) error {
	subject, textBody, htmlBody, err := renderSemanticMessage(template, payload, sender.mailConfig)
	if err != nil {
		return err
	}
	message := gomail.NewMsg()
	if err := message.FromFormat(sender.fromName, sender.fromAddress); err != nil {
		return fmt.Errorf("set mail sender: %w", err)
	}
	if err := message.To(toEmail); err != nil {
		return fmt.Errorf("set mail recipient: %w", err)
	}

	message.Subject(subject)
	message.SetBodyString(gomail.TypeTextPlain, textBody)
	message.AddAlternativeString(gomail.TypeTextHTML, htmlBody)
	if err := sender.client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("send SMTP message: %w", err)
	}
	return nil
}

// renderMessageContent is retained solely for previously queued legacy jobs
// and older unit callers. New SMTP delivery uses renderSemanticMessage.
func renderMessageContent(template string, payload MessagePayload) (string, string, error) {
	if err := validateMessagePayload(template, payload); err != nil {
		return "", "", err
	}
	subject := "Your TransEV CMS verification code"
	body := ""
	switch template {
	case "LOGIN_OTP":
		subject = "Your TransEV CMS sign-in code"
		body = fmt.Sprintf(
			"Use %s to sign in to your TransEV CMS account. It expires at %s.",
			payload.Code,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	case "CUSTOMER_LOGIN_OTP":
		subject = "Your charging app sign-in code"
		body = fmt.Sprintf(
			"Use %s to sign in to your charging account. It expires at %s.",
			payload.Code,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	case "CUSTOMER_PASSWORD_RESET_OTP":
		subject = "Reset your charging account password"
		body = fmt.Sprintf(
			"Use code %s and recovery ID %s to reset your charging account password. They expire at %s.",
			payload.Code,
			payload.ChallengeID,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	case "CUSTOMER_SIGNUP_OTP":
		subject = "Verify your charging account"
		body = fmt.Sprintf(
			"Use %s to verify your charging account. It expires at %s.",
			payload.Code,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	case "PASSWORD_RESET_OTP":
		subject = "Reset your TransEV CMS password"
		body = fmt.Sprintf(
			"Use code %s and recovery ID %s to reset your password. They expire at %s.",
			payload.Code,
			payload.ChallengeID,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	case "CPO_ADMIN_WELCOME":
		subject = "Your TransEV CMS CPO administrator account"
		body = fmt.Sprintf(
			"You have been assigned as an administrator for %s.\n\nCPO ID: %s\nTemporary password: %s\nOnboarding app ID: %s\n\nThis password does not expire, but you must change it after signing in before using tenant operations.",
			payload.CPOName,
			payload.CPOID,
			payload.TemporaryPassword,
			payload.CPOAppID,
		)
	case "PLATFORM_ADMIN_INVITE":
		subject = "Your TransEV CMS platform administrator account"
		body = fmt.Sprintf(
			"You have been invited as a platform administrator for TransEV CMS.\n\nTemporary password: %s\n\nChange this password after signing in.",
			payload.TemporaryPassword,
		)
	case "PLATFORM_ADMIN_GRANTED":
		subject = "TransEV CMS platform administrator access granted"
		body = "Platform administrator authority has been granted to your existing TransEV CMS identity. Sign in with your existing password."
	case "CPO_MEMBERSHIP_ASSIGNED":
		subject = "You were added to a TransEV CMS CPO"
		body = fmt.Sprintf(
			"Your existing TransEV CMS identity was assigned as an administrator for %s.\n\nCPO ID: %s\nOnboarding app ID: %s\nUse your existing password to sign in after the CPO is activated.",
			payload.CPOName,
			payload.CPOID,
			payload.CPOAppID,
		)
	case "CPO_ONBOARDING_RESENT":
		subject = "Your TransEV CMS CPO access details"
		body = fmt.Sprintf(
			"Your administrator access details for %s are below.\n\nCPO ID: %s\nCurrent app ID: %s\n\nUse your existing TransEV CMS password. If you do not know it, use the password-recovery flow; passwords are never resent by email.",
			payload.CPOName,
			payload.CPOID,
			payload.CPOAppID,
		)
	case "PASSWORD_CHANGE_REMINDER":
		subject = "Change your temporary TransEV CMS password"
		body = "Your account is still using its temporary password. Change it now from the authenticated password-change screen before using tenant operations."
	default:
		return "", "", fmt.Errorf("render mail template: unknown template %q", template)
	}
	return subject, body, nil
}

func renderSemanticMessage(template string, payload MessagePayload, cfg config.Mail) (string, string, string, error) {
	if err := validateMessagePayload(template, payload); err != nil {
		return "", "", "", err
	}
	name := strings.TrimSpace(payload.RecipientName)
	if name == "" {
		name = "there"
	}
	expires := payload.ExpiresAt.In(cfg.DisplayLocation).Format("02 Jan 2006, 3:04 PM MST")
	link := payload.ActionURL
	if link == "" && payload.ChallengeID != "" {
		var err error
		switch template {
		case "LOGIN_OTP":
			link, err = config.BuildActionURL(cfg.Frontend.AdminLoginVerifyTemplate, map[string]string{"challenge_id": payload.ChallengeID}, "challenge_id")
		case "PASSWORD_RESET_OTP":
			link, err = config.BuildActionURL(cfg.Frontend.AdminPasswordResetTemplate, map[string]string{"challenge_id": payload.ChallengeID}, "challenge_id")
		case "CUSTOMER_LOGIN_OTP":
			link, err = config.BuildActionURL(cfg.Frontend.CustomerLoginVerifyTemplate, map[string]string{"challenge_id": payload.ChallengeID}, "challenge_id")
		case "CUSTOMER_SIGNUP_OTP":
			link, err = config.BuildActionURL(cfg.Frontend.CustomerSignupVerifyTemplate, map[string]string{"challenge_id": payload.ChallengeID}, "challenge_id")
		case "CUSTOMER_PASSWORD_RESET_OTP":
			link, err = config.BuildActionURL(cfg.Frontend.CustomerPasswordResetTemplate, map[string]string{"challenge_id": payload.ChallengeID}, "challenge_id")
		}
		if err != nil {
			return "", "", "", err
		}
	}
	var subject, text string
	switch template {
	case "LOGIN_OTP":
		subject = "Your TransEV CMS sign-in code"
		text = fmt.Sprintf("Hi %s,\n\nUse this sign-in code: %s\nIt expires %s. Do not share it. If this was not you, secure your account.\n\n%s", name, payload.Code, expires, link)
	case "PASSWORD_RESET_OTP":
		subject = "Reset your TransEV CMS password"
		text = fmt.Sprintf("Hi %s,\n\nA password reset was requested. Your verification code is: %s\nIt expires %s. Receiving this email has not changed your password.\n\nReset password: %s", name, payload.Code, expires, link)
	case "CUSTOMER_LOGIN_OTP":
		subject = "Your charging app sign-in code"
		text = fmt.Sprintf("Hi %s,\n\nUse this sign-in code: %s\nIt expires %s. Do not share it.\n\n%s", name, payload.Code, expires, link)
	case "CUSTOMER_SIGNUP_OTP":
		subject = "Verify your charging account"
		text = fmt.Sprintf("Hi %s,\n\nUse this verification code: %s\nIt expires %s.\n\n%s", name, payload.Code, expires, link)
	case "CUSTOMER_PASSWORD_RESET_OTP":
		subject = "Reset your charging account password"
		text = fmt.Sprintf("Hi %s,\n\nYour password-reset code is: %s\nIt expires %s. Receiving this email has not changed your password.\n\nReset password: %s", name, payload.Code, expires, link)
	case "CPO_STAFF_NEW_IDENTITY", "CPO_ADMIN_WELCOME":
		subject = "Your CPO access"
		text = fmt.Sprintf("Hi %s,\n\nYou have been added to %s as %s. Sign in using the temporary password below, then change it before using CPO operations.\n\nTemporary password: %s\n\n%s", name, payload.CPOName, roleName(payload.Role), payload.TemporaryPassword, payload.ActionURL)
	case "CPO_STAFF_EXISTING_IDENTITY", "CPO_MEMBERSHIP_ASSIGNED":
		subject = "You have CPO access"
		text = fmt.Sprintf("Hi %s,\n\nYou have been added to %s as %s. Use your existing TransEV credentials to sign in.\n\n%s", name, payload.CPOName, roleName(payload.Role), payload.ActionURL)
	case "CPO_ONBOARDING_RESENT":
		subject = "Your CPO access reminder"
		text = fmt.Sprintf("Hi %s,\n\nUse your existing TransEV credentials to access %s. Passwords are never resent; use password recovery if needed.\n\n%s", name, payload.CPOName, payload.ActionURL)
	case "CPO_STAFF_ROLE_CHANGED":
		subject = "Your CPO role has changed"
		text = fmt.Sprintf("Hi %s,\n\nYour role for %s is now %s. Use your existing TransEV credentials to sign in.\n\n%s", name, payload.CPOName, roleName(payload.Role), payload.ActionURL)
	case "CPO_STAFF_SUSPENDED":
		subject = "Your CPO access has been suspended"
		text = fmt.Sprintf("Hi %s,\n\nYour access to %s has been suspended. Your TransEV account has not been deleted.\n\n%s", name, payload.CPOName, payload.ActionURL)
	case "CPO_STAFF_REACTIVATED":
		subject = "Your CPO access has been restored"
		text = fmt.Sprintf("Hi %s,\n\nYour access to %s has been restored. Use your existing TransEV credentials to sign in.\n\n%s", name, payload.CPOName, payload.ActionURL)
	case "CPO_STAFF_REVOKED":
		subject = "Your CPO access has been removed"
		text = fmt.Sprintf("Hi %s,\n\nYour access to %s has been removed. This does not delete your TransEV account.\n\n%s", name, payload.CPOName, payload.ActionURL)
	case "CPO_SUBSCRIPTION_EXPIRY_WARNING":
		subject = "Your CPO subscription is ending soon"
		text = fmt.Sprintf("Hi %s,\n\n%s subscription ends on %s. Review renewal with your platform administrator.\n\n%s", name, payload.CPOName, expires, cfg.Frontend.CPOSubscriptionURL)
	case "CPO_SUBSCRIPTION_EXPIRED":
		subject = "Your CPO subscription has expired"
		text = fmt.Sprintf("Hi %s,\n\n%s subscription expired on %s. New customer charging starts and recharge orders remain unavailable until a platform administrator renews the subscription.\n\n%s", name, payload.CPOName, expires, cfg.Frontend.CPOSubscriptionURL)
	default:
		return "", "", "", fmt.Errorf("render semantic mail template: unknown template %q", template)
	}
	htmlBody := "<html><body><pre>" + html.EscapeString(text) + "</pre></body></html>"
	return subject, text, htmlBody, nil
}

func roleName(role string) string {
	switch strings.ToUpper(strings.TrimSpace(role)) {
	case "OPERATOR":
		return "Operator"
	case "VIEWER":
		return "Viewer"
	case "OWNER":
		return "Owner"
	default:
		return "Administrator"
	}
}
