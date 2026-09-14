package mail

import "testing"

func TestInvoiceFromNameUsesCPOIssuerAndKeepsTransportFallback(t *testing.T) {
	if got := invoiceFromName("Sunrise Mobility", "Technical SMTP"); got != "Sunrise Mobility" {
		t.Fatalf("visible invoice sender = %q, want CPO issuer", got)
	}
	if got := invoiceFromName("  ", "Technical SMTP"); got != "Technical SMTP" {
		t.Fatalf("blank issuer fallback = %q", got)
	}
}
