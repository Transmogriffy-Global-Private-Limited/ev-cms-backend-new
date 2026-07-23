package cpo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestConcurrentCPOCreationReusesOneAdminIdentityWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	actorEmail := "concurrent-platform-" + uuid.NewString() + "@example.com"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{
		Email:    actorEmail,
		Password: "PlatformPassword!123",
		FullName: "Concurrent Platform Admin",
	}); err != nil {
		t.Fatalf("seed platform administrator: %v", err)
	}
	var actor models.User
	if err := gormDB.Where("email = ?", actorEmail).First(&actor).Error; err != nil {
		t.Fatalf("load platform administrator: %v", err)
	}
	mailBox, err := security.NewSecretBox(
		"concurrent-cpo-test-v1",
		[]byte(strings.Repeat("q", 32)),
	)
	if err != nil {
		t.Fatalf("create mail box: %v", err)
	}
	service := NewService(gormDB, cmsmail.NewOutbox(mailBox), true)
	principal := auth.Principal{
		UserID: actor.ID,
		Scope:  constants.AuthScopePlatform,
	}
	adminEmail := "concurrent-admin-" + uuid.NewString() + "@example.com"
	suffix := strings.ToLower(uuid.NewString())

	type outcome struct {
		response CreateResponse
		err      error
	}
	results := make(chan outcome, 2)
	for index := 0; index < 2; index++ {
		index := index
		go func() {
			response, err := service.Create(ctx, principal, CreateRequest{
				Slug:         fmt.Sprintf("concurrent-%d-%s", index, suffix),
				BusinessName: fmt.Sprintf("Concurrent CPO %d", index),
				CompanyType:  constants.CPOCompanyTypeCompany,
				Admin: InitialAdminRequest{
					Email:    adminEmail,
					FullName: "Concurrent Administrator",
				},
			})
			results <- outcome{response: response, err: err}
		}()
	}

	createdIdentities := 0
	for index := 0; index < 2; index++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent CPO creation failed: %v", result.err)
		}
		if result.response.Admin.IdentityCreated {
			createdIdentities++
		}
	}
	if createdIdentities != 1 {
		t.Fatalf("got %d newly created identities, want exactly 1", createdIdentities)
	}
	var identityCount int64
	if err := gormDB.Model(&models.User{}).
		Where("email = ?", adminEmail).
		Count(&identityCount).Error; err != nil {
		t.Fatalf("count concurrent identities: %v", err)
	}
	if identityCount != 1 {
		t.Fatalf("got %d identities for one email, want 1", identityCount)
	}
	var membershipCount int64
	if err := gormDB.Model(&models.CPOMembership{}).
		Joins("JOIN users ON users.id = cpo_memberships.user_id").
		Where("users.email = ?", adminEmail).
		Count(&membershipCount).Error; err != nil {
		t.Fatalf("count concurrent memberships: %v", err)
	}
	if membershipCount != 2 {
		t.Fatalf("got %d memberships, want 2", membershipCount)
	}
}

