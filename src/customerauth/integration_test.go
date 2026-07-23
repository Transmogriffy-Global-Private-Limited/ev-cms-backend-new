package customerauth

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

func TestCustomerSignupLifecycleWithPostgreSQL(t *testing.T) {
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
	box, err := security.NewSecretBox("customer-signup-test", []byte(strings.Repeat("m", 32)))
	if err != nil {
		t.Fatalf("create mail encryption: %v", err)
	}
	tokenManager, err := security.NewTokenManager(
		"customer-signup-test", "customer-signup-test-api", 15*time.Minute,
		[]byte(strings.Repeat("s", 32)), []byte(strings.Repeat("e", 32)),
	)
	if err != nil {
		t.Fatalf("create customer token manager: %v", err)
	}
	service, err := NewService(gormDB, config.Auth{
		OTPExpiry: 10 * time.Minute, OTPResendCooldown: time.Minute,
		OTPHMACKey:      []byte(strings.Repeat("o", 32)),
		RateLimitWindow: 15 * time.Minute, RateLimitMax: 100,
		SessionTTL: 24 * time.Hour, LoginMaxAttempts: 5,
		LoginLockDuration: 15 * time.Minute,
	}, true, cmsmail.NewOutbox(box), tokenManager)
	if err != nil {
		t.Fatalf("create customer auth service: %v", err)
	}
	ip := "127.0.0.1"
	metadata := RequestMetadata{IPAddress: &ip, UserAgent: "customer-signup-test"}

	firstCPO := createActiveTestCPO(t, gormDB)
	email := "customer-" + uuid.NewString() + "@example.com"
	password := "CustomerPassword!123"
	challenge, err := service.Start(ctx, firstCPO.AppID, SignupRequest{
		Email: email, Password: password, FullName: "Customer One",
	}, metadata)
	if err != nil {
		t.Fatalf("start signup: %v", err)
	}
	service.now = func() time.Time {
		return challenge.ResendAvailableAt.Add(time.Second)
	}
	replacement, err := service.Resend(ctx, firstCPO.AppID, ResendRequest{
		ChallengeID: challenge.ChallengeID,
	}, metadata)
	if err != nil {
		t.Fatalf("resend signup OTP: %v", err)
	}
	code := readSignupOTP(t, gormDB, box, email)
	if _, err := service.Verify(ctx, firstCPO.AppID, ChallengeRequest{
		ChallengeID: challenge.ChallengeID, Code: code,
	}, metadata); !errors.Is(err, errInvalidChallenge) {
		t.Fatalf("invalidated challenge error=%v, want invalid challenge", err)
	}
	var invalidated models.CustomerSignupChallenge
	if err := gormDB.First(&invalidated, "id = ?", challenge.ChallengeID).Error; err != nil {
		t.Fatalf("load invalidated challenge: %v", err)
	}
	if invalidated.PasswordHash != "INVALIDATED" {
		t.Fatal("resend did not scrub the obsolete password hash")
	}
	created, err := service.Verify(ctx, firstCPO.AppID, ChallengeRequest{
		ChallengeID: replacement.ChallengeID, Code: code,
	}, metadata)
	if err != nil {
		t.Fatalf("verify signup: %v", err)
	}
	if !created.IdentityCreated || created.ExistingPassword {
		t.Fatalf("unexpected new identity flags: %+v", created)
	}
	var user models.User
	if err := gormDB.First(&user, "id = ?", created.UserID).Error; err != nil {
		t.Fatalf("load created identity: %v", err)
	}
	passwordMatches, err := security.VerifyPassword(password, user.PasswordHash)
	if err != nil || !passwordMatches {
		t.Fatalf("created password mismatch: matches=%v err=%v", passwordMatches, err)
	}
	var wallet models.Wallet
	if err := gormDB.First(&wallet, "id = ?", created.WalletID).Error; err != nil {
		t.Fatalf("load created wallet: %v", err)
	}
	if wallet.CPOID != firstCPO.ID || wallet.CustomerID != created.CustomerID ||
		wallet.Currency != "INR" || !wallet.Balance.IsZero() {
		t.Fatalf("unexpected wallet: %+v", wallet)
	}
	if _, err := service.Verify(ctx, firstCPO.AppID, ChallengeRequest{
		ChallengeID: replacement.ChallengeID, Code: code,
	}, metadata); !errors.Is(err, errInvalidChallenge) {
		t.Fatalf("replayed challenge error=%v, want invalid challenge", err)
	}
	var consumed models.CustomerSignupChallenge
	if err := gormDB.First(&consumed, "id = ?", replacement.ChallengeID).Error; err != nil {
		t.Fatalf("load consumed challenge: %v", err)
	}
	if consumed.PasswordHash != "CONSUMED" {
		t.Fatal("verification did not scrub the consumed password hash")
	}

	secondCPO := createActiveTestCPO(t, gormDB)
	secondChallenge, err := service.Start(ctx, secondCPO.AppID, SignupRequest{
		Email: email, Password: "MustNotReplace!456", FullName: "Replacement Name",
	}, metadata)
	if err != nil {
		t.Fatalf("start second CPO signup: %v", err)
	}
	secondCode := readSignupOTP(t, gormDB, box, email)
	reused, err := service.Verify(ctx, secondCPO.AppID, ChallengeRequest{
		ChallengeID: secondChallenge.ChallengeID, Code: secondCode,
	}, metadata)
	if err != nil {
		t.Fatalf("verify second CPO signup: %v", err)
	}
	if reused.IdentityCreated || !reused.ExistingPassword || reused.UserID != created.UserID {
		t.Fatalf("unexpected identity reuse: %+v", reused)
	}
	var retained models.User
	if err := gormDB.First(&retained, "id = ?", created.UserID).Error; err != nil {
		t.Fatalf("reload reused identity: %v", err)
	}
	retainedPassword, _ := security.VerifyPassword(password, retained.PasswordHash)
	replacementPassword, _ := security.VerifyPassword("MustNotReplace!456", retained.PasswordHash)
	if !retainedPassword || replacementPassword || retained.FullName != "Customer One" {
		t.Fatal("identity reuse overwrote global credentials or profile")
	}
}

