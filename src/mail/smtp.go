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
	}, nil
}

func (sender *SMTPSender) SendMessage(
	ctx context.Context,
	toEmail string,
	template string,
	payload MessagePayload,
) error {
	message := gomail.NewMsg()
	if err := message.FromFormat(sender.fromName, sender.fromAddress); err != nil {
		return fmt.Errorf("set mail sender: %w", err)
	}
	if err := message.To(toEmail); err != nil {
		return fmt.Errorf("set mail recipient: %w", err)
	}

	subject := "Your TransEV CMS verification code"
	body := ""
	switch template {
	case "PASSWORD_RESET_OTP":
		subject = "Reset your TransEV CMS password"
		body = fmt.Sprintf(
			"Use %s to reset your password. It expires at %s.",
			payload.Code,
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
	case "CPO_MEMBERSHIP_ASSIGNED":
		subject = "You were added to a TransEV CMS CPO"
		body = fmt.Sprintf(
			"Your existing TransEV CMS identity was assigned as an administrator for %s.\n\nCPO ID: %s\nOnboarding app ID: %s\nUse your existing password to sign in after the CPO is activated.",
			payload.CPOName,
			payload.CPOID,
			payload.CPOAppID,
		)
	case "PASSWORD_CHANGE_REMINDER":
		subject = "Change your temporary TransEV CMS password"
		body = "Your account is still using its temporary password. Change it now from the authenticated password-change screen before using tenant operations."
	default:
		body = fmt.Sprintf(
			"Use %s to sign in. It expires at %s.",
			payload.Code,
			payload.ExpiresAt.UTC().Format("02 Jan 2006 15:04 UTC"),
		)
	}
	message.Subject(subject)
	name := strings.TrimSpace(payload.RecipientName)
	if name == "" {
		name = "there"
	}
	message.SetBodyString(
		gomail.TypeTextPlain,
		fmt.Sprintf(
			"Hi %s,\n\n%s\n\nIf you did not expect this message, contact your administrator.",
			name,
			body,
		),
	)
	if err := sender.client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("send SMTP message: %w", err)
	}
	return nil
}
