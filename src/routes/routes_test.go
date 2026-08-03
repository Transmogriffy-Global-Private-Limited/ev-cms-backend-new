package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		{http.MethodGet, "/api/v1/platform/cpos/slug-availability?slug=example-cpo"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001"},
		{http.MethodPut, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/profile"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/activate"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/suspend"},
		{http.MethodPut, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/app-id"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/primary-admin"},
		{http.MethodPut, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/primary-admin"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/primary-admin/resend-onboarding"},
		{http.MethodPost, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/administrative-sessions/revoke"},
		{http.MethodGet, "/api/v1/platform/events"},
		{http.MethodGet, "/api/v1/platform/realtime/stream"},
		{http.MethodGet, "/api/v1/platform/audit-logs"},
		{http.MethodGet, "/api/v1/platform/workers"},
		{http.MethodGet, "/api/v1/cpo/integrations"},
		{http.MethodGet, "/api/v1/cpo/integrations/RAZORPAY"},
		{http.MethodPut, "/api/v1/cpo/integrations/RAZORPAY"},
		{http.MethodDelete, "/api/v1/cpo/integrations/RAZORPAY"},
		{http.MethodGet, "/api/v1/cpo/admin/profile"},
		{http.MethodPatch, "/api/v1/cpo/admin/profile"},
		{http.MethodGet, "/api/v1/cpo/organization"},
		{http.MethodPost, "/api/v1/cpo/chargers"},
		{http.MethodGet, "/api/v1/cpo/chargers"},
		{http.MethodGet, "/api/v1/cpo/chargers/abc123"},
		{http.MethodPatch, "/api/v1/cpo/chargers/abc123"},
		{http.MethodDelete, "/api/v1/cpo/chargers/abc123"},
		{http.MethodPost, "/api/v1/cpo/hubs"},
		{http.MethodGet, "/api/v1/cpo/hubs"},
		{http.MethodGet, "/api/v1/cpo/hubs/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/api/v1/cpo/hubs/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/cpo/tariffs"},
		{http.MethodGet, "/api/v1/cpo/tariffs"},
		{http.MethodGet, "/api/v1/cpo/tariffs/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/api/v1/cpo/tariffs/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/cpo/gsts"},
		{http.MethodGet, "/api/v1/cpo/gsts"},
		{http.MethodGet, "/api/v1/cpo/gsts/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/api/v1/cpo/gsts/00000000-0000-0000-0000-000000000001"},
		{http.MethodGet, "/api/v1/app/auth/me"},
		{http.MethodGet, "/api/v1/app/auth/sessions"},
		{http.MethodDelete, "/api/v1/app/auth/sessions/00000000-0000-0000-0000-000000000001"},
		{http.MethodPost, "/api/v1/app/auth/logout"},
		{http.MethodPost, "/api/v1/app/auth/logout-all"},
		{http.MethodPost, "/api/v1/app/auth/password/change"},
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
		"/api/v1/app/auth/signup",
		"/api/v1/app/auth/signup/verify",
		"/api/v1/app/auth/signup/resend",
		"/api/v1/app/auth/login",
		"/api/v1/app/auth/login/verify",
		"/api/v1/app/auth/login/resend",
		"/api/v1/app/auth/refresh",
		"/api/v1/app/auth/password/forgot",
		"/api/v1/app/auth/password/reset/resend",
		"/api/v1/app/auth/password/reset",
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

func TestRetiredCommercialRoutesAreNotRegistered(t *testing.T) {
	t.Parallel()

	router := newCredentialRouteTestRouter(t)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/platform/plans"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/subscription"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/entitlements"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/billing-account"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/invoices"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/payments"},
		{http.MethodGet, "/api/v1/platform/cpos/00000000-0000-0000-0000-000000000001/billing-timeline"},
	}
	for _, route := range routes {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, route.path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf(
				"%s %s got status %d, want 404",
				route.method,
				route.path,
				recorder.Code,
			)
		}
	}
}

