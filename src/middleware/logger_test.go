package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type testDiagnosticError struct {
	detail string
}

func (err *testDiagnosticError) Error() string {
	return err.detail
}

func (err *testDiagnosticError) SQLState() string {
	return "23505"
}

func TestRequestLoggerWritesSafeStructuredCompletionRecord(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	clockValues := []time.Time{startedAt, startedAt.Add(1500 * time.Microsecond)}
	clockIndex := 0
	clock := func() time.Time {
		value := clockValues[clockIndex]
		clockIndex++
		return value
	}

	router := gin.New()
	router.Use(newRequestLogger(&output, false, clock, func() string { return "request-test-id" }))
	router.POST("/api/v1/cpos/:cpo_id", func(ctx *gin.Context) {
		requestID, ok := RequestID(ctx)
		if !ok || requestID != "request-test-id" {
			t.Fatalf("request ID = %q, %v", requestID, ok)
		}
		SetRequestActor(ctx, RequestActor{
			AuthScope: "CPO",
			UserID:    "user-id",
			CPOID:     "cpo-id",
			Role:      "ADMIN",
		})
		ctx.JSON(http.StatusCreated, gin.H{"status": "created"})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/cpos/path-secret?email=query-secret@example.com",
		strings.NewReader(`{"password":"body-secret"}`),
	)
	request.RemoteAddr = "192.0.2.44:54321"
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set(RequestIDHeader, "client-request-id-secret")
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	request.Header.Set("User-Agent", "agent-secret")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	if got := recorder.Header().Get(RequestIDHeader); got != "request-test-id" {
		t.Fatalf("%s = %q", RequestIDHeader, got)
	}

	var record requestLogRecord
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode request log: %v\n%s", err, output.String())
	}
	if record.Timestamp != "2026-08-03T06:30:00.0015Z" ||
		record.Level != "INFO" ||
		record.Event != "http_request_completed" ||
		record.RequestID != "request-test-id" ||
		record.Method != http.MethodPost ||
		record.Route != "/api/v1/cpos/:cpo_id" ||
		record.Status != http.StatusCreated ||
		record.DurationMS != 1.5 ||
		record.ResponseBytes == 0 ||
		record.PeerIP != "192.0.2.44" ||
		record.ClientIP != "192.0.2.44" ||
		record.AuthScope != "CPO" ||
		record.UserID != "user-id" ||
		record.CPOID != "cpo-id" ||
		record.Role != "ADMIN" {
		t.Fatalf("unexpected request log: %#v", record)
	}
	for _, secret := range []string{
		"path-secret",
		"query-secret",
		"body-secret",
		"header-secret",
		"client-request-id-secret",
		"agent-secret",
	} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("request log leaked %q", secret)
		}
	}
}

func TestRequestLoggerTrustsForwardedClientOnlyFromLoopbackProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.8, 127.0.0.1")
	if got := requestClientIP(request, requestPeerIP(request.RemoteAddr)); got != "198.51.100.8" {
		t.Fatalf("client IP = %q, want trusted forwarded address", got)
	}

	request.RemoteAddr = "192.0.2.44:54321"
	if got := requestClientIP(request, requestPeerIP(request.RemoteAddr)); got != "192.0.2.44" {
		t.Fatalf("client IP = %q, want direct untrusted peer", got)
	}
}

func TestRequestLoggerIncludesHandledErrorCodeWithoutErrorMessage(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, time.August, 3, 6, 30, 0, 0, time.UTC)
	clockValues := []time.Time{startedAt, startedAt.Add(time.Millisecond)}
	clockIndex := 0

	router := gin.New()
	router.Use(newRequestLogger(&output, false, func() time.Time {
		value := clockValues[clockIndex]
		clockIndex++
		return value
	}, func() string { return "conflict-request-id" }))
	router.POST("/cpos", func(ctx *gin.Context) {
		SetRequestErrorCode(ctx, "cpo_gstin_conflict")
		ctx.JSON(http.StatusConflict, gin.H{
			"error": gin.H{
				"code":    "cpo_gstin_conflict",
				"message": "sensitive diagnostic detail",
			},
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/cpos", nil))

	var record requestLogRecord
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode request log: %v", err)
	}
	if record.Status != http.StatusConflict ||
		record.Level != "WARN" ||
		record.ErrorCode != "cpo_gstin_conflict" {
		t.Fatalf("unexpected conflict log: %#v", record)
	}
	if strings.Contains(output.String(), "sensitive diagnostic detail") {
		t.Fatal("request log included API error message")
	}
}

func TestDebugLoggingAddsSafeRequestStartAndHandledErrorDiagnostics(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, time.August, 3, 6, 30, 0, 0, time.UTC)
	clockValues := []time.Time{
		startedAt,
		startedAt.Add(500 * time.Microsecond),
		startedAt.Add(time.Millisecond),
	}
	clockIndex := 0

	router := gin.New()
	router.Use(newRequestLogger(&output, true, func() time.Time {
		value := clockValues[clockIndex]
		clockIndex++
		return value
	}, func() string { return "debug-request-id" }))
	router.POST("/api/v1/cpos/:cpo_id", func(ctx *gin.Context) {
		debugErr := &testDiagnosticError{detail: "database-value-secret"}
		LogHandledError(ctx, "cpo", "cpo_gstin_conflict", http.StatusConflict, debugErr)
		ctx.JSON(http.StatusConflict, gin.H{"error": "response-message-secret"})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/cpos/path-debug-secret?gstin=query-debug-secret",
		strings.NewReader(`{"password":"body-debug-secret"}`),
	)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.8")
	request.Header.Set("Authorization", "Bearer debug-token-secret")
	router.ServeHTTP(recorder, request)

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("log lines = %d, want start, handled error, and completion\n%s", len(lines), output.String())
	}
	var started requestDebugRecord
	if err := json.Unmarshal(lines[0], &started); err != nil {
		t.Fatalf("decode start diagnostic: %v", err)
	}
	var handled requestDebugRecord
	if err := json.Unmarshal(lines[1], &handled); err != nil {
		t.Fatalf("decode handled-error diagnostic: %v", err)
	}
	var completed requestLogRecord
	if err := json.Unmarshal(lines[2], &completed); err != nil {
		t.Fatalf("decode completion log: %v", err)
	}
	if started.Event != "http_request_started" ||
		started.Level != "DEBUG" ||
		started.RequestID != "debug-request-id" ||
		started.Method != http.MethodPost ||
		started.Route != "/api/v1/cpos/:cpo_id" ||
		started.PeerIP != "127.0.0.1" ||
		started.ClientIP != "198.51.100.8" {
		t.Fatalf("unexpected request-start diagnostic: %#v", started)
	}
	if handled.Event != "http_error_handled" ||
		handled.Level != "DEBUG" ||
		handled.Route != "/api/v1/cpos/:cpo_id" ||
		handled.Component != "cpo" ||
		handled.Status != http.StatusConflict ||
		handled.ErrorCode != "cpo_gstin_conflict" ||
		handled.ErrorType != "*middleware.testDiagnosticError" ||
		handled.ErrorClass != "postgresql" ||
		handled.SQLState != "23505" {
		t.Fatalf("unexpected handled-error diagnostic: %#v", handled)
	}
	if completed.Route != "/api/v1/cpos/:cpo_id" ||
		completed.ErrorCode != "cpo_gstin_conflict" {
		t.Fatalf("unexpected completion record: %#v", completed)
	}
	for _, secret := range []string{
		"database-value-secret",
		"response-message-secret",
		"path-debug-secret",
		"query-debug-secret",
		"body-debug-secret",
		"debug-token-secret",
	} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("debug log leaked %q", secret)
		}
	}
}

