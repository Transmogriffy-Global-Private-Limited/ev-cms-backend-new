package integrations

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

func TestMaskedKeyID(t *testing.T) {
	t.Parallel()

	if got := maskedKeyID("rzp_test_12345678"); got != "****5678" {
		t.Fatalf("got %q", got)
	}
	if got := maskedKeyID("abc"); got != "****" {
		t.Fatalf("short key got %q", got)
	}
}

func TestCPOCredentialServiceRejectsNonAdministrativePrincipal(t *testing.T) {
	t.Parallel()

	cpoID := uuid.New()
	operator := constants.CPORoleOperator
	_, err := requireCPOPrincipal(auth.Principal{
		UserID: uuid.New(),
		Scope:  constants.AuthScopeCPO,
		CPOID:  &cpoID,
		Role:   &operator,
	})
	if err == nil {
		t.Fatal("operator was allowed to manage integration credentials")
	}
}