func TestCPOProvisioningAndFirstAdminLifecycleWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	platformEmail := "platform-" + uuid.NewString() + "@example.com"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{
		Email:    platformEmail,
		Password: "PlatformPassword!123",
		FullName: "Platform Test Admin",
	}); err != nil {
		t.Fatalf("seed platform administrator: %v", err)
	}
	var platformUser models.User
	if err := gormDB.Where("email = ?", platformEmail).First(&platformUser).Error; err != nil {
		t.Fatalf("load platform administrator: %v", err)
	}
	platformPrincipal := auth.Principal{
		UserID: platformUser.ID,
		Scope:  constants.AuthScopePlatform,
	}

	mailBox, err := security.NewSecretBox(
		"cpo-test-mail-v1",
		[]byte(strings.Repeat("m", 32)),
	)
	if err != nil {
		t.Fatalf("create mail encryption box: %v", err)
	}
	outbox := cmsmail.NewOutbox(mailBox)
	service := NewService(gormDB, outbox, true)

	adminEmail := "admin-" + uuid.NewString() + "@example.com"
	created, err := service.Create(ctx, platformPrincipal, CreateRequest{
		Slug:         "provisioned-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Provisioning Test CPO",
		CompanyType:  constants.CPOCompanyTypeCompany,
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
		Admin: InitialAdminRequest{
			Email:    adminEmail,
			FullName: "Provisioned Admin",
		},
	})
	if err != nil {
		t.Fatalf("create CPO: %v", err)
	}
	if created.CPO.Status != constants.CPOStatusPending ||
		created.CPO.AppIDMode != constants.CPOAppIDModeDummy ||
		!strings.HasPrefix(created.CPO.AppID, dummyAppIDPrefix) ||
		!created.Admin.IdentityCreated ||
		created.Admin.Role != constants.CPORoleAdmin {
		t.Fatalf("unexpected CPO creation response: %#v", created)
	}

	welcome := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		adminEmail,
		"CPO_ADMIN_WELCOME",
	)
	if welcome.TemporaryPassword == "" ||
		welcome.CPOID != created.CPO.ID.String() ||
		welcome.CPOAppID != created.CPO.AppID ||
		welcome.CPOName != created.CPO.BusinessName {
		t.Fatalf("unexpected onboarding message: %#v", welcome)
	}
	var admin models.User
	if err := gormDB.First(&admin, "id = ?", created.Admin.UserID).Error; err != nil {
		t.Fatalf("load initial administrator: %v", err)
	}
	if !admin.MustChangePassword {
		t.Fatal("new CPO administrator was not marked for password change")
	}
	passwordMatches, err := security.VerifyPassword(
		welcome.TemporaryPassword,
		admin.PasswordHash,
	)
	if err != nil || !passwordMatches {
		t.Fatalf("temporary password does not match stored hash: matches=%v err=%v", passwordMatches, err)
	}
	if strings.Contains(admin.PasswordHash, welcome.TemporaryPassword) {
		t.Fatal("temporary password appears in stored password hash")
	}

	authService := newCPOIntegrationAuthService(t, gormDB, outbox)
	metadata := auth.RequestMetadata{UserAgent: "cpo-provisioning-test"}
	if _, err := authService.Login(ctx, auth.LoginRequest{
		Email:    adminEmail,
		Password: welcome.TemporaryPassword,
		Scope:    constants.AuthScopeCPO,
		CPOID:    &created.CPO.ID,
	}, metadata); err == nil {
		t.Fatal("pending CPO administrator was allowed to log in")
	}

	if _, err := service.Activate(ctx, platformPrincipal, created.CPO.ID); err != nil {
		t.Fatalf("activate CPO: %v", err)
	}
	firstChallenge, err := authService.Login(ctx, auth.LoginRequest{
		Email:    adminEmail,
		Password: welcome.TemporaryPassword,
		Scope:    constants.AuthScopeCPO,
		CPOID:    &created.CPO.ID,
	}, metadata)
	if err != nil {
		t.Fatalf("start first CPO login: %v", err)
	}
	firstOTP := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		adminEmail,
		"LOGIN_OTP",
	)
	firstTokens, err := authService.VerifyLoginChallenge(
		ctx,
		auth.ChallengeRequest{
			ChallengeID: firstChallenge.ChallengeID,
			Code:        firstOTP.Code,
		},
		metadata,
	)
	if err != nil {
		t.Fatalf("verify first CPO login: %v", err)
	}
	if !firstTokens.MustChangePassword ||
		firstTokens.CPOAppID == nil ||
		*firstTokens.CPOAppID != created.CPO.AppID {
		t.Fatalf("first login did not return onboarding state: %#v", firstTokens)
	}
	assertAppBoundaryStatus(
		t,
		authService,
		firstTokens.AccessToken,
		created.CPO.AppID,
		http.StatusForbidden,
		"password_change_required",
	)
	var reminderCount int64
	if err := gormDB.Model(&models.MailOutbox{}).
		Where("to_email = ? AND template = ?", adminEmail, "PASSWORD_CHANGE_REMINDER").
		Count(&reminderCount).Error; err != nil {
		t.Fatalf("count password reminders: %v", err)
	}
	if reminderCount != 1 {
		t.Fatalf("got %d password reminders after first login, want 1", reminderCount)
	}

	firstPrincipal, err := authService.ValidateAccess(ctx, firstTokens.AccessToken)
	if err != nil {
		t.Fatalf("validate first CPO token: %v", err)
	}
	firstMe := authService.Me(firstPrincipal)
	if firstMe.CPOAppID == nil ||
		*firstMe.CPOAppID != created.CPO.AppID ||
		!firstMe.User.MustChangePassword {
		t.Fatalf("me did not return current onboarding state: %#v", firstMe)
	}

	reminderChallenge, err := authService.Login(ctx, auth.LoginRequest{
		Email:    adminEmail,
		Password: welcome.TemporaryPassword,
		Scope:    constants.AuthScopeCPO,
		CPOID:    &created.CPO.ID,
	}, metadata)
	if err != nil {
		t.Fatalf("start repeated temporary-password login: %v", err)
	}
	reminderOTP := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		adminEmail,
		"LOGIN_OTP",
	)
	reminderTokens, err := authService.VerifyLoginChallenge(
		ctx,
		auth.ChallengeRequest{
			ChallengeID: reminderChallenge.ChallengeID,
			Code:        reminderOTP.Code,
		},
		metadata,
	)
	if err != nil {
		t.Fatalf("verify repeated temporary-password login: %v", err)
	}
	if err := gormDB.Model(&models.MailOutbox{}).
		Where("to_email = ? AND template = ?", adminEmail, "PASSWORD_CHANGE_REMINDER").
		Count(&reminderCount).Error; err != nil {
		t.Fatalf("recount repeated password reminders: %v", err)
	}
	if reminderCount != 2 {
		t.Fatalf("got %d password reminders after two logins, want 2", reminderCount)
	}

	replacementPassword := "ReplacementPassword!456"
	if err := authService.ChangePassword(ctx, firstPrincipal, auth.ChangePasswordRequest{
		CurrentPassword: welcome.TemporaryPassword,
		NewPassword:     replacementPassword,
	}); err != nil {
		t.Fatalf("change temporary password: %v", err)
	}
	if _, err := authService.ValidateAccess(ctx, firstTokens.AccessToken); err == nil {
		t.Fatal("password change did not revoke the original session")
	}
	if _, err := authService.ValidateAccess(ctx, reminderTokens.AccessToken); err == nil {
		t.Fatal("password change did not revoke the repeated-login session")
	}
	if err := gormDB.First(&admin, "id = ?", admin.ID).Error; err != nil {
		t.Fatalf("reload administrator after password change: %v", err)
	}
	if admin.MustChangePassword {
		t.Fatal("password change did not clear the onboarding flag")
	}

	secondChallenge, err := authService.Login(ctx, auth.LoginRequest{
		Email:    adminEmail,
		Password: replacementPassword,
		Scope:    constants.AuthScopeCPO,
		CPOID:    &created.CPO.ID,
	}, metadata)
	if err != nil {
		t.Fatalf("start second CPO login: %v", err)
	}
	secondOTP := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		adminEmail,
		"LOGIN_OTP",
	)
	secondTokens, err := authService.VerifyLoginChallenge(
		ctx,
		auth.ChallengeRequest{
			ChallengeID: secondChallenge.ChallengeID,
			Code:        secondOTP.Code,
		},
		metadata,
	)
	if err != nil {
		t.Fatalf("verify second CPO login: %v", err)
	}
	if secondTokens.MustChangePassword {
		t.Fatal("normal login retained the password-change requirement")
	}
	var reminderCountAfter int64
	if err := gormDB.Model(&models.MailOutbox{}).
		Where("to_email = ? AND template = ?", adminEmail, "PASSWORD_CHANGE_REMINDER").
		Count(&reminderCountAfter).Error; err != nil {
		t.Fatalf("recount password reminders: %v", err)
	}
	if reminderCountAfter != reminderCount {
		t.Fatal("password reminder was queued after the password had been changed")
	}
	assertAppBoundaryStatus(
		t,
		authService,
		secondTokens.AccessToken,
		created.CPO.AppID,
		http.StatusNoContent,
		"",
	)

	liveAppID := "live_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	updated, err := service.SetLiveAppID(
		ctx,
		platformPrincipal,
		created.CPO.ID,
		SetAppIDRequest{AppID: liveAppID},
	)
	if err != nil {
		t.Fatalf("set live app ID: %v", err)
	}
	if updated.AppIDMode != constants.CPOAppIDModeLive || updated.AppID != liveAppID {
		t.Fatalf("unexpected live app identity: %#v", updated)
	}
	assertAppBoundaryStatus(
		t,
		authService,
		secondTokens.AccessToken,
		created.CPO.AppID,
		http.StatusForbidden,
		"cpo_app_id_mismatch",
	)
	assertAppBoundaryStatus(
		t,
		authService,
		secondTokens.AccessToken,
		liveAppID,
		http.StatusNoContent,
		"",
	)
	livePrincipal, err := authService.ValidateAccess(ctx, secondTokens.AccessToken)
	if err != nil {
		t.Fatalf("validate token after app ID promotion: %v", err)
	}
	liveMe := authService.Me(livePrincipal)
	if liveMe.CPOAppID == nil ||
		*liveMe.CPOAppID != liveAppID ||
		liveMe.CPOAppIDMode == nil ||
		*liveMe.CPOAppIDMode != constants.CPOAppIDModeLive {
		t.Fatalf("me did not return current live app identity: %#v", liveMe)
	}
	refreshed, err := authService.Refresh(ctx, auth.RefreshRequest{
		RefreshToken: secondTokens.RefreshToken,
	}, metadata)
	if err != nil {
		t.Fatalf("refresh after app ID promotion: %v", err)
	}
	if refreshed.CPOAppID == nil ||
		*refreshed.CPOAppID != liveAppID ||
		refreshed.CPOAppIDMode == nil ||
		*refreshed.CPOAppIDMode != constants.CPOAppIDModeLive {
		t.Fatalf("refresh did not return current live app identity: %#v", refreshed)
	}

	passwordHashBeforeReuse := admin.PasswordHash
	reused, err := service.Create(ctx, platformPrincipal, CreateRequest{
		Slug:         "reused-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Existing Identity CPO",
		CompanyType:  constants.CPOCompanyTypeCompany,
		Admin: InitialAdminRequest{
			Email:    adminEmail,
			FullName: "Ignored Existing Name",
		},
	})
	if err != nil {
		t.Fatalf("create CPO with existing identity: %v", err)
	}
	if reused.Admin.IdentityCreated {
		t.Fatal("existing identity was reported as newly created")
	}
	if err := gormDB.First(&admin, "id = ?", admin.ID).Error; err != nil {
		t.Fatalf("reload reused identity: %v", err)
	}
	if admin.PasswordHash != passwordHashBeforeReuse {
		t.Fatal("existing identity password was overwritten")
	}
	assigned := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		adminEmail,
		"CPO_MEMBERSHIP_ASSIGNED",
	)
	if assigned.TemporaryPassword != "" ||
		assigned.CPOID != reused.CPO.ID.String() ||
		assigned.CPOAppID != reused.CPO.AppID {
		t.Fatalf("unexpected existing-membership message: %#v", assigned)
	}

	if _, err := service.Suspend(ctx, platformPrincipal, created.CPO.ID); err != nil {
		t.Fatalf("suspend CPO: %v", err)
	}
	if _, err := authService.ValidateAccess(ctx, refreshed.AccessToken); err == nil {
		t.Fatal("suspension did not invalidate the active CPO session")
	}
	if _, err := authService.Refresh(ctx, auth.RefreshRequest{
		RefreshToken: refreshed.RefreshToken,
	}, metadata); err == nil {
		t.Fatal("suspension did not invalidate the active refresh token")
	}
	if _, err := authService.Login(ctx, auth.LoginRequest{
		Email:    adminEmail,
		Password: replacementPassword,
		Scope:    constants.AuthScopeCPO,
		CPOID:    &created.CPO.ID,
	}, metadata); err == nil {
		t.Fatal("suspended CPO administrator was allowed to log in")
	}
}