func TestCustomerAuthenticationLifecycleWithPostgreSQL(t *testing.T) {
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
	box, err := security.NewSecretBox(
		"customer-auth-test", []byte(strings.Repeat("m", 32)),
	)
	if err != nil {
		t.Fatalf("create mail encryption: %v", err)
	}
	tokenManager, err := security.NewTokenManager(
		"customer-auth-test", "customer-auth-test-api", 15*time.Minute,
		[]byte(strings.Repeat("s", 32)), []byte(strings.Repeat("e", 32)),
	)
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	service, err := NewService(gormDB, config.Auth{
		OTPExpiry: 10 * time.Minute, OTPResendCooldown: time.Minute,
		OTPHMACKey: []byte(strings.Repeat("o", 32)), SessionTTL: 24 * time.Hour,
		LoginMaxAttempts: 5, LoginLockDuration: 15 * time.Minute,
		RateLimitWindow: 15 * time.Minute, RateLimitMax: 100,
	}, true, cmsmail.NewOutbox(box), tokenManager)
	if err != nil {
		t.Fatalf("create customer auth service: %v", err)
	}
	cpo := createActiveTestCPO(t, gormDB)
	password := "CustomerLogin!123"
	email := "login-" + uuid.NewString() + "@example.com"
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("hash customer password: %v", err)
	}
	now := time.Now().UTC()
	user := models.User{
		ID: uuid.New(), Email: email, PasswordHash: passwordHash,
		FullName: "Authenticated Customer", IsActive: true, IsVerified: true,
		PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&user).Error; err != nil {
		t.Fatalf("create customer identity: %v", err)
	}
	customer := models.Customer{
		ID: uuid.New(), CPOID: cpo.ID, UserID: user.ID,
		Status: constants.CustomerStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&customer).Error; err != nil {
		t.Fatalf("create customer relationship: %v", err)
	}
	wallet := models.Wallet{
		ID: uuid.New(), CPOID: cpo.ID, CustomerID: customer.ID,
		Currency: "INR", CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&wallet).Error; err != nil {
		t.Fatalf("create customer wallet: %v", err)
	}
	otherUser := models.User{
		ID: uuid.New(), Email: "other-" + uuid.NewString() + "@example.com",
		PasswordHash: passwordHash, FullName: "Other Identity",
		IsActive: true, IsVerified: true, PasswordChangedAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&otherUser).Error; err != nil {
		t.Fatalf("create other identity: %v", err)
	}
	customerID := customer.ID
	cpoID := cpo.ID
	mismatchedSession := models.AuthSession{
		ID: uuid.New(), UserID: otherUser.ID, Scope: constants.AuthScopeCustomer,
		CPOID: &cpoID, CustomerID: &customerID, TokenVersion: 1,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := gormDB.Create(&mismatchedSession).Error; err == nil {
		t.Fatal("database accepted a customer session for the wrong global identity")
	}
	ip := "127.0.0.1"
	metadata := RequestMetadata{IPAddress: &ip, UserAgent: "customer-auth-test"}

	first := loginCustomer(t, ctx, gormDB, box, service, cpo.AppID, email, password, metadata)
	principal, err := service.ValidateAccess(ctx, first.AccessToken)
	if err != nil {
		t.Fatalf("validate customer access token: %v", err)
	}
	if principal.UserID != user.ID || principal.CustomerID != customer.ID ||
		principal.CPOID != cpo.ID || service.Me(principal).Wallet.ID != wallet.ID {
		t.Fatalf("unexpected customer principal: %+v", principal)
	}
	var storedSession models.AuthSession
	if err := gormDB.First(&storedSession, "id = ?", principal.SessionID).Error; err != nil {
		t.Fatalf("load customer session: %v", err)
	}
	if storedSession.Scope != constants.AuthScopeCustomer ||
		storedSession.CustomerID == nil || *storedSession.CustomerID != customer.ID ||
		storedSession.Role != nil {
		t.Fatalf("invalid persisted customer session context: %+v", storedSession)
	}

	rotated, err := service.Refresh(ctx, cpo.AppID, RefreshRequest{
		RefreshToken: first.RefreshToken,
	}, metadata)
	if err != nil {
		t.Fatalf("rotate customer refresh token: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, rotated.AccessToken); err != nil {
		t.Fatalf("validate rotated customer access: %v", err)
	}
	if _, err := service.Refresh(ctx, cpo.AppID, RefreshRequest{
		RefreshToken: first.RefreshToken,
	}, metadata); !errors.Is(err, errInvalidRefresh) {
		t.Fatalf("refresh reuse error=%v, want invalid refresh", err)
	}
	if _, err := service.ValidateAccess(ctx, rotated.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("reused refresh did not revoke session: %v", err)
	}

	second := loginCustomer(t, ctx, gormDB, box, service, cpo.AppID, email, password, metadata)
	secondPrincipal, err := service.ValidateAccess(ctx, second.AccessToken)
	if err != nil {
		t.Fatalf("validate second customer access: %v", err)
	}
	third := loginCustomer(t, ctx, gormDB, box, service, cpo.AppID, email, password, metadata)
	thirdPrincipal, err := service.ValidateAccess(ctx, third.AccessToken)
	if err != nil {
		t.Fatalf("validate third customer access: %v", err)
	}
	sessions, err := service.ListSessions(ctx, secondPrincipal)
	if err != nil {
		t.Fatalf("list customer sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d active customer sessions, want 2", len(sessions))
	}
	if err := service.RevokeSession(ctx, secondPrincipal, thirdPrincipal.SessionID); err != nil {
		t.Fatalf("revoke owned customer session: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, third.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("revoked customer session remains valid: %v", err)
	}

	if err := service.ForgotPassword(ctx, cpo.AppID, ForgotPasswordRequest{
		Email: email,
	}, metadata); err != nil {
		t.Fatalf("start customer password recovery: %v", err)
	}
	resetCode := readCustomerOTP(t, gormDB, box, email, customerResetMailTemplate)
	var resetChallenge models.AuthChallenge
	if err := gormDB.Where(
		"user_id = ? AND purpose = ?", user.ID, constants.ChallengeCustomerReset,
	).Order("created_at DESC").First(&resetChallenge).Error; err != nil {
		t.Fatalf("load customer reset challenge: %v", err)
	}
	resetPassword := "CustomerReset!456"
	if err := service.ResetPassword(ctx, cpo.AppID, ResetPasswordRequest{
		ChallengeID: resetChallenge.ID, Code: resetCode, NewPassword: resetPassword,
	}, metadata); err != nil {
		t.Fatalf("reset customer password: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, second.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("password reset did not revoke customer session: %v", err)
	}
	afterReset := loginCustomer(
		t, ctx, gormDB, box, service, cpo.AppID, email, resetPassword, metadata,
	)
	afterResetPrincipal, err := service.ValidateAccess(ctx, afterReset.AccessToken)
	if err != nil {
		t.Fatalf("validate post-reset access: %v", err)
	}
	changedPassword := "CustomerChanged!789"
	if err := service.ChangePassword(ctx, afterResetPrincipal, ChangePasswordRequest{
		CurrentPassword: resetPassword, NewPassword: changedPassword,
	}); err != nil {
		t.Fatalf("change customer password: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, afterReset.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("password change did not revoke customer session: %v", err)
	}
	final := loginCustomer(
		t, ctx, gormDB, box, service, cpo.AppID, email, changedPassword, metadata,
	)
	finalPrincipal, err := service.ValidateAccess(ctx, final.AccessToken)
	if err != nil {
		t.Fatalf("validate final customer access: %v", err)
	}
	platformSession := models.AuthSession{
		ID: uuid.New(), UserID: user.ID, Scope: constants.AuthScopePlatform,
		TokenVersion: 1, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	if err := gormDB.Create(&platformSession).Error; err != nil {
		t.Fatalf("create unrelated platform session: %v", err)
	}
	if err := service.LogoutAll(ctx, finalPrincipal); err != nil {
		t.Fatalf("logout all customer sessions: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, final.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("customer logout-all left access valid: %v", err)
	}
	if err := gormDB.First(&platformSession, "id = ?", platformSession.ID).Error; err != nil {
		t.Fatalf("reload unrelated platform session: %v", err)
	}
	if platformSession.RevokedAt != nil {
		t.Fatal("customer logout-all revoked an administrative session")
	}
}

func loginCustomer(
	t *testing.T,
	ctx context.Context,
	database *gorm.DB,
	box *security.SecretBox,
	service *Service,
	appID string,
	email string,
	password string,
	metadata RequestMetadata,
) TokenResponse {
	t.Helper()
	challenge, err := service.Login(ctx, appID, LoginRequest{
		Email: email, Password: password,
	}, metadata)
	if err != nil {
		t.Fatalf("start customer login: %v", err)
	}
	code := readCustomerOTP(t, database, box, email, customerLoginMailTemplate)
	response, err := service.VerifyLogin(ctx, appID, ChallengeRequest{
		ChallengeID: challenge.ChallengeID, Code: code,
	}, metadata)
	if err != nil {
		t.Fatalf("verify customer login: %v", err)
	}
	return response
}

func readCustomerOTP(
	t *testing.T,
	database *gorm.DB,
	box *security.SecretBox,
	email string,
	template string,
) string {
	t.Helper()
	var job models.MailOutbox
	if err := database.Where("to_email = ? AND template = ?", email, template).
		Order("created_at DESC").First(&job).Error; err != nil {
		t.Fatalf("load customer OTP mail: %v", err)
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+template+":"+email),
	)
	if err != nil {
		t.Fatalf("decrypt customer OTP mail: %v", err)
	}
	var payload cmsmail.OTPPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode customer OTP mail: %v", err)
	}
	return payload.Code
}

func createActiveTestCPO(t *testing.T, database *gorm.DB) models.CPO {
	t.Helper()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	cpo := models.CPO{
		ID: uuid.New(), Slug: "signup-" + suffix, BusinessName: "Signup Test CPO",
		CompanyType: constants.CPOCompanyTypeCompany, Status: constants.CPOStatusActive,
		AppID: "cpo_dummy_" + suffix, AppIDMode: constants.CPOAppIDModeDummy,
		AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := database.Create(&cpo).Error; err != nil {
		t.Fatalf("create active test CPO: %v", err)
	}
	return cpo
}

func readSignupOTP(
	t *testing.T,
	database *gorm.DB,
	box *security.SecretBox,
	email string,
) string {
	t.Helper()
	var job models.MailOutbox
	if err := database.Where("to_email = ? AND template = ?", email, signupMailTemplate).
		Order("created_at DESC").First(&job).Error; err != nil {
		t.Fatalf("load signup OTP mail: %v", err)
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+signupMailTemplate+":"+email),
	)
	if err != nil {
		t.Fatalf("decrypt signup OTP mail: %v", err)
	}
	var payload cmsmail.OTPPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode signup OTP mail: %v", err)
	}
	return payload.Code
}
