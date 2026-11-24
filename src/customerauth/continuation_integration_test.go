package customerauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// This proves the HTTP boundary under suspension, NOT a physical HAL Stop.
// Physical HAL/charger acceptance remains an independent required test.
func TestSuspendedCPOCustomerStopContinuationHTTPBoundaryWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	database, sqlDB, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(context.Background(), sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	tokens, err := security.NewTokenManager("safety-continuation-test", "safety-continuation-test-api", 15*time.Minute,
		[]byte(strings.Repeat("s", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(database, config.Auth{SessionTTL: time.Hour, RateLimitWindow: time.Minute, RateLimitMax: 100}, false, nil, tokens)
	if err != nil {
		t.Fatal(err)
	}
	cpo := createActiveTestCPO(t, database)
	now := time.Now().UTC()
	customer := models.Customer{ID: uuid.New(), CPOID: cpo.ID, Email: "continuation-" + uuid.NewString() + "@example.test",
		PasswordHash: "test-only-not-authenticatable", FullName: "Test Customer", IsVerified: true,
		Status: constants.CustomerStatusActive, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatal(err)
	}
	wallet := models.Wallet{ID: uuid.New(), CPOID: cpo.ID, CustomerID: customer.ID,
		Currency: "INR", CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	session := models.CustomerAuthSession{ID: uuid.New(), CPOID: cpo.ID, CustomerID: customer.ID,
		TokenVersion: 1, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := database.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	accessToken, _, err := tokens.Issue(now, customer.ID, session.ID, constants.AuthScopeCustomer, &cpo.ID, nil, session.TokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/app"), service)
	call := func(method, path, appID, token string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("X-CPO-App-ID", appID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if code := call(http.MethodGet, "/api/v1/app/me", cpo.AppID, accessToken); code != http.StatusOK {
		t.Fatalf("active CPO customer /me = %d, want 200", code)
	}
	if err := database.Model(&models.CPO{}).Where("id = ?", cpo.ID).Update("status", constants.CPOStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if code := call(http.MethodGet, "/api/v1/app/me", cpo.AppID, accessToken); code != http.StatusUnauthorized {
		t.Fatalf("suspended CPO ordinary customer read = %d, want 401", code)
	}
	for _, path := range []string{"/api/v1/app/charging-sessions", "/api/v1/app/wallet/recharge/orders"} {
		if code := call(http.MethodPost, path, cpo.AppID, accessToken); code != http.StatusUnauthorized {
			t.Fatalf("suspended CPO paid route %s = %d, want 401", path, code)
		}
	}
	// Existing-session and intent lookups are narrowly reachable with an owned
	// identifier, without enabling new starts or generic Customer profile reads.
	if code := call(http.MethodGet, "/api/v1/app/charging-start-intents/"+uuid.NewString(), cpo.AppID, accessToken); code != http.StatusNotFound {
		t.Fatalf("suspended CPO start-intent recovery read = %d, want 404 for unknown intent", code)
	}
	if code := call(http.MethodGet, "/api/v1/app/charging-sessions/"+uuid.NewString(), cpo.AppID, accessToken); code != http.StatusNotFound {
		t.Fatalf("suspended CPO session recovery read = %d, want 404 for unknown session", code)
	}
	stopURL := "/api/v1/app/charging-sessions/" + uuid.NewString() + "/stop"
	// HAL is deliberately not configured. 503 proves the route passed auth
	// and reached StopCharging; it does NOT prove HAL command delivery.
	if code := call(http.MethodPost, stopURL, cpo.AppID, accessToken); code != http.StatusServiceUnavailable {
		t.Fatalf("suspended CPO Stop auth boundary = %d, want service 503", code)
	}
	if code := call(http.MethodPost, stopURL, "wrong-application", accessToken); code != http.StatusForbidden {
		t.Fatalf("suspended CPO mismatched app ID = %d, want 403", code)
	}
	if err := database.Model(&models.CustomerAuthSession{}).Where("id = ?", session.ID).
		Update("revoked_at", now).Error; err != nil {
		t.Fatal(err)
	}
	if code := call(http.MethodPost, stopURL, cpo.AppID, accessToken); code != http.StatusUnauthorized {
		t.Fatalf("revoked customer Stop = %d, want 401", code)
	}
}