func newCPOIntegrationAuthService(
	t *testing.T,
	database *gorm.DB,
	outbox *cmsmail.Outbox,
) *auth.Service {
	t.Helper()
	tokenManager, err := security.NewTokenManager(
		"cpo-integration-test",
		"cpo-integration-api",
		15*time.Minute,
		[]byte(strings.Repeat("s", 32)),
		[]byte(strings.Repeat("e", 32)),
	)
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	service, err := auth.NewService(
		database,
		config.Auth{
			Issuer:            "cpo-integration-test",
			Audience:          "cpo-integration-api",
			AccessTTL:         15 * time.Minute,
			SessionTTL:        24 * time.Hour,
			OTPExpiry:         10 * time.Minute,
			OTPResendCooldown: time.Minute,
			OTPHMACKey:        []byte(strings.Repeat("o", 32)),
			LoginMaxAttempts:  5,
			LoginLockDuration: 15 * time.Minute,
			RateLimitWindow:   15 * time.Minute,
			RateLimitMax:      100,
		},
		true,
		outbox,
		tokenManager,
	)
	if err != nil {
		t.Fatalf("create authentication service: %v", err)
	}
	return service
}

func readMessageFromOutbox(
	t *testing.T,
	database *gorm.DB,
	box *security.SecretBox,
	email string,
	template string,
) cmsmail.MessagePayload {
	t.Helper()
	var job models.MailOutbox
	if err := database.
		Where("to_email = ? AND template = ?", email, template).
		Order("created_at DESC").
		First(&job).Error; err != nil {
		t.Fatalf("load %s mail job: %v", template, err)
	}
	if json.Valid(job.PayloadCiphertext) {
		t.Fatalf("%s mail job unexpectedly contains plaintext JSON", template)
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+template+":"+strings.ToLower(strings.TrimSpace(email))),
	)
	if err != nil {
		t.Fatalf("decrypt %s mail payload: %v", template, err)
	}
	var payload cmsmail.MessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode %s mail payload: %v", template, err)
	}
	return payload
}

func assertAppBoundaryStatus(
	t *testing.T,
	authService *auth.Service,
	accessToken string,
	appID string,
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/business",
		authService.Authenticate(),
		auth.RequireCPOAppID(),
		func(ctx *gin.Context) {
			ctx.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(http.MethodGet, "/business", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set(auth.CPOAppIDHeader, appID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf(
			"app boundary got status %d, want %d: %s",
			recorder.Code,
			wantStatus,
			recorder.Body.String(),
		)
	}
	if wantCode != "" && !strings.Contains(
		recorder.Body.String(),
		`"code":"`+wantCode+`"`,
	) {
		t.Fatalf("response %s does not contain error code %q", recorder.Body.String(), wantCode)
	}
}
