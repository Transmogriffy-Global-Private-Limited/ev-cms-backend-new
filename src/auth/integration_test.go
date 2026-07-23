package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
	resetCode := readOTPFromOutbox(
		t,
		gormDB,
		mailBox,
		email,
		passwordResetMailTemplate,
	)
	var resetChallenge models.AuthChallenge
	if err := gormDB.
		Where("user_id = ? AND purpose = ?", user.ID, constants.ChallengePasswordReset).
		Order("created_at DESC").
		First(&resetChallenge).Error; err != nil {
		t.Fatalf("load password reset challenge: %v", err)
	}
	newPassword := "ReplacementPassword!789"
	if err := authService.ResetPassword(ctx, ResetPasswordRequest{
		ChallengeID: resetChallenge.ID,
		Code:        resetCode,
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
		ID:             uuid.New(),
		Slug:           "auth-cpo-" + uuid.NewString(),
		BusinessName:   "Authentication Test CPO",
		CompanyType:    constants.CPOCompanyTypeCompany,
		Status:         constants.CPOStatusActive,
		AppID:          "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		AppIDMode:      constants.CPOAppIDModeDummy,
		AppIDUpdatedAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
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
	var payload cmsmail.OTPPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode test mail payload: %v", err)
	}
	if len(payload.Code) != 6 {
		t.Fatalf("got invalid OTP shape %q", payload.Code)
	}
	return payload.Code
}