func TestRequestLoggerRecordsRecoveredPanicAsServerError(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, time.August, 3, 6, 30, 0, 0, time.UTC)
	clockValues := []time.Time{startedAt, startedAt.Add(2 * time.Millisecond)}
	clockIndex := 0

	router := gin.New()
	router.Use(newRequestLogger(&output, false, func() time.Time {
		value := clockValues[clockIndex]
		clockIndex++
		return value
	}, func() string { return "panic-request-id" }))
	router.Use(Recovery(&output))
	router.GET("/panic/:panic_id", func(_ *gin.Context) {
		panic("secret panic detail")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/panic/path-panic-secret?email=query-panic-secret",
		nil,
	)
	request.Header.Set("Authorization", "Bearer panic-token-secret")
	request.Header.Set("Cookie", "session=panic-cookie-secret")
	request.Header.Set("X-CPO-App-ID", "panic-app-id-secret")
	router.ServeHTTP(recorder, request)

	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want panic and completion records\n%s", len(lines), output.String())
	}
	var panicRecord panicLogRecord
	if err := json.Unmarshal(lines[0], &panicRecord); err != nil {
		t.Fatalf("decode panic log: %v", err)
	}
	var record requestLogRecord
	if err := json.Unmarshal(lines[1], &record); err != nil {
		t.Fatalf("decode request log: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError ||
		panicRecord.Event != "http_panic_recovered" ||
		panicRecord.Level != "ERROR" ||
		panicRecord.RequestID != "panic-request-id" ||
		panicRecord.Method != http.MethodGet ||
		panicRecord.Route != "/panic/:panic_id" ||
		panicRecord.PanicType != "string" ||
		!strings.Contains(panicRecord.Stack, "TestRequestLoggerRecordsRecoveredPanicAsServerError") ||
		record.Status != http.StatusInternalServerError ||
		record.Level != "ERROR" ||
		record.Route != "/panic/:panic_id" ||
		record.ErrorCode != "internal_error" {
		t.Fatalf(
			"unexpected panic result: status=%d panic=%#v completion=%#v",
			recorder.Code,
			panicRecord,
			record,
		)
	}
	for _, secret := range []string{
		"secret panic detail",
		"path-panic-secret",
		"query-panic-secret",
		"panic-token-secret",
		"panic-cookie-secret",
		"panic-app-id-secret",
	} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("panic/request log leaked %q", secret)
		}
	}
}

func TestRequestLogLevel(t *testing.T) {
	tests := map[int]string{
		http.StatusOK:                  "INFO",
		http.StatusPermanentRedirect:   "INFO",
		http.StatusBadRequest:          "WARN",
		http.StatusTooManyRequests:     "WARN",
		http.StatusInternalServerError: "ERROR",
	}
	for status, expected := range tests {
		if actual := requestLogLevel(status); actual != expected {
			t.Errorf("status %d level = %q, want %q", status, actual, expected)
		}
	}
}

func TestClassifyRequestError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantClass string
		wantState string
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantClass: "timeout"},
		{name: "canceled", err: context.Canceled, wantClass: "canceled"},
		{
			name: "postgresql", err: &testDiagnosticError{detail: "secret"},
			wantClass: "postgresql", wantState: "23505",
		},
		{name: "network", err: net.UnknownNetworkError("secret"), wantClass: "network"},
		{name: "application", err: errors.New("secret"), wantClass: "application"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualClass, actualState := classifyRequestError(test.err)
			if actualClass != test.wantClass || actualState != test.wantState {
				t.Fatalf(
					"classification = %q/%q, want %q/%q",
					actualClass,
					actualState,
					test.wantClass,
					test.wantState,
				)
			}
		})
	}
}
