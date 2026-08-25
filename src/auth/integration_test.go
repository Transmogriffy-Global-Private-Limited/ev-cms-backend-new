package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAdministrativeChallengeAndPasswordConcurrencyWithPostgreSQL(t *testing.T) {
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

	email := "auth-concurrency-" + uuid.NewString() + "@example.com"
	oldPassword := "InitialPassword!123"
	if err := db.SeedSuperadmin(ctx, gormDB, config.Superadmin{Email: email, Password: oldPassword, FullName: "Concurrency Admin"}); err != nil {
		t.Fatalf("seed administrative identity: %v", err)
	}
	var user models.User
	if err := gormDB.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatalf("load seeded administrative identity: %v", err)
	}
	service, _ := newIntegrationAuthService(t, gormDB)
	ip := "127.0.0.1"
	metadata := RequestMetadata{IPAddress: &ip, UserAgent: "auth-concurrency-test"}

	results := make(chan error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := service.Login(ctx, LoginRequest{Email: email, Password: oldPassword, Scope: constants.AuthScopePlatform}, metadata)
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	for result := range results {
		if result != nil {
			t.Fatalf("concurrent login challenge creation: %v", result)
		}
	}
	var currentChallenges int64
	if err := gormDB.Model(&models.AuthChallenge{}).Where("user_id = ? AND purpose = ? AND consumed_at IS NULL AND invalidated_at IS NULL", user.ID, constants.ChallengeLogin2FA).Count(&currentChallenges).Error; err != nil {
		t.Fatalf("count current login challenges: %v", err)
	}
	if currentChallenges != 1 {
		t.Fatalf("current login challenges = %d, want 1", currentChallenges)
	}

	resetResults := make(chan error, 2)
	resetStart := make(chan struct{})
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-resetStart
			resetResults <- service.ForgotPassword(ctx, ForgotPasswordRequest{Email: email}, metadata)
		}()
	}
	close(resetStart)
	group.Wait()
	close(resetResults)
	for result := range resetResults {
		if result != nil {
			t.Fatalf("concurrent password-reset challenge creation: %v", result)
		}
	}
	var currentResetChallenges int64
	if err := gormDB.Model(&models.AuthChallenge{}).Where("user_id = ? AND purpose = ? AND consumed_at IS NULL AND invalidated_at IS NULL", user.ID, constants.ChallengePasswordReset).Count(&currentResetChallenges).Error; err != nil {
		t.Fatalf("count current password-reset challenges: %v", err)
	}
	if currentResetChallenges != 1 {
		t.Fatalf("current password-reset challenges = %d, want 1", currentResetChallenges)
	}

	passwordResults := make(chan error, 2)
	passwordStart := make(chan struct{})
	for _, replacement := range []string{"ReplacementPassword!456", "ReplacementPassword!789"} {
		replacement := replacement
		group.Add(1)
		go func() {
			defer group.Done()
			<-passwordStart
			passwordResults <- service.ChangePassword(ctx, Principal{UserID: user.ID, Scope: constants.AuthScopePlatform}, ChangePasswordRequest{CurrentPassword: oldPassword, NewPassword: replacement})
		}()
	}
	close(passwordStart)
	group.Wait()
	close(passwordResults)
	succeeded := 0
	for result := range passwordResults {
		if result == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent password changes succeeded %d times, want exactly one", succeeded)
	}
	var changes int64
	if err := gormDB.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", user.ID, "AUTH_PASSWORD_CHANGED").Count(&changes).Error; err != nil {
		t.Fatalf("count committed password audits: %v", err)
	}
	if changes != 1 {
		t.Fatalf("password change audit count = %d, want 1", changes)
	}
}

