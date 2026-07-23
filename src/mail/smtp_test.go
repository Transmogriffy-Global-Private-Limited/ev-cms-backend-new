package mail

import (
	"strings"
	"testing"

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
