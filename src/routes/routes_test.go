package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingerStub struct {
	err error
}

func (stub pingerStub) PingContext(context.Context) error {
	return stub.err
}

func TestHealthRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pinger     DatabasePinger
		path       string
		wantStatus int
	}{
		{
			name:       "live process",
			pinger:     pingerStub{},
			path:       "/health/live",
			wantStatus: http.StatusOK,
		},
		{
			name:       "ready database",
			pinger:     pingerStub{},
			path:       "/health/ready",
			wantStatus: http.StatusOK,
		},
		{
			name:       "unavailable database",
			pinger:     pingerStub{err: errors.New("database unavailable")},
			path:       "/health/ready",
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			New(test.pinger).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("got status %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}
