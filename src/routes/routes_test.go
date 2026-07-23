package routes

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
)

type pingerStub struct {
	err error
}

func TestCredentialRoutesAreRegisteredAndProtected(t *testing.T) {
	t.Parallel()

	tokenManager, err := security.NewTokenManager(
		"routes-test",
		"routes-test-api",
		15*time.Minute,
		[]byte(strings.Repeat("s", 32)),
		[]byte(strings.Repeat("e", 32)),
	)
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	mailBox, err := security.NewSecretBox(
		"routes-test-v1",
		[]byte(strings.Repeat("m", 32)),
	)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	authService, err := auth.NewService(
		nil,
		config.Auth{
			AccessTTL:         15 * time.Minute,
			SessionTTL:        24 * time.Hour,
			OTPExpiry:         10 * time.Minute,
			OTPResendCooldown: time.Minute,
			OTPHMACKey:        []byte(strings.Repeat("o", 32)),
			LoginMaxAttempts:  5,
			LoginLockDuration: 15 * time.Minute,
			RateLimitWindow:   15 * time.Minute,
			RateLimitMax:      20,
		},
		false,
		cmsmail.NewOutbox(mailBox),
		tokenManager,
	)
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	credentialBox, err := security.NewSecretBox(
		"routes-test-v1",
		[]byte(strings.Repeat("c", 32)),
	)
	if err != nil {
		t.Fatalf("create credential secret box: %v", err)
	}
	router := New(
		pingerStub{},
		authService,
		cpo.NewService(nil, cmsmail.NewOutbox(mailBox), true),
		integrations.NewService(nil, credentialBox),
	)

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodPost, "/api/v1/auth/logout-all"},
		{http.MethodGet, "/api/v1/auth/sessions"},
		{http.MethodDelete, "/api/v1/auth/sessions/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/auth/password/change"},
		{http.MethodPost, "/api/v1/platform/cpos"},
		{http.MethodGet, "/api/v1/platform/cpos"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/activate"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/suspend"},
		{http.MethodPut, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/app-id"},
		{http.MethodGet, "/api/v1/cpo/integrations"},
		{http.MethodGet, "/api/v1/cpo/integrations/RAZORPAY"},
		{http.MethodPut, "/api/v1/cpo/integrations/RAZORPAY"},
		{http.MethodDelete, "/api/v1/cpo/integrations/RAZORPAY"},
	}
	for _, route := range protected {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			route.method,
			route.path,
			bytes.NewBufferString(`{}`),
		)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf(
				"%s %s got status %d, want 401",
				route.method,
				route.path,
				recorder.Code,
			)
		}
	}

	public := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/2fa/verify",
		"/api/v1/auth/2fa/resend",
		"/api/v1/auth/refresh",
		"/api/v1/auth/password/forgot",
		"/api/v1/auth/password/reset",
	}
	for _, path := range public {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			path,
			bytes.NewBufferString(`{`),
		)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("POST %s got status %d, want 400", path, recorder.Code)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("POST %s did not set no-store", path)
		}
	}
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
			New(test.pinger, nil, nil, nil).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("got status %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}
