package customerauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestParseRechargeAmount(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantMinor int64
		wantErr   bool
	}{
		{name: "two decimals", value: "1250.50", wantMinor: 125050},
		{name: "whole amount", value: "100", wantMinor: 10000},
		{name: "too many decimals", value: "10.001", wantErr: true},
		{name: "zero", value: "0.00", wantErr: true},
		{name: "negative", value: "-1.00", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRechargeAmount(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected amount validation error")
				}
				return
			}
			if err != nil || got != test.wantMinor {
				t.Fatalf("parseRechargeAmount() = %d, %v; want %d", got, err, test.wantMinor)
			}
		})
	}
}

func TestVerifyRazorpayPaymentSignature(t *testing.T) {
	orderID := "order_test"
	paymentID := "pay_test"
	secret := "secret_test"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(orderID + "|" + paymentID))
	signature := hex.EncodeToString(mac.Sum(nil))

	if !verifyRazorpayPaymentSignature(orderID, paymentID, signature, secret) {
		t.Fatal("expected valid Razorpay payment signature")
	}
	if verifyRazorpayPaymentSignature(orderID, paymentID, signature+"0", secret) {
		t.Fatal("expected altered Razorpay payment signature to fail")
	}
}

func TestProviderSnapshotRemovesSensitiveFields(t *testing.T) {
	snapshot := providerSnapshot(map[string]interface{}{
		"id":     "pay_test",
		"amount": 125050,
		"secret": "must-not-persist",
		"card": map[string]interface{}{
			"last4":  "4242",
			"number": "4111111111111111",
		},
	})
	if snapshot["id"] != "pay_test" || snapshot["amount"] != 125050 {
		t.Fatalf("provider identity/value was not retained: %#v", snapshot)
	}
	if _, ok := snapshot["secret"]; ok {
		t.Fatal("provider secret was retained")
	}
	card := snapshot["card"].(map[string]any)
	if _, ok := card["number"]; ok {
		t.Fatal("card number was retained")
	}
	if card["last4"] != "4242" {
		t.Fatal("safe card metadata was not retained")
	}
}