func TestPlatformAuthenticationLifecycleWithPostgreSQL(t *testing.T) {
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

	email := "auth-" + uuid.NewString() + "@example.com"
	initialPassword := "InitialPassword!123"
	seed := config.Superadmin{
		Email: email, Password: initialPassword, FullName: "Auth Test Admin",
	}
	if err := db.SeedSuperadmin(ctx, gormDB, seed); err != nil {
		t.Fatalf("seed first superadmin: %v", err)
	}
	seed.Password = "MustNotOverwrite!456"
	if err := db.SeedSuperadmin(ctx, gormDB, seed); err != nil {
		t.Fatalf("repeat superadmin seed: %v", err)
	}

	var user models.User
	if err := gormDB.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatalf("load seeded superadmin: %v", err)
	}
	matchesInitial, err := security.VerifyPassword(initialPassword, user.PasswordHash)
	if err != nil || !matchesInitial {
		t.Fatalf("initial password was not retained: matches=%v err=%v", matchesInitial, err)
	}
	matchesReplacement, err := security.VerifyPassword(seed.Password, user.PasswordHash)
	if err != nil {
		t.Fatalf("verify non-overwrite password: %v", err)
	}
	if matchesReplacement {
		t.Fatal("repeated seed overwrote the existing password")
	}

	authService, mailBox := newIntegrationAuthService(t, gormDB)
	ip := "127.0.0.1"
	metadata := RequestMetadata{IPAddress: &ip, UserAgent: "auth-integration-test"}
	challenge, err := authService.Login(ctx, LoginRequest{
		Email: email, Password: initialPassword, Scope: constants.AuthScopePlatform,
	}, metadata)
	if err != nil {
		t.Fatalf("start platform login: %v", err)
	}
	code := readOTPFromOutbox(t, gormDB, mailBox, email, loginMailTemplate)
	tokens, err := authService.VerifyLoginChallenge(ctx, ChallengeRequest{
		ChallengeID: challenge.ChallengeID, Code: code,
	}, metadata)
	if err != nil {
		t.Fatalf("verify platform login: %v", err)
	}
	principal, err := authService.ValidateAccess(ctx, tokens.AccessToken)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if principal.UserID != user.ID || principal.Scope != constants.AuthScopePlatform {
		t.Fatalf("unexpected principal: %#v", principal)
	}

	rotated, err := authService.Refresh(ctx, RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}, metadata)
	if err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	if rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if _, err := authService.Refresh(ctx, RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}, metadata); !errors.Is(err, errInvalidRefresh) {
		t.Fatalf("got refresh reuse error %v, want invalid refresh", err)
	}
	if _, err := authService.ValidateAccess(ctx, rotated.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("reused refresh token did not revoke session: %v", err)
	}

	if err := authService.ForgotPassword(ctx, ForgotPasswordRequest{Email: email}, metadata); err != nil {
		t.Fatalf("start password recovery: %v", err)
	}
	resetMail := readOTPMessageFromOutbox(
		t,
		gormDB,
		mailBox,
		email,
		passwordResetMailTemplate,
	)
	resetChallengeID, err := uuid.Parse(resetMail.ChallengeID)
	if err != nil {
		t.Fatalf("parse password-recovery ID from recipient mail: %v", err)
	}
	newPassword := "ReplacementPassword!789"
	if err := authService.ResetPassword(ctx, ResetPasswordRequest{
		ChallengeID: resetChallengeID,
		Code:        resetMail.Code,
		NewPassword: newPassword,
	}, metadata); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if err := gormDB.First(&user, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("reload reset identity: %v", err)
	}
	matchesNew, err := security.VerifyPassword(newPassword, user.PasswordHash)
	if err != nil || !matchesNew {
		t.Fatalf("new password was not stored: matches=%v err=%v", matchesNew, err)
	}

	now := time.Now().UTC()
	cpo := models.CPO{
		ID:              uuid.New(),
		Slug:            "auth-cpo-" + uuid.NewString(),
		BusinessName:    "Authentication Test CPO",
		CompanyType:     constants.CPOCompanyTypeCompany,
		GSTIN:           testsupport.ValidGSTIN("19"),
		Address:         "1 Test Road",
		City:            "Kolkata",
		State:           "West Bengal",
		Pincode:         "700001",
		Status:          constants.CPOStatusActive,
		StatusReason:    "Authentication integration fixture",
		StatusChangedAt: now,
		AppID:           "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		AppIDMode:       constants.CPOAppIDModeDummy,
		AppIDUpdatedAt:  now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := gormDB.Create(&cpo).Error; err != nil {
		t.Fatalf("create login test CPO: %v", err)
	}
	membership := models.CPOMembership{
		ID:        uuid.New(),
		CPOID:     cpo.ID,
		UserID:    user.ID,
		Role:      constants.CPORoleAdmin,
		Status:    constants.MembershipStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := gormDB.Create(&membership).Error; err != nil {
		t.Fatalf("create CPO admin membership: %v", err)
	}
	cpoChallenge, err := authService.Login(ctx, LoginRequest{
		Email: email, Password: newPassword, Scope: constants.AuthScopeCPO, CPOID: &cpo.ID,
	}, metadata)
	if err != nil {
		t.Fatalf("start CPO login: %v", err)
	}
	cpoCode := readOTPFromOutbox(t, gormDB, mailBox, email, loginMailTemplate)
	cpoTokens, err := authService.VerifyLoginChallenge(ctx, ChallengeRequest{
		ChallengeID: cpoChallenge.ChallengeID, Code: cpoCode,
	}, metadata)
	if err != nil {
		t.Fatalf("verify CPO login: %v", err)
	}
	cpoPrincipal, err := authService.ValidateAccess(ctx, cpoTokens.AccessToken)
	if err != nil {
		t.Fatalf("validate CPO access token: %v", err)
	}
	if cpoPrincipal.CPOID == nil ||
		*cpoPrincipal.CPOID != cpo.ID ||
		cpoPrincipal.Role == nil ||
		*cpoPrincipal.Role != constants.CPORoleAdmin ||
		cpoPrincipal.CPOAppID == nil ||
		*cpoPrincipal.CPOAppID != cpo.AppID ||
		cpoTokens.CPOAppID == nil ||
		*cpoTokens.CPOAppID != cpo.AppID {
		t.Fatalf("unexpected CPO principal: %#v", cpoPrincipal)
	}

	if err := gormDB.Model(&models.CPOMembership{}).
		Where("id = ?", membership.ID).
		Update("role", constants.CPORoleOwner).Error; err != nil {
		t.Fatalf("set dormant membership role: %v", err)
	}
	if _, err := authService.ValidateAccess(ctx, cpoTokens.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("dormant role did not invalidate active CPO access: %v", err)
	}
	if _, err := authService.Login(ctx, LoginRequest{
		Email: email, Password: newPassword, Scope: constants.AuthScopeCPO, CPOID: &cpo.ID,
	}, metadata); !errors.Is(err, errInvalidCredentials) {
		t.Fatalf("dormant role was allowed to start CPO login: %v", err)
	}
}

func newIntegrationAuthService(
	t *testing.T,
	database *gorm.DB,
) (*Service, *security.SecretBox) {
	t.Helper()
	signingKey := []byte(strings.Repeat("s", 32))
	encryptionKey := []byte(strings.Repeat("e", 32))
	mailKey := []byte(strings.Repeat("m", 32))
	tokenManager, err := security.NewTokenManager(
		"ev-cms-test",
		"ev-cms-test-api",
		15*time.Minute,
		signingKey,
		encryptionKey,
	)
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	mailBox, err := security.NewSecretBox("test-v1", mailKey)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	service, err := NewService(
		database,
		config.Auth{
			Issuer:            "ev-cms-test",
			Audience:          "ev-cms-test-api",
			AccessTTL:         15 * time.Minute,
			SessionTTL:        24 * time.Hour,
			OTPExpiry:         10 * time.Minute,
			OTPResendCooldown: time.Minute,
			OTPHMACKey:        []byte(strings.Repeat("o", 32)),
			LoginMaxAttempts:  5,
			LoginLockDuration: 15 * time.Minute,
			RateLimitWindow:   15 * time.Minute,
			RateLimitMax:      50,
		},
		true,
		cmsmail.NewOutbox(mailBox),
		tokenManager,
	)
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	return service, mailBox
}

func readOTPFromOutbox(
	t *testing.T,
	database *gorm.DB,
	box *security.SecretBox,
	email string,
	template string,
) string {
	return readOTPMessageFromOutbox(t, database, box, email, template).Code
}

func readOTPMessageFromOutbox(
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
		t.Fatalf("load encrypted mail job: %v", err)
	}
	if json.Valid(job.PayloadCiphertext) {
		t.Fatal("mail job unexpectedly contains plaintext JSON")
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+template+":"+email),
	)
	if err != nil {
		t.Fatalf("decrypt test mail payload: %v", err)
	}
	var payload cmsmail.MessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode test mail payload: %v", err)
	}
	if len(payload.Code) != 6 {
		t.Fatalf("got invalid OTP shape %q", payload.Code)
	}
	return payload
}
