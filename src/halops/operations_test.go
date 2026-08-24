package halops

import (
	"errors"
	"net"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func TestMappingFailureDiagnosticKeepsOnlySafeOperationalEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                   string
		cause                  error
		wantCategory, wantCode string
		wantStatus             any
	}{
		{name: "unavailable", cause: halclient.ErrUnavailable, wantCategory: "hal_unavailable"},
		{name: "timeout", cause: timeoutError{}, wantCategory: "timeout"},
		{name: "provider", cause: &halclient.HTTPError{Status: 409, Code: "mapping_conflict"}, wantCategory: "provider_http", wantCode: "mapping_conflict", wantStatus: 409},
		{name: "unsafe provider code discarded", cause: &halclient.HTTPError{Status: 400, Code: "body with secret"}, wantCategory: "provider_http", wantStatus: 400},
		{name: "invalid correlation", cause: halclient.ErrMissingCorrelationID, wantCategory: "invalid_correlation"},
		{name: "transport", cause: errors.New("transport"), wantCategory: "transport"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			category, status, code, detail := mappingFailureDiagnostic(test.cause)
			if category != test.wantCategory || status != test.wantStatus || code != test.wantCode || detail == "" {
				t.Fatalf("diagnostic=(%q,%v,%q,%q)", category, status, code, detail)
			}
		})
	}
}
