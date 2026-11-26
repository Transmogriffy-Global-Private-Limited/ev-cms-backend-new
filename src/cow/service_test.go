package cow

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/google/uuid"
)

func TestFailClosedWithoutDatabaseOrKnownCapability(t *testing.T) {
	for _, err := range []error{
		Authorize(context.Background(), nil, auth.Principal{}, "cow.jobs.read"),
		Authorize(context.Background(), nil, auth.Principal{}, "cow.*"),
		func() error { _, err := GetCPOAccess(context.Background(), nil, auth.Principal{}); return err }(),
		func() error {
			_, err := SetPlatformEntitlement(context.Background(), nil, auth.Principal{}, uuid.Nil, ChangeRequest{})
			return err
		}(),
	} {
		var api *auth.APIError
		if !errors.As(err, &api) || (api.Status != http.StatusForbidden && api.Status != http.StatusInternalServerError) {
			t.Fatalf("expected safe denial, got %v", err)
		}
	}
}