func TestTenantOrganizationProfileIsNotRegistered(t *testing.T) {
	t.Parallel()

	router := newCredentialRouteTestRouter(t)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/api/v1/cpo/profile", nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf(
				"%s /api/v1/cpo/profile got status %d, want 404",
				method,
				recorder.Code,
			)
		}
	}
}

func newCredentialRouteTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return newCredentialRouteTestRouterWithLog(t, io.Discard, false)
}

func newCredentialRouteTestRouterWithLog(
	t *testing.T,
	requestLogWriter io.Writer,
	debugLogging bool,
) *gin.Engine {
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
	customerAuthService, err := customerauth.NewService(
		nil, config.Auth{}, false, cmsmail.NewOutbox(mailBox), tokenManager,
	)
	if err != nil {
		t.Fatalf("create customer auth service: %v", err)
	}
	router := New(
		pingerStub{},
		authService,
		customerAuthService,
		cpo.NewService(nil, cmsmail.NewOutbox(mailBox), true),
		integrations.NewService(nil, credentialBox),
		platformops.NewService(nil, config.Platform{}),
		true,
		true,
		requestLogWriter,
		debugLogging,
	)
	return router
}

func TestDebugRequestLoggerIncludesHandledAuthenticationFailure(t *testing.T) {
	var output bytes.Buffer
	router := newCredentialRouteTestRouterWithLog(t, &output, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if _, err := uuid.Parse(recorder.Header().Get("X-Request-ID")); err != nil {
		t.Fatalf("response request ID is not a UUID: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("log lines = %d, want start, handled error, and completion\n%s", len(lines), output.String())
	}
	var handled map[string]any
	if err := json.Unmarshal(lines[1], &handled); err != nil {
		t.Fatalf("decode handled-error log: %v", err)
	}
	if handled["event"] != "http_error_handled" ||
		handled["component"] != "auth" ||
		handled["status"] != float64(http.StatusUnauthorized) ||
		handled["error_code"] != "unauthorized" ||
		handled["error_type"] != "*auth.APIError" ||
		handled["error_class"] != "application" {
		t.Fatalf("unexpected handled authentication log: %#v", handled)
	}
	var record map[string]any
	if err := json.Unmarshal(lines[2], &record); err != nil {
		t.Fatalf("decode request log: %v", err)
	}
	if record["route"] != "/api/v1/auth/me" ||
		record["status"] != float64(http.StatusUnauthorized) ||
		record["error_code"] != "unauthorized" {
		t.Fatalf("unexpected authentication request log: %#v", record)
	}
}

func TestPermissiveCORSAllowsRemoteBrowserPreflight(t *testing.T) {
	t.Parallel()

	router := newCredentialRouteTestRouter(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/app/auth/login", nil)
	request.Header.Set("Origin", "http://192.0.2.10:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set(
		"Access-Control-Request-Headers",
		"authorization,content-type,x-cpo-app-id,x-client-version",
	)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight got status %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("allow origin = %q, want *", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); got !=
		"authorization,content-type,x-cpo-app-id,x-client-version" {
		t.Errorf("allow headers = %q, want requested headers", got)
	}
	if got := recorder.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-ID" {
		t.Errorf("expose headers = %q, want X-Request-ID", got)
	}
}

func TestDisabledCORSDoesNotAddCrossOriginHeaders(t *testing.T) {
	t.Parallel()

	router := gin.New()
	router.Use(permissiveCORSMiddleware(false))
	router.GET("/test", func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set("Origin", "http://192.0.2.10:5173")
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disabled CORS set allow origin %q", got)
	}
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
			New(test.pinger, nil, nil, nil, nil, nil, false, false, io.Discard, false).
				ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("got status %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

func TestAPIDocumentationRoutesCanBeDisabled(t *testing.T) {
	t.Parallel()

	router := New(pingerStub{}, nil, nil, nil, nil, nil, false, false, io.Discard, false)
	for _, path := range []string{"/docs", "/docs/", "/openapi.yaml"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s got status %d, want 404", path, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("health route got status %d with docs disabled, want 200", recorder.Code)
	}
}
