package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCPOAdministrativeAppIDLoginWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	email := "appid-admin-" + uuid.NewString() + "@example.test"
	password := "AppIDLogin-Test!123"
	if err := db.SeedSuperadmin(ctx, database, config.Superadmin{Email: email, Password: password, FullName: "App-ID Admin"}); err != nil {
		t.Fatal(err)
	}
	var user models.User
	if err := database.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	service, box := newIntegrationAuthService(t, database)
	service.config.RateLimitMax = 500
	now := time.Now().UTC()
	createCPO := func(member bool) (models.CPO, models.CPOMembership) {
		cpo := models.CPO{ID: uuid.New(), Slug: "appid-" + uuid.NewString(), BusinessName: "App-ID Test CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: testsupport.ValidGSTIN("19"), Address: "1 Test Road", City: "Kolkata", State: constants.WestBengal, Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "test", StatusChangedAt: now, AppID: "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := database.Create(&cpo).Error; err != nil {
			t.Fatal(err)
		}
		membership := models.CPOMembership{ID: uuid.New(), CPOID: cpo.ID, UserID: user.ID, Role: constants.CPORoleAdmin, Status: constants.MembershipStatusActive, CreatedAt: now, UpdatedAt: now}
		if member {
			if err := database.Create(&membership).Error; err != nil {
				t.Fatal(err)
			}
		}
		return cpo, membership
	}
	a, memberA := createCPO(true)
	b, _ := createCPO(true)
	foreign, _ := createCPO(false)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/auth"), service)
	router.GET("/protected", service.Authenticate(), RequireCPOAppID(), RequireCPOPermission(database, cpopermissions.HubsRead), func(c *gin.Context) { c.Status(204) })
	call := func(method, path string, body any, appID, token string) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if appID != "" {
			req.Header.Set(CPOAppIDHeader, appID)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	login := func(cpo models.CPO) ChallengeResponse {
		response := call("POST", "/auth/login", LoginRequest{Email: email, Password: password, Scope: constants.AuthScopeCPO}, cpo.AppID, "")
		if response.Code != 202 {
			t.Fatalf("login status=%d", response.Code)
		}
		var challenge ChallengeResponse
		if err := json.Unmarshal(response.Body.Bytes(), &challenge); err != nil {
			t.Fatal(err)
		}
		var stored models.AuthChallenge
		if err := database.First(&stored, "id = ?", challenge.ChallengeID).Error; err != nil || stored.CPOID == nil || *stored.CPOID != cpo.ID {
			t.Fatalf("challenge not bound to selected CPO: %v", err)
		}
		return challenge
	}
	verify := func(challenge ChallengeResponse, header string, status int) TokenResponse {
		code := readOTPFromOutbox(t, database, box, email, loginMailTemplate)
		response := call("POST", "/auth/2fa/verify", ChallengeRequest{ChallengeID: challenge.ChallengeID, Code: code}, header, "")
		if response.Code != status {
			t.Fatalf("OTP status=%d want=%d", response.Code, status)
		}
		var tokens TokenResponse
		if status == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &tokens); err != nil {
				t.Fatal(err)
			}
		}
		return tokens
	}
	assertTenant := func(token string, cpoID uuid.UUID) Principal {
		principal, err := service.ValidateAccess(ctx, token)
		if err != nil || principal.CPOID == nil || *principal.CPOID != cpoID {
			t.Fatalf("session tenant changed: %v", err)
		}
		return principal
	}
	// One identity, two memberships: no arbitrary first-membership selection.
	for _, cpo := range []models.CPO{a, b} {
		tokens := verify(login(cpo), "", 200)
		assertTenant(tokens.AccessToken, cpo.ID)
	}
	// Resend and verify keep A even with B's header. Refresh also keeps A.
	challenge := login(a)
	service.now = func() time.Time { return now.Add(2 * time.Minute) }
	resend := call("POST", "/auth/2fa/resend", ResendRequest{ChallengeID: challenge.ChallengeID}, b.AppID, "")
	if resend.Code != 202 {
		t.Fatalf("resend status=%d", resend.Code)
	}
	var replacement ChallengeResponse
	if err := json.Unmarshal(resend.Body.Bytes(), &replacement); err != nil {
		t.Fatal(err)
	}
	var stored models.AuthChallenge
	if err := database.First(&stored, "id = ?", replacement.ChallengeID).Error; err != nil || stored.CPOID == nil || *stored.CPOID != a.ID {
		t.Fatalf("resend switched CPO: %v", err)
	}
	tokens := verify(replacement, b.AppID, 200)
	assertTenant(tokens.AccessToken, a.ID)
	refresh := call("POST", "/auth/refresh", RefreshRequest{RefreshToken: tokens.RefreshToken}, b.AppID, "")
	if refresh.Code != 200 {
		t.Fatalf("refresh status=%d", refresh.Code)
	}
	if err := json.Unmarshal(refresh.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	principal := assertTenant(tokens.AccessToken, a.ID)
	if response := call("GET", "/protected", nil, a.AppID, tokens.AccessToken); response.Code != 204 {
		t.Fatalf("active access status=%d", response.Code)
	}
	// Fresh permission overrides, role and membership state outrank token claims.
	override := models.CPOMembershipPermissionOverride{MembershipID: memberA.ID, Permission: cpopermissions.HubsRead, Effect: "DENY", CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&override).Error; err != nil {
		t.Fatal(err)
	}
	if response := call("GET", "/protected", nil, a.AppID, tokens.AccessToken); response.Code != 403 {
		t.Fatalf("fresh DENY not applied: %d", response.Code)
	}
	update := func(model any, id uuid.UUID, field string, value any) {
		if err := database.Model(model).Where("id = ?", id).Update(field, value).Error; err != nil {
			t.Fatal(err)
		}
	}
	update(&models.CPOMembership{}, memberA.ID, "role", constants.CPORoleViewer)
	access, allowed, err := EvaluateCPOPermission(ctx, database, principal, cpopermissions.HubsManage)
	if err != nil || allowed || access.Membership.Role != constants.CPORoleViewer {
		t.Fatalf("role change was not freshly evaluated: %v", err)
	}
	update(&models.CPOMembership{}, memberA.ID, "role", constants.CPORoleAdmin)
	update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusSuspended)
	if _, err := service.ValidateAccess(ctx, tokens.AccessToken); err == nil {
		t.Fatal("suspended membership retained access")
	}
	update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusActive)

	// Credential/context failures share exactly the same public response.
	base := LoginRequest{Email: email, Password: password, Scope: constants.AuthScopeCPO}
	var generic string
	for _, scenario := range []string{"unknown App ID", "foreign membership", "inactive CPO", "inactive membership", "bad password", "unknown user"} {
		t.Run(scenario, func(t *testing.T) {
			request, appID := base, a.AppID
			switch scenario {
			case "unknown App ID":
				appID = "unknown_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			case "foreign membership":
				appID = foreign.AppID
			case "inactive CPO":
				update(&models.CPO{}, a.ID, "status", constants.CPOStatusSuspended)
				defer update(&models.CPO{}, a.ID, "status", constants.CPOStatusActive)
			case "inactive membership":
				update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusSuspended)
				defer update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusActive)
			case "bad password":
				request.Password = "wrong-password"
			case "unknown user":
				request.Email = "absent-" + uuid.NewString() + "@example.test"
			}
			response := call("POST", "/auth/login", request, appID, "")
			if response.Code != 401 || !strings.Contains(response.Body.String(), `"code":"invalid_credentials"`) {
				t.Fatalf("failure leaked context: status=%d", response.Code)
			}
			if generic == "" {
				generic = response.Body.String()
			} else if generic != response.Body.String() {
				t.Fatal("credential/context failures have different response bodies")
			}
		})
	}
	for _, target := range []string{"CPO", "membership"} {
		t.Run("deactivated before OTP "+target, func(t *testing.T) {
			challenge := login(a)
			if target == "CPO" {
				update(&models.CPO{}, a.ID, "status", constants.CPOStatusSuspended)
				defer update(&models.CPO{}, a.ID, "status", constants.CPOStatusActive)
			} else {
				update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusSuspended)
				defer update(&models.CPOMembership{}, memberA.ID, "status", constants.MembershipStatusActive)
			}
			verify(challenge, b.AppID, 401)
		})
	}
	// Database errors at the App-ID boundary must remain internal failures.
	if err := database.Callback().Query().Before("gorm:query").Register("test:fail_cpo_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "cpos" {
			tx.AddError(errors.New("injected CPO lookup infrastructure failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	response := call("POST", "/auth/login", base, a.AppID, "")
	if err := database.Callback().Query().Remove("test:fail_cpo_lookup"); err != nil {
		t.Fatal(err)
	}
	if response.Code != 500 || !strings.Contains(response.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("infrastructure error disguised as credentials: %d", response.Code)
	}
	platform := call("POST", "/auth/login", LoginRequest{Email: email, Password: password, Scope: constants.AuthScopePlatform}, "", "")
	if platform.Code != 202 {
		t.Fatalf("platform regression: %d", platform.Code)
	}
}
