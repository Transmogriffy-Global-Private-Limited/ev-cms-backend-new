package cpo

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func uniqueCPOGSTIN() string {
	return strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:15]
}

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
				GSTIN:        uniqueCPOGSTIN(),
				Address:      "1 Test Road",
				City:         "Kolkata",
				State:        "West Bengal",
				Pincode:      "700001",
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
		GSTIN:        uniqueCPOGSTIN(),
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

	if _, err := service.Activate(
		ctx,
		platformPrincipal,
		created.CPO.ID,
		LifecycleRequest{Reason: "CPO onboarding approved"},
	); err != nil {
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
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
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

	if _, err := service.Suspend(
		ctx,
		platformPrincipal,
		created.CPO.ID,
		LifecycleRequest{Reason: "Access suspended for lifecycle verification"},
	); err != nil {
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

func TestCPOSuperadminDependencyLifecycleWithPostgreSQL(t *testing.T) {
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

	platformEmail := "cpo-control-platform-" + uuid.NewString() + "@example.com"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{
		Email:    platformEmail,
		Password: "PlatformPassword!123",
		FullName: "CPO Control Platform Admin",
	}); err != nil {
		t.Fatalf("seed platform administrator: %v", err)
	}
	var platformUser models.User
	if err := gormDB.First(&platformUser, "email = ?", platformEmail).Error; err != nil {
		t.Fatalf("load platform administrator: %v", err)
	}
	principal := auth.Principal{
		UserID: platformUser.ID,
		Scope:  constants.AuthScopePlatform,
	}
	mailBox, err := security.NewSecretBox(
		"cpo-control-test-v1",
		[]byte(strings.Repeat("r", 32)),
	)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	eventService := platformops.NewService(gormDB, config.Platform{
		EventRetention: 24 * time.Hour,
	})
	service := NewService(
		gormDB,
		cmsmail.NewOutbox(mailBox),
		true,
	).WithPlatformEvents(eventService)

	oldAdminEmail := "primary-old-" + uuid.NewString() + "@example.com"
	created, err := service.Create(ctx, principal, CreateRequest{
		Slug:         "control-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Control Plane Search Target",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
		Admin: InitialAdminRequest{
			Email:    oldAdminEmail,
			FullName: "Original Primary Administrator",
		},
	})
	if err != nil {
		t.Fatalf("create controlled CPO: %v", err)
	}
	if created.CPO.StatusReason != "Initial provisioning" ||
		created.CPO.StatusChangedByUserID == nil ||
		*created.CPO.StatusChangedByUserID != platformUser.ID {
		t.Fatalf("creation omitted lifecycle evidence: %#v", created.CPO)
	}

	availableSlug := "available-" + strings.ToLower(uuid.NewString())
	availability, err := service.CheckSlugAvailability(
		ctx,
		principal,
		"  "+strings.ToUpper(availableSlug)+"  ",
	)
	if err != nil {
		t.Fatalf("check available slug: %v", err)
	}
	if availability.Slug != availableSlug || !availability.Available {
		t.Fatalf("unexpected available slug response: %#v", availability)
	}

	availability, err = service.CheckSlugAvailability(ctx, principal, created.CPO.Slug)
	if err != nil {
		t.Fatalf("check allocated slug: %v", err)
	}
	if availability.Slug != created.CPO.Slug || availability.Available {
		t.Fatalf("unexpected allocated slug response: %#v", availability)
	}

	duplicateSlugRequest := CreateRequest{
		Slug:         strings.ToUpper(created.CPO.Slug),
		BusinessName: "Duplicate Slug CPO",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "3 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700003",
		Admin: InitialAdminRequest{
			Email:    "duplicate-slug-" + uuid.NewString() + "@example.com",
			FullName: "Duplicate Slug Administrator",
		},
	}
	if _, err := service.Create(ctx, principal, duplicateSlugRequest); err == nil {
		t.Fatal("case-insensitive duplicate slug was accepted")
	} else {
		var apiErr *auth.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "cpo_slug_conflict" {
			t.Fatalf("duplicate slug returned %v, want cpo_slug_conflict", err)
		}
	}

	duplicateGSTINRequest := duplicateSlugRequest
	duplicateGSTINRequest.Slug = "duplicate-gstin-" + strings.ToLower(uuid.NewString())
	duplicateGSTINRequest.GSTIN = strings.ToLower(created.CPO.GSTIN)
	duplicateGSTINRequest.Admin.Email = "duplicate-gstin-" + uuid.NewString() + "@example.com"
	duplicateGSTINRequest.Admin.FullName = "Duplicate GSTIN Administrator"
	if _, err := service.Create(ctx, principal, duplicateGSTINRequest); err == nil {
		t.Fatal("case-insensitive duplicate GSTIN was accepted")
	} else {
		var apiErr *auth.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "cpo_gstin_conflict" {
			t.Fatalf("duplicate GSTIN returned %v, want cpo_gstin_conflict", err)
		}
	}

	if err := gormDB.Exec(
		"UPDATE cpos SET address = '' WHERE id = ?",
		created.CPO.ID,
	).Error; err == nil {
		t.Fatal("database accepted a blank CPO address")
	}
	if err := gormDB.Exec(
		"UPDATE cpos SET gstin = NULL WHERE id = ?",
		created.CPO.ID,
	).Error; err == nil {
		t.Fatal("database accepted a null CPO GSTIN")
	}
	var primaryCount int64
	if err := gormDB.Model(&models.CPOMembership{}).
		Where("cpo_id = ? AND is_primary_admin", created.CPO.ID).
		Count(&primaryCount).Error; err != nil {
		t.Fatalf("count primary memberships: %v", err)
	}
	if primaryCount != 1 {
		t.Fatalf("got %d primary memberships, want 1", primaryCount)
	}
	var correlatedWelcome models.MailOutbox
	if err := gormDB.
		Where(
			"cpo_id = ? AND user_id = ? AND template = ?",
			created.CPO.ID,
			created.Admin.UserID,
			"CPO_ADMIN_WELCOME",
		).
		First(&correlatedWelcome).Error; err != nil {
		t.Fatalf("load correlated onboarding mail: %v", err)
	}

	second, err := service.Create(ctx, principal, CreateRequest{
		Slug:         "control-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Control Plane Cursor Sibling",
		CompanyType:  constants.CPOCompanyTypeIndividual,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "2 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700002",
		Admin: InitialAdminRequest{
			Email:    "cursor-admin-" + uuid.NewString() + "@example.com",
			FullName: "Cursor Administrator",
		},
	})
	if err != nil {
		t.Fatalf("create cursor sibling CPO: %v", err)
	}
	page, err := service.List(ctx, principal, ListQuery{
		Search: oldAdminEmail,
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("search CPOs by primary administrator: %v", err)
	}
	if len(page.CPOs) != 1 || page.CPOs[0].ID != created.CPO.ID {
		t.Fatalf("unexpected primary-admin search page: %#v", page)
	}
	page, err = service.List(ctx, principal, ListQuery{Limit: 1})
	if err != nil {
		t.Fatalf("list first cursor page: %v", err)
	}
	if !page.HasMore || page.NextBefore == nil || page.NextBeforeID == nil {
		t.Fatalf("first cursor page omitted continuation: %#v", page)
	}
	nextPage, err := service.List(ctx, principal, ListQuery{
		Before:   page.NextBefore,
		BeforeID: page.NextBeforeID,
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("list second cursor page: %v", err)
	}
	if len(nextPage.CPOs) != 1 || nextPage.CPOs[0].ID == page.CPOs[0].ID {
		t.Fatalf("cursor repeated or omitted a CPO: first=%#v next=%#v", page, nextPage)
	}
	_ = second

	gstin := "19" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:13]
	updated, err := service.UpdateProfile(
		ctx,
		principal,
		created.CPO.ID,
		UpdateProfileRequest{
			BusinessName: "Updated Control Plane CPO",
			CompanyType:  constants.CPOCompanyTypeCompany,
			GSTIN:        gstin,
			Address:      "2 Recovery Road",
			City:         "Kolkata",
			State:        "West Bengal",
			Pincode:      "700001",
		},
	)
	if err != nil {
		t.Fatalf("update CPO profile: %v", err)
	}
	if updated.BusinessName != "Updated Control Plane CPO" ||
		updated.GSTIN != gstin ||
		updated.Slug != created.CPO.Slug {
		t.Fatalf("unexpected profile update: %#v", updated)
	}

	activated, err := service.Activate(
		ctx,
		principal,
		created.CPO.ID,
		LifecycleRequest{Reason: "Approved by onboarding operations"},
	)
	if err != nil {
		t.Fatalf("activate controlled CPO: %v", err)
	}
	if activated.StatusReason != "Approved by onboarding operations" ||
		activated.StatusChangedByUserID == nil ||
		*activated.StatusChangedByUserID != platformUser.ID {
		t.Fatalf("activation omitted reasoned state: %#v", activated)
	}
	var activationEventsBefore int64
	if err := gormDB.Model(&models.PlatformEvent{}).
		Where(
			"event_type = ? AND resource_id = ?",
			"platform.cpo.activated",
			created.CPO.ID.String(),
		).
		Count(&activationEventsBefore).Error; err != nil {
		t.Fatalf("count activation events: %v", err)
	}
	if _, err := service.Activate(
		ctx,
		principal,
		created.CPO.ID,
		LifecycleRequest{Reason: "Duplicate delivery retry"},
	); err != nil {
		t.Fatalf("repeat activation: %v", err)
	}
	var activationEventsAfter int64
	if err := gormDB.Model(&models.PlatformEvent{}).
		Where(
			"event_type = ? AND resource_id = ?",
			"platform.cpo.activated",
			created.CPO.ID.String(),
		).
		Count(&activationEventsAfter).Error; err != nil {
		t.Fatalf("recount activation events: %v", err)
	}
	if activationEventsAfter != activationEventsBefore {
		t.Fatal("idempotent activation emitted another transition event")
	}

	oldSessionID := uuid.New()
	oldRole := constants.CPORoleAdmin
	now := time.Now().UTC()
	if err := gormDB.Create(&models.AuthSession{
		ID:           oldSessionID,
		UserID:       created.Admin.UserID,
		Scope:        constants.AuthScopeCPO,
		CPOID:        &created.CPO.ID,
		Role:         &oldRole,
		TokenVersion: 1,
		UserAgent:    "primary-replacement-test",
		CreatedAt:    now,
		LastSeenAt:   now,
		ExpiresAt:    now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create previous-primary session: %v", err)
	}
	oldRefreshID := uuid.New()
	oldTokenHash := strings.Repeat(
		strings.ReplaceAll(uuid.NewString(), "-", ""),
		2,
	)
	if err := gormDB.Create(&models.AuthRefreshToken{
		ID:        oldRefreshID,
		SessionID: oldSessionID,
		TokenHash: oldTokenHash,
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create previous-primary refresh token: %v", err)
	}

	newAdminEmail := "primary-new-" + uuid.NewString() + "@example.com"
	primary, err := service.SetPrimaryAdmin(
		ctx,
		principal,
		created.CPO.ID,
		PrimaryAdminRequest{
			Email:    newAdminEmail,
			FullName: "Replacement Primary Administrator",
			Reason:   "Original administrator requested replacement",
		},
	)
	if err != nil {
		t.Fatalf("replace primary administrator: %v", err)
	}
	if primary.Email != newAdminEmail ||
		primary.MembershipStatus != constants.MembershipStatusActive ||
		!primary.MustChangePassword {
		t.Fatalf("unexpected replacement primary administrator: %#v", primary)
	}
	if err := gormDB.First(
		&models.AuthSession{},
		"id = ? AND revoked_at IS NOT NULL",
		oldSessionID,
	).Error; err != nil {
		t.Fatalf("previous primary session was not revoked: %v", err)
	}
	if err := gormDB.First(
		&models.AuthRefreshToken{},
		"id = ? AND revoked_at IS NOT NULL",
		oldRefreshID,
	).Error; err != nil {
		t.Fatalf("previous primary refresh token was not revoked: %v", err)
	}
	if err := gormDB.Model(&models.CPOMembership{}).
		Where("cpo_id = ? AND is_primary_admin", created.CPO.ID).
		Count(&primaryCount).Error; err != nil {
		t.Fatalf("recount primary memberships: %v", err)
	}
	if primaryCount != 1 {
		t.Fatalf("replacement left %d primary memberships", primaryCount)
	}
	replacementWelcome := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		newAdminEmail,
		"CPO_ADMIN_WELCOME",
	)
	if replacementWelcome.TemporaryPassword == "" {
		t.Fatal("new replacement identity did not receive a temporary password")
	}

	resent, err := service.ResendPrimaryAdminOnboarding(
		ctx,
		principal,
		created.CPO.ID,
		ReasonRequest{Reason: "Administrator requested onboarding details again"},
	)
	if err != nil {
		t.Fatalf("resend primary onboarding: %v", err)
	}
	if resent.LatestOnboardingDelivery == nil ||
		resent.LatestOnboardingDelivery.Template != "CPO_ONBOARDING_RESENT" {
		t.Fatalf("resend did not expose safe delivery metadata: %#v", resent)
	}
	resentPayload := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		newAdminEmail,
		"CPO_ONBOARDING_RESENT",
	)
	if resentPayload.TemporaryPassword != "" {
		t.Fatal("onboarding resend exposed a password")
	}

	existingEmail := "existing-primary-" + uuid.NewString() + "@example.com"
	existingHash, err := security.HashPassword("ExistingPassword!123")
	if err != nil {
		t.Fatalf("hash existing primary password: %v", err)
	}
	existingUser := models.User{
		ID:                uuid.New(),
		Email:             existingEmail,
		PasswordHash:      existingHash,
		FullName:          "Existing Global Identity",
		IsActive:          true,
		IsVerified:        true,
		PasswordChangedAt: now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := gormDB.Create(&existingUser).Error; err != nil {
		t.Fatalf("create existing replacement identity: %v", err)
	}
	primary, err = service.SetPrimaryAdmin(
		ctx,
		principal,
		created.CPO.ID,
		PrimaryAdminRequest{
			Email:    existingEmail,
			FullName: "Ignored Replacement Name",
			Reason:   "Use an existing verified global identity",
		},
	)
	if err != nil {
		t.Fatalf("assign existing primary administrator: %v", err)
	}
	var existingAfter models.User
	if err := gormDB.First(&existingAfter, "id = ?", existingUser.ID).Error; err != nil {
		t.Fatalf("reload existing replacement identity: %v", err)
	}
	if existingAfter.PasswordHash != existingHash ||
		existingAfter.FullName != existingUser.FullName {
		t.Fatal("primary replacement overwrote an existing global identity")
	}
	assignment := readMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		existingEmail,
		"CPO_MEMBERSHIP_ASSIGNED",
	)
	if assignment.TemporaryPassword != "" {
		t.Fatal("existing replacement identity received a temporary password")
	}

	newRole := constants.CPORoleAdmin
	newCPOSessionID := uuid.New()
	platformSessionID := uuid.New()
	for _, session := range []models.AuthSession{
		{
			ID:           newCPOSessionID,
			UserID:       primary.UserID,
			Scope:        constants.AuthScopeCPO,
			CPOID:        &created.CPO.ID,
			Role:         &newRole,
			TokenVersion: 1,
			UserAgent:    "bulk-revocation-cpo",
			CreatedAt:    now,
			LastSeenAt:   now,
			ExpiresAt:    now.Add(time.Hour),
		},
		{
			ID:           platformSessionID,
			UserID:       primary.UserID,
			Scope:        constants.AuthScopePlatform,
			TokenVersion: 1,
			UserAgent:    "bulk-revocation-platform",
			CreatedAt:    now,
			LastSeenAt:   now,
			ExpiresAt:    now.Add(time.Hour),
		},
	} {
		if err := gormDB.Create(&session).Error; err != nil {
			t.Fatalf("create scoped session: %v", err)
		}
	}
	revokeResult, err := service.RevokeAdministrativeSessions(
		ctx,
		principal,
		created.CPO.ID,
		ReasonRequest{Reason: "Forced sign-in refresh after admin recovery"},
	)
	if err != nil {
		t.Fatalf("revoke administrative sessions: %v", err)
	}
	if revokeResult.RevokedSessions != 1 {
		t.Fatalf("revoked %d administrative sessions, want 1", revokeResult.RevokedSessions)
	}
	var platformSession models.AuthSession
	if err := gormDB.First(&platformSession, "id = ?", platformSessionID).Error; err != nil {
		t.Fatalf("load unaffected platform session: %v", err)
	}
	if platformSession.RevokedAt != nil {
		t.Fatal("CPO administrative revocation touched a platform session")
	}

	type replacementResult struct {
		view PrimaryAdminView
		err  error
	}
	concurrentResults := make(chan replacementResult, 2)
	for index := 0; index < 2; index++ {
		index := index
		go func() {
			view, replaceErr := service.SetPrimaryAdmin(
				ctx,
				principal,
				created.CPO.ID,
				PrimaryAdminRequest{
					Email: fmt.Sprintf(
						"concurrent-primary-%d-%s@example.com",
						index,
						uuid.NewString(),
					),
					FullName: fmt.Sprintf("Concurrent Primary %d", index),
					Reason:   "Concurrent primary administrator recovery test",
				},
			)
			concurrentResults <- replacementResult{view: view, err: replaceErr}
		}()
	}
	for index := 0; index < 2; index++ {
		result := <-concurrentResults
		if result.err != nil {
			t.Fatalf("concurrent primary replacement: %v", result.err)
		}
	}
	if err := gormDB.Model(&models.CPOMembership{}).
		Where("cpo_id = ? AND is_primary_admin", created.CPO.ID).
		Count(&primaryCount).Error; err != nil {
		t.Fatalf("count concurrent replacement primary memberships: %v", err)
	}
	if primaryCount != 1 {
		t.Fatalf("concurrent replacement left %d primary memberships", primaryCount)
	}
	var changedEvents int64
	if err := gormDB.Model(&models.PlatformEvent{}).
		Where(
			"event_type = ? AND resource_id = ?",
			"platform.cpo.primary_admin_changed",
			created.CPO.ID.String(),
		).
		Count(&changedEvents).Error; err != nil {
		t.Fatalf("count primary-admin change events: %v", err)
	}
	if changedEvents < 4 {
		t.Fatalf("got %d canonical primary-admin change events, want at least 4", changedEvents)
	}

	customer := models.Customer{
		ID:                uuid.New(),
		Email:             "suspension-customer-" + uuid.NewString() + "@example.com",
		PasswordHash:      existingHash,
		CPOID:             created.CPO.ID,
		FullName:          "Suspension Customer",
		IsVerified:        true,
		Status:            constants.CustomerStatusActive,
		PasswordChangedAt: now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := gormDB.Create(&customer).Error; err != nil {
		t.Fatalf("create suspension customer: %v", err)
	}
	customerSessionID := uuid.New()
	if err := gormDB.Create(&models.CustomerAuthSession{
		ID:           customerSessionID,
		CPOID:        created.CPO.ID,
		CustomerID:   customer.ID,
		TokenVersion: 1,
		UserAgent:    "suspension-customer",
		CreatedAt:    now,
		LastSeenAt:   now,
		ExpiresAt:    now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create suspension customer session: %v", err)
	}
	if _, err := service.Suspend(
		ctx,
		principal,
		created.CPO.ID,
		LifecycleRequest{Reason: "Suspend all tenant access after recovery test"},
	); err != nil {
		t.Fatalf("suspend CPO after recovery test: %v", err)
	}
	var customerSession models.CustomerAuthSession
	if err := gormDB.First(&customerSession, "id = ?", customerSessionID).Error; err != nil {
		t.Fatalf("load suspended customer session: %v", err)
	}
	if customerSession.RevokedAt == nil {
		t.Fatal("CPO suspension did not revoke a customer session")
	}
	if err := gormDB.First(&platformSession, "id = ?", platformSessionID).Error; err != nil {
		t.Fatalf("reload platform session after suspension: %v", err)
	}
	if platformSession.RevokedAt != nil {
		t.Fatal("CPO suspension touched a platform session")
	}
}

func TestCPOAdminProfileAndNetworkConfigurationWithPostgreSQL(t *testing.T) {
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

	platformEmail := "network-platform-" + uuid.NewString() + "@example.com"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{
		Email:    platformEmail,
		Password: "PlatformPassword!123",
		FullName: "Network Platform Admin",
	}); err != nil {
		t.Fatalf("seed platform administrator: %v", err)
	}
	var platformUser models.User
	if err := gormDB.First(&platformUser, "email = ?", platformEmail).Error; err != nil {
		t.Fatalf("load platform administrator: %v", err)
	}
	platformPrincipal := auth.Principal{
		UserID: platformUser.ID,
		Scope:  constants.AuthScopePlatform,
	}
	mailBox, err := security.NewSecretBox(
		"network-config-test-v1",
		[]byte(strings.Repeat("n", 32)),
	)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	service := NewService(gormDB, cmsmail.NewOutbox(mailBox), true)
	created, err := service.Create(ctx, platformPrincipal, CreateRequest{
		Slug:         "network-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Network Configuration CPO",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
		Admin: InitialAdminRequest{
			Email:    "network-admin-" + uuid.NewString() + "@example.com",
			FullName: "Network Administrator",
		},
	})
	if err != nil {
		t.Fatalf("create CPO: %v", err)
	}
	if _, err := service.Activate(
		ctx,
		platformPrincipal,
		created.CPO.ID,
		LifecycleRequest{Reason: "Approved for network configuration"},
	); err != nil {
		t.Fatalf("activate CPO: %v", err)
	}

	adminRole := constants.CPORoleAdmin
	adminPrincipal := auth.Principal{
		UserID: created.Admin.UserID,
		Scope:  constants.AuthScopeCPO,
		CPOID:  &created.CPO.ID,
		Role:   &adminRole,
	}
	organization, err := service.GetOrganization(ctx, adminPrincipal)
	if err != nil {
		t.Fatalf("get CPO organization: %v", err)
	}
	if organization.ID != created.CPO.ID ||
		organization.BusinessName != created.CPO.BusinessName ||
		organization.AppID != created.CPO.AppID ||
		organization.Status != constants.CPOStatusActive {
		t.Fatalf("unexpected CPO organization: %#v", organization)
	}
	phone := "+919876543210"
	updatedName := "Updated Network Administrator"
	profile, err := service.UpdateAdminProfile(
		ctx,
		adminPrincipal,
		UpdateAdminProfileRequest{FullName: &updatedName, Phone: &phone},
	)
	if err != nil {
		t.Fatalf("update administrator profile: %v", err)
	}
	if profile.FullName != updatedName || profile.Phone == nil || *profile.Phone != phone {
		t.Fatalf("unexpected administrator profile: %#v", profile)
	}
	user, err := service.GetUser(ctx, adminPrincipal, created.Admin.UserID)
	if err != nil {
		t.Fatalf("get CPO-linked administrator: %v", err)
	}
	if user.ID != created.Admin.UserID || user.CPOID != created.CPO.ID ||
		user.Role == nil || *user.Role != constants.CPORoleAdmin {
		t.Fatalf("unexpected CPO user projection: %#v", user)
	}
	if _, err := service.GetUser(ctx, adminPrincipal, uuid.New()); err == nil {
		t.Fatal("unlinked user ID was returned to the CPO")
	} else {
		var apiErr *auth.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "user_not_found" {
			t.Fatalf("get unlinked user: %v, want user_not_found", err)
		}
	}

	latitude := 22.5524
	longitude := 88.3521
	hub, err := service.CreateHub(ctx, adminPrincipal, CreateHubRequest{
		Name:      "Park Street Hub",
		Address:   "12 Park Street, Kolkata",
		Latitude:  &latitude,
		Longitude: &longitude,
	})
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	hubID := hub.ID
	charger, err := service.CreateCharger(ctx, adminPrincipal, CreateChargerRequest{
		HubID:        &hubID,
		Vendor:       "Delta",
		Model:        "DC Wallbox",
		SerialNumber: "SN-" + strings.ToUpper(uuid.NewString()[:8]),
		MaxPowerKW:   25,
		Connectors: []CreateConnectorRequest{{
			ConnectorNumber: 1,
			ConnectorType:   "CCS2",
			MaxCurrent:      60,
			MaxVoltage:      500,
		}},
	})
	if err != nil {
		t.Fatalf("create charger: %v", err)
	}
	if !chargerIDPattern.MatchString(charger.ChargerID) ||
		charger.OCPPIdentity == "" ||
		len(charger.Connectors) != 1 {
		t.Fatalf("server-generated charger identity is incomplete: %#v", charger)
	}

	nine := decimal.RequireFromString("9.00")
	eighteen := decimal.RequireFromString("18.00")
	gst, err := service.CreateGST(ctx, adminPrincipal, CreateGSTRequest{
		Name:     "Standard GST " + uuid.NewString()[:8],
		SGSTRate: &nine,
		CGSTRate: &nine,
		IGSTRate: &eighteen,
	})
	if err != nil {
		t.Fatalf("create GST: %v", err)
	}
	price := decimal.RequireFromString("18.5000")
	tariff, err := service.CreateTariff(ctx, adminPrincipal, CreateTariffRequest{
		HubID:       hub.ID,
		ChargerID:   &charger.ID,
		GSTID:       &gst.ID,
		PricePerKWh: price,
	})
	if err != nil {
		t.Fatalf("create tariff: %v", err)
	}
	if tariff.Currency != "INR" {
		t.Fatalf("tariff currency is %q, want INR", tariff.Currency)
	}
	hubPage, err := service.ListHubs(ctx, adminPrincipal, TenantListQuery{Limit: 1})
	if err != nil || len(hubPage.Hubs) != 1 || hubPage.Hubs[0].ID != hub.ID {
		t.Fatalf("unexpected hub page %#v: %v", hubPage, err)
	}
	page, err := service.ListChargers(
		ctx,
		adminPrincipal,
		TenantListQuery{Limit: 1},
	)
	if err != nil {
		t.Fatalf("list chargers: %v", err)
	}
	if len(page.Chargers) != 1 || page.Chargers[0].CPOID != created.CPO.ID {
		t.Fatalf("unexpected charger page: %#v", page)
	}
	gstPage, err := service.ListGSTs(ctx, adminPrincipal, TenantListQuery{Limit: 1})
	if err != nil || len(gstPage.GSTs) != 1 || gstPage.GSTs[0].ID != gst.ID {
		t.Fatalf("unexpected GST page %#v: %v", gstPage, err)
	}
	tariffPage, err := service.ListTariffs(ctx, adminPrincipal, TenantListQuery{Limit: 1})
	if err != nil || len(tariffPage.Tariffs) != 1 || tariffPage.Tariffs[0].ID != tariff.ID {
		t.Fatalf("unexpected tariff page %#v: %v", tariffPage, err)
	}

	err = service.DeleteCharger(ctx, adminPrincipal, charger.ChargerID)
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "charger_in_use" {
		t.Fatalf("delete referenced charger got %v, want charger_in_use", err)
	}

	ownerRole := constants.CPORoleOwner
	ownerPrincipal := adminPrincipal
	ownerPrincipal.Role = &ownerRole
	if _, err := service.GetHub(ctx, ownerPrincipal, hub.ID); err == nil {
		t.Fatal("dormant OWNER role was allowed to call a CPO operation")
	}

	var auditCount int64
	if err := gormDB.Model(&models.AuditLog{}).
		Where(
			"cpo_id = ? AND action IN ?",
			created.CPO.ID,
			[]string{
				"CPO_ADMIN_PROFILE_UPDATED",
				"HUB_CREATED",
				"CHARGER_CREATED",
				"GST_CREATED",
				"TARIFF_CREATED",
			},
		).
		Count(&auditCount).Error; err != nil {
		t.Fatalf("count configuration audit records: %v", err)
	}
	if auditCount != 5 {
		t.Fatalf("got %d configuration audit records, want 5", auditCount)
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

func TestAssignChargerToHubTenantScope(t *testing.T) {
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

	platformEmail := "network-platform-" + uuid.NewString() + "@example.com"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{
		Email:    platformEmail,
		Password: "PlatformPassword!123",
		FullName: "Network Platform Admin",
	}); err != nil {
		t.Fatalf("seed platform administrator: %v", err)
	}
	var platformUser models.User
	if err := gormDB.First(&platformUser, "email = ?", platformEmail).Error; err != nil {
		t.Fatalf("load platform administrator: %v", err)
	}
	platformPrincipal := auth.Principal{
		UserID: platformUser.ID,
		Scope:  constants.AuthScopePlatform,
	}
	mailBox, err := security.NewSecretBox(
		"network-config-test-v1",
		[]byte(strings.Repeat("n", 32)),
	)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	service := NewService(gormDB, cmsmail.NewOutbox(mailBox), true)

	// Create CPO 1
	cpo1, err := service.Create(ctx, platformPrincipal, CreateRequest{
		Slug:         "network-1-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Network Configuration CPO 1",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
		Admin: InitialAdminRequest{
			Email:    "network-admin-1-" + uuid.NewString() + "@example.com",
			FullName: "Network Administrator 1",
		},
	})
	if err != nil {
		t.Fatalf("create CPO 1: %v", err)
	}
	if _, err := service.Activate(
		ctx,
		platformPrincipal,
		cpo1.CPO.ID,
		LifecycleRequest{Reason: "Approved for network configuration"},
	); err != nil {
		t.Fatalf("activate CPO 1: %v", err)
	}
	admin1Role := constants.CPORoleAdmin
	admin1Principal := auth.Principal{
		UserID: cpo1.Admin.UserID,
		Scope:  constants.AuthScopeCPO,
		CPOID:  &cpo1.CPO.ID,
		Role:   &admin1Role,
	}

	// Create CPO 2
	cpo2, err := service.Create(ctx, platformPrincipal, CreateRequest{
		Slug:         "network-2-" + strings.ToLower(uuid.NewString()),
		BusinessName: "Network Configuration CPO 2",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        uniqueCPOGSTIN(),
		Address:      "2 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700002",
		Admin: InitialAdminRequest{
			Email:    "network-admin-2-" + uuid.NewString() + "@example.com",
			FullName: "Network Administrator 2",
		},
	})
	if err != nil {
		t.Fatalf("create CPO 2: %v", err)
	}
	if _, err := service.Activate(
		ctx,
		platformPrincipal,
		cpo2.CPO.ID,
		LifecycleRequest{Reason: "Approved for network configuration"},
	); err != nil {
		t.Fatalf("activate CPO 2: %v", err)
	}
	admin2Role := constants.CPORoleAdmin
	admin2Principal := auth.Principal{
		UserID: cpo2.Admin.UserID,
		Scope:  constants.AuthScopeCPO,
		CPOID:  &cpo2.CPO.ID,
		Role:   &admin2Role,
	}

	latitude := 22.5524
	longitude := 88.3521
	hub1, err := service.CreateHub(ctx, admin1Principal, CreateHubRequest{
		Name:      "Park Street Hub",
		Address:   "12 Park Street, Kolkata",
		Latitude:  &latitude,
		Longitude: &longitude,
	})
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	hub1ID := hub1.ID
	charger, err := service.CreateCharger(ctx, admin1Principal, CreateChargerRequest{
		HubID:        &hub1ID,
		Vendor:       "Delta",
		Model:        "DC Wallbox",
		SerialNumber: "SN-" + strings.ToUpper(uuid.NewString()[:8]),
		MaxPowerKW:   25,
		Connectors: []CreateConnectorRequest{{
			ConnectorNumber: 1,
			ConnectorType:   "CCS2",
			MaxCurrent:      60,
			MaxVoltage:      500,
		}},
	})
	if err != nil {
		t.Fatalf("create charger: %v", err)
	}

	hub2, err := service.CreateHub(ctx, admin1Principal, CreateHubRequest{
		Name:      "New Town Hub",
		Address:   "1 New Town, Kolkata",
		Latitude:  &latitude,
		Longitude: &longitude,
	})
	if err != nil {
		t.Fatalf("create hub 2: %v", err)
	}

	assignedCharger, err := service.AssignChargerToHub(ctx, admin1Principal, hub2.ID, charger.ID)
	if err != nil {
		t.Fatalf("assign charger to hub: %v", err)
	}
	if assignedCharger.HubID == nil || *assignedCharger.HubID != hub2.ID {
		t.Fatalf("charger not assigned to correct hub. got %s, want %s", assignedCharger.HubID, hub2.ID)
	}

	idempotentCharger, err := service.AssignChargerToHub(ctx, admin1Principal, hub2.ID, charger.ID)
	if err != nil {
		t.Fatalf("repeat charger assignment: %v", err)
	}
	if idempotentCharger.HubID == nil || *idempotentCharger.HubID != hub2.ID {
		t.Fatalf("repeat assignment changed charger hub. got %s, want %s", idempotentCharger.HubID, hub2.ID)
	}

	independentCharger, err := service.CreateCharger(ctx, admin1Principal, CreateChargerRequest{
		Vendor:       "Delta",
		Model:        "Standalone Wallbox",
		SerialNumber: "SN-" + strings.ToUpper(uuid.NewString()[:8]),
		MaxPowerKW:   7.4,
		Connectors: []CreateConnectorRequest{{
			ConnectorNumber: 1,
			ConnectorType:   "TYPE2",
			MaxCurrent:      32,
			MaxVoltage:      230,
		}},
	})
	if err != nil {
		t.Fatalf("create independent charger: %v", err)
	}
	if independentCharger.HubID != nil {
		t.Fatalf("independent charger unexpectedly has hub %s", *independentCharger.HubID)
	}
	assignedIndependentCharger, err := service.AssignChargerToHub(ctx, admin1Principal, hub1.ID, independentCharger.ID)
	if err != nil {
		t.Fatalf("assign independent charger to hub: %v", err)
	}
	if assignedIndependentCharger.HubID == nil || *assignedIndependentCharger.HubID != hub1.ID {
		t.Fatalf("independent charger not assigned to correct hub: %#v", assignedIndependentCharger.HubID)
	}

	// Try to get the charger with the wrong tenant. This should fail.
	_, err = service.GetCharger(ctx, admin2Principal, charger.ChargerID)
	if err == nil {
		t.Fatalf("should not be able to get charger from another tenant")
	}

	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "charger_not_found" {
		t.Fatalf("expected charger_not_found error, got %v", err)
	}
}
