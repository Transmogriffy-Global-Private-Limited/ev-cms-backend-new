package platformops

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

func TestBoundedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  int
	}{
		{0, defaultPageSize},
		{-1, defaultPageSize},
		{25, 25},
		{maxPageSize + 1, maxPageSize},
	}
	for _, test := range tests {
		if got := boundedLimit(test.input); got != test.want {
			t.Errorf("boundedLimit(%d) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestPlatformOperationsRejectNonPlatformPrincipal(t *testing.T) {
	t.Parallel()

	err := requirePlatform(auth.Principal{Scope: constants.AuthScopeCPO})
	if err == nil {
		t.Fatal("expected CPO principal to be rejected")
	}
	apiError, ok := err.(*auth.APIError)
	if !ok || apiError.Code != "permission_denied" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
