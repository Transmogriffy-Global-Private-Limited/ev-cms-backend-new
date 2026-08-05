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
	code := readSignupOTP(t, gormDB, box, email, firstCPO.ID)
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
	var customer models.Customer
	if err := gormDB.First(&customer, "id = ?", created.CustomerID).Error; err != nil {
		t.Fatalf("load created customer account: %v", err)
	}
	passwordMatches, err := security.VerifyPassword(password, customer.PasswordHash)
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
		Email: email, Password: password, FullName: "Replacement Name",
	}, metadata)
	if err != nil {
		t.Fatalf("start second CPO signup: %v", err)
	}
	secondCode := readSignupOTP(t, gormDB, box, email, secondCPO.ID)
	reused, err := service.Verify(ctx, secondCPO.AppID, ChallengeRequest{
		ChallengeID: secondChallenge.ChallengeID, Code: secondCode,
	}, metadata)
	if err != nil {
		t.Fatalf("verify second CPO signup: %v", err)
	}
	var secondCustomer models.Customer
	if err := gormDB.First(&secondCustomer, "id = ?", reused.CustomerID).Error; err != nil {
		t.Fatalf("load second customer account: %v", err)
	}
	firstPassword, _ := security.VerifyPassword(password, customer.PasswordHash)
	secondPassword, _ := security.VerifyPassword(password, secondCustomer.PasswordHash)
	if !firstPassword || !secondPassword || customer.ID == secondCustomer.ID ||
		customer.CPOID == secondCustomer.CPOID || customer.FullName != "Customer One" ||
		secondCustomer.FullName != "Replacement Name" {
		t.Fatal("CPO-local customer accounts did not retain independent credentials and profiles")
	}
	firstTokens := loginCustomer(t, ctx, gormDB, box, service, firstCPO.AppID, email, password, metadata)
	firstPrincipal, err := service.ValidateAccess(ctx, firstTokens.AccessToken)
	if err != nil || firstPrincipal.CustomerID != customer.ID || firstPrincipal.CPOID != firstCPO.ID {
		t.Fatalf("same-credential first-CPO login resolved wrong account: principal=%+v err=%v", firstPrincipal, err)
	}
	secondTokens := loginCustomer(t, ctx, gormDB, box, service, secondCPO.AppID, email, password, metadata)
	secondPrincipal, err := service.ValidateAccess(ctx, secondTokens.AccessToken)
	if err != nil || secondPrincipal.CustomerID != secondCustomer.ID || secondPrincipal.CPOID != secondCPO.ID {
		t.Fatalf("same-credential second-CPO login resolved wrong account: principal=%+v err=%v", secondPrincipal, err)
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
	customer := models.Customer{
		ID: uuid.New(), Email: email, PasswordHash: passwordHash,
		CPOID: cpo.ID, FullName: "Authenticated Customer", IsVerified: true,
		Status:            constants.CustomerStatusActive,
		PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&customer).Error; err != nil {
		t.Fatalf("create CPO-local customer account: %v", err)
	}
	wallet := models.Wallet{
		ID: uuid.New(), CPOID: cpo.ID, CustomerID: customer.ID,
		Currency: "INR", CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&wallet).Error; err != nil {
		t.Fatalf("create customer wallet: %v", err)
	}
	otherCPO := createActiveTestCPO(t, gormDB)
	mismatchedSession := models.CustomerAuthSession{
		ID: uuid.New(), CPOID: otherCPO.ID, CustomerID: customer.ID, TokenVersion: 1,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := gormDB.Create(&mismatchedSession).Error; err == nil {
		t.Fatal("database accepted a customer session under the wrong CPO")
	}
	ip := "127.0.0.1"
	metadata := RequestMetadata{IPAddress: &ip, UserAgent: "customer-auth-test"}

	first := loginCustomer(t, ctx, gormDB, box, service, cpo.AppID, email, password, metadata)
	principal, err := service.ValidateAccess(ctx, first.AccessToken)
	if err != nil {
		t.Fatalf("validate customer access token: %v", err)
	}
	if principal.UserID != customer.ID || principal.CustomerID != customer.ID ||
		principal.CPOID != cpo.ID || service.Me(principal).Wallet.ID != wallet.ID {
		t.Fatalf("unexpected customer principal: %+v", principal)
	}
	updatedPhone := "+919876543210"
	updatedProfile, err := service.UpdateProfile(ctx, principal, UpdateProfileRequest{
		FullName: "Updated Customer", Phone: &updatedPhone,
	})
	if err != nil {
		t.Fatalf("update customer profile: %v", err)
	}
	if updatedProfile.FullName != "Updated Customer" || updatedProfile.Phone == nil ||
		*updatedProfile.Phone != updatedPhone || updatedProfile.ID != customer.ID {
		t.Fatalf("unexpected updated customer profile: %+v", updatedProfile)
	}
	var storedCustomer models.Customer
	if err := gormDB.First(&storedCustomer, "id = ? AND cpo_id = ?", customer.ID, cpo.ID).Error; err != nil {
		t.Fatalf("reload updated customer profile: %v", err)
	}
	if storedCustomer.FullName != "Updated Customer" || storedCustomer.Phone == nil || *storedCustomer.Phone != updatedPhone {
		t.Fatalf("customer profile update was not persisted: %+v", storedCustomer)
	}
	var profileAudit models.AuditLog
	if err := gormDB.Where(
		"cpo_id = ? AND action = ? AND entity = ? AND entity_id = ?", cpo.ID,
		"CUSTOMER_PROFILE_UPDATED", "CUSTOMER", customer.ID,
	).Order("created_at DESC").First(&profileAudit).Error; err != nil {
		t.Fatalf("load customer profile audit: %v", err)
	}
	changedFields, ok := profileAudit.Details["changed_fields"].([]any)
	if !ok || len(changedFields) != 2 {
		t.Fatalf("unexpected customer profile audit details: %#v", profileAudit.Details)
	}
	var clearRequest UpdateProfileRequest
	if err := json.Unmarshal([]byte(`{"full_name":"Updated Customer","phone":null}`), &clearRequest); err != nil {
		t.Fatalf("decode customer phone clear: %v", err)
	}
	clearedProfile, err := service.UpdateProfile(ctx, principal, clearRequest)
	if err != nil {
		t.Fatalf("clear customer phone: %v", err)
	}
	if clearedProfile.Phone != nil {
		t.Fatalf("customer phone was not cleared: %+v", clearedProfile)
	}
	var storedSession models.CustomerAuthSession
	if err := gormDB.First(&storedSession, "id = ?", principal.SessionID).Error; err != nil {
		t.Fatalf("load customer session: %v", err)
	}
	if storedSession.CustomerID != customer.ID || storedSession.CPOID != cpo.ID {
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
	resetMail := readCustomerOTPMessage(t, gormDB, box, email, customerResetMailTemplate, cpo.ID)
	resetChallengeID, err := uuid.Parse(resetMail.ChallengeID)
	if err != nil {
		t.Fatalf("parse customer recovery ID from recipient mail: %v", err)
	}
	resetPassword := "CustomerReset!456"
	if err := service.ResetPassword(ctx, cpo.AppID, ResetPasswordRequest{
		ChallengeID: resetChallengeID, Code: resetMail.Code, NewPassword: resetPassword,
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
	staleChallenge, err := service.Login(ctx, cpo.AppID, LoginRequest{
		Email: email, Password: resetPassword,
	}, metadata)
	if err != nil {
		t.Fatalf("create pre-change login challenge: %v", err)
	}
	staleCode := readCustomerOTP(t, gormDB, box, email, customerLoginMailTemplate, cpo.ID)
	changedPassword := "CustomerChanged!789"
	if err := service.ChangePassword(ctx, afterResetPrincipal, ChangePasswordRequest{
		CurrentPassword: resetPassword, NewPassword: changedPassword,
	}); err != nil {
		t.Fatalf("change customer password: %v", err)
	}
	if _, err := service.ValidateAccess(ctx, afterReset.AccessToken); !errors.Is(err, errUnauthorized) {
		t.Fatalf("password change did not revoke customer session: %v", err)
	}
	if _, err := service.VerifyLogin(ctx, cpo.AppID, ChallengeRequest{
		ChallengeID: staleChallenge.ChallengeID, Code: staleCode,
	}, metadata); !errors.Is(err, errInvalidChallenge) {
		t.Fatalf("password change left a pre-change login challenge usable: %v", err)
	}
	final := loginCustomer(
		t, ctx, gormDB, box, service, cpo.AppID, email, changedPassword, metadata,
	)
	finalPrincipal, err := service.ValidateAccess(ctx, final.AccessToken)
	if err != nil {
		t.Fatalf("validate final customer access: %v", err)
	}
	adminUser := models.User{
		ID: uuid.New(), Email: "admin-" + uuid.NewString() + "@example.com", PasswordHash: passwordHash,
		FullName: "Unrelated Administrator", IsActive: true, IsVerified: true,
		PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&adminUser).Error; err != nil {
		t.Fatalf("create unrelated administrative identity: %v", err)
	}
	platformSession := models.AuthSession{
		ID: uuid.New(), UserID: adminUser.ID, Scope: constants.AuthScopePlatform,
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
	var cpo models.CPO
	if err := database.Select("id").First(&cpo, "app_id = ?", appID).Error; err != nil {
		t.Fatalf("load customer CPO for mail correlation: %v", err)
	}
	challenge, err := service.Login(ctx, appID, LoginRequest{
		Email: email, Password: password,
	}, metadata)
	if err != nil {
		t.Fatalf("start customer login: %v", err)
	}
	code := readCustomerOTP(t, database, box, email, customerLoginMailTemplate, cpo.ID)
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
	cpoID uuid.UUID,
) string {
	return readCustomerOTPMessage(t, database, box, email, template, cpoID).Code
}

func readCustomerOTPMessage(
	t *testing.T,
	database *gorm.DB,
	box *security.SecretBox,
	email string,
	template string,
	cpoID uuid.UUID,
) cmsmail.MessagePayload {
	t.Helper()
	var job models.MailOutbox
	if err := database.Where("to_email = ? AND template = ?", email, template).
		Order("created_at DESC").First(&job).Error; err != nil {
		t.Fatalf("load customer OTP mail: %v", err)
	}
	if job.CPOID == nil || *job.CPOID != cpoID || job.UserID != nil {
		t.Fatalf("invalid customer mail correlation: cpo_id=%v user_id=%v", job.CPOID, job.UserID)
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+template+":"+email),
	)
	if err != nil {
		t.Fatalf("decrypt customer OTP mail: %v", err)
	}
	var payload cmsmail.MessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode customer OTP mail: %v", err)
	}
	return payload
}

func createActiveTestCPO(t *testing.T, database *gorm.DB) models.CPO {
	t.Helper()
	now := time.Now().UTC()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	cpo := models.CPO{
		ID: uuid.New(), Slug: "signup-" + suffix, BusinessName: "Signup Test CPO",
		CompanyType: constants.CPOCompanyTypeCompany, Status: constants.CPOStatusActive,
		GSTIN: suffix[:15], Address: "1 Test Road", City: "Kolkata",
		State: "West Bengal", Pincode: "700001",
		StatusReason: "Customer authentication fixture", StatusChangedAt: now,
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
	cpoID uuid.UUID,
) string {
	t.Helper()
	var job models.MailOutbox
	if err := database.Where("to_email = ? AND template = ?", email, signupMailTemplate).
		Order("created_at DESC").First(&job).Error; err != nil {
		t.Fatalf("load signup OTP mail: %v", err)
	}
	if job.CPOID == nil || *job.CPOID != cpoID || job.UserID != nil {
		t.Fatalf("invalid signup mail correlation: cpo_id=%v user_id=%v", job.CPOID, job.UserID)
	}
	plaintext, err := box.Open(
		job.PayloadCiphertext,
		[]byte("ev-cms-mail:"+signupMailTemplate+":"+email),
	)
	if err != nil {
		t.Fatalf("decrypt signup OTP mail: %v", err)
	}
	var payload cmsmail.MessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode signup OTP mail: %v", err)
	}
	return payload.Code
}
