package routes

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	apidocs "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/docs/contracts/openapi"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
)

type pingerStub struct {
	err error
}

func TestCredentialRoutesAreRegisteredAndProtected(t *testing.T) {
	t.Parallel()

	router := newCredentialRouteTestRouter(t)

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

func newCredentialRouteTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
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
	return router
}

func TestOpenAPIContractMatchesRuntimeRoutesAndServesUI(t *testing.T) {
	t.Parallel()

	loader := openapi3.NewLoader()
	document, err := loader.LoadFromData(apidocs.Specification())
	if err != nil {
		t.Fatalf("parse embedded OpenAPI contract: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("validate embedded OpenAPI contract: %v", err)
	}

	router := newCredentialRouteTestRouter(t)
	runtimeOperations := make(map[string]struct{})
	parameter := regexp.MustCompile(`:([A-Za-z0-9_]+)`)
	for _, route := range router.Routes() {
		if route.Path == "/openapi.yaml" ||
			route.Path == "/docs" ||
			strings.HasPrefix(route.Path, "/docs/") {
			continue
		}
		path := parameter.ReplaceAllString(route.Path, `{$1}`)
		runtimeOperations[route.Method+" "+path] = struct{}{}
	}

	specOperations := make(map[string]struct{})
	for path, item := range document.Paths.Map() {
		operations := map[string]bool{
			http.MethodGet:    item.Get != nil,
			http.MethodPost:   item.Post != nil,
			http.MethodPut:    item.Put != nil,
			http.MethodDelete: item.Delete != nil,
			http.MethodPatch:  item.Patch != nil,
		}
		for method, present := range operations {
			if present {
				specOperations[method+" "+path] = struct{}{}
			}
		}
	}
	if difference := operationDifference(runtimeOperations, specOperations); len(difference) > 0 {
		t.Fatalf("runtime routes missing from OpenAPI: %s", strings.Join(difference, ", "))
	}
	if difference := operationDifference(specOperations, runtimeOperations); len(difference) > 0 {
		t.Fatalf("OpenAPI operations missing from runtime: %s", strings.Join(difference, ", "))
	}

	tests := []struct {
		path       string
		wantStatus int
		contains   string
	}{
		{"/openapi.yaml", http.StatusOK, "openapi: 3.1.0"},
		{"/docs", http.StatusTemporaryRedirect, ""},
		{"/docs/", http.StatusOK, "Swagger UI"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.wantStatus {
			t.Errorf("GET %s got status %d, want %d", test.path, recorder.Code, test.wantStatus)
		}
		if test.contains != "" && !strings.Contains(recorder.Body.String(), test.contains) {
			t.Errorf("GET %s response did not contain %q", test.path, test.contains)
		}
	}
}

func operationDifference(left, right map[string]struct{}) []string {
	result := make([]string, 0)
	for operation := range left {
		if _, exists := right[operation]; !exists {
			result = append(result, operation)
		}
	}
	sort.Strings(result)
	return result
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
