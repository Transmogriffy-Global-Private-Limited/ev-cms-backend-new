package billing

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

func TestMultiplyExact(t *testing.T) {
	t.Parallel()

	if value, overflow := multiplyExact(3, 2500); overflow || value != 7500 {
		t.Fatalf("multiplyExact(3, 2500) = %d, %v", value, overflow)
	}
	if _, overflow := multiplyExact(maxMoney, 2); !overflow {
		t.Fatal("expected oversized multiplication to overflow policy limit")
	}
	if _, overflow := multiplyExact(-1, 2); !overflow {
		t.Fatal("expected negative multiplication to be rejected")
	}
}

func TestBillingServiceRejectsNonPlatformPrincipal(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, false, nil)
	_, err := service.ListInvoices(
		t.Context(),
		auth.Principal{Scope: constants.AuthScopeCPO},
		uuid.Nil,
	)
	if err == nil {
		t.Fatal("expected CPO principal to be rejected")
	}
}
