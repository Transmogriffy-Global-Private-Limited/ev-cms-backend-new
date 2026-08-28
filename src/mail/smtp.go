package mail

import (
	"context"
	"errors"
	"fmt"
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
	var subject, textBody, htmlBody string
	var err error
	if isLegacyTemplate(template) {
		subject, textBody, err = renderMessageContent(template, payload)
		if err == nil {
			textBody, htmlBody, err = renderMessageTemplates(payload.RecipientName, textBody)
		}
	} else {
		subject, textBody, htmlBody, err = renderSemanticMessage(template, payload, sender.mailConfig)
	}
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
	occurred := payload.OccurredAt.In(cfg.DisplayLocation).Format("02 Jan 2006, 3:04 PM MST")
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
	if link == "" && (template == "CPO_SUBSCRIPTION_EXPIRY_WARNING" || template == "CPO_SUBSCRIPTION_EXPIRED") {
		link = cfg.Frontend.CPOSubscriptionURL
	}
	return renderSemanticTemplates(template, semanticMessage{
		RecipientName: name, Code: payload.Code, ExpiresAt: expires, OccurredAt: occurred,
		TemporaryPassword: payload.TemporaryPassword, CPOName: payload.CPOName,
		Role: roleName(payload.Role), ActionURL: link, SupportSubject: payload.SupportSubject,
		SupportStatus: payload.SupportStatus,
	})
}

func isLegacyTemplate(template string) bool {
	switch template {
	case "CPO_ADMIN_WELCOME", "CPO_MEMBERSHIP_ASSIGNED", "PLATFORM_ADMIN_INVITE", "PLATFORM_ADMIN_GRANTED", "PASSWORD_CHANGE_REMINDER":
		return true
	default:
		return false
	}
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
