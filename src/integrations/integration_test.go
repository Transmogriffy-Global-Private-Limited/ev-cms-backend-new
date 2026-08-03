package integrations

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
)

func TestCPOIntegrationCredentialIsolationWithPostgreSQL(t *testing.T) {
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

	now := time.Now().UTC()
	user := models.User{
		ID:                uuid.New(),
		Email:             "cpo-" + uuid.NewString() + "@example.com",
		PasswordHash:      "not-used-by-this-test",
		FullName:          "CPO Test Admin",
		IsActive:          true,
		IsVerified:        true,
		MFAEnabled:        true,
		PasswordChangedAt: now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := gormDB.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}
	cpo := models.CPO{
		ID:              uuid.New(),
		Slug:            "cpo-" + uuid.NewString(),
		BusinessName:    "Credential Test CPO",
		CompanyType:     constants.CPOCompanyTypeCompany,
		GSTIN:           strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:15],
		Address:         "1 Test Road",
		City:            "Kolkata",
		State:           "West Bengal",
		Pincode:         "700001",
		Status:          constants.CPOStatusActive,
		StatusReason:    "Integration credential fixture",
		StatusChangedAt: now,
		AppID:           "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		AppIDMode:       constants.CPOAppIDModeDummy,
		AppIDUpdatedAt:  now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := gormDB.Create(&cpo).Error; err != nil {
		t.Fatalf("create test CPO: %v", err)
	}
	role := constants.CPORoleAdmin
	principal := auth.Principal{
		UserID: user.ID,
		Scope:  constants.AuthScopeCPO,
		CPOID:  &cpo.ID,
		Role:   &role,
	}
	box, err := security.NewSecretBox("test-v1", bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatalf("create integration secret box: %v", err)
	}
	service := NewService(gormDB, box)
	credentials := RazorpayCredentials{
		KeyID:         "rzp_test_12345678",
		KeySecret:     "a-secret-value-that-is-long",
		WebhookSecret: "a-webhook-secret-that-is-long",
	}
	created, err := service.PutRazorpay(
		ctx,
		principal,
		constants.IntegrationProviderRazorpay,
		credentials,
	)
	if err != nil {
		t.Fatalf("put credentials: %v", err)
	}
	if created.DisplayHint != "****5678" {
		t.Fatalf("got display hint %q", created.DisplayHint)
	}

	var stored models.CPOIntegration
	if err := gormDB.
		Where("cpo_id = ? AND provider = ?", cpo.ID, constants.IntegrationProviderRazorpay).
		First(&stored).Error; err != nil {
		t.Fatalf("load stored integration: %v", err)
	}
	if bytes.Contains(stored.CredentialCiphertext, []byte(credentials.KeySecret)) ||
		bytes.Contains(stored.CredentialCiphertext, []byte(credentials.WebhookSecret)) {
		t.Fatal("stored integration ciphertext exposes secret plaintext")
	}
	resolved, err := service.ResolveRazorpay(ctx, cpo.ID)
	if err != nil {
		t.Fatalf("resolve credentials internally: %v", err)
	}
	if resolved != credentials {
		t.Fatalf("resolved credentials differ: %#v", resolved)
	}

	platformPrincipal := auth.Principal{
		UserID: user.ID,
		Scope:  constants.AuthScopePlatform,
	}
	if _, err := service.Get(
		ctx,
		platformPrincipal,
		constants.IntegrationProviderRazorpay,
	); err == nil {
		t.Fatal("platform principal accessed CPO integration metadata")
	}
	if err := service.Delete(
		ctx,
		principal,
		constants.IntegrationProviderRazorpay,
	); err != nil {
		t.Fatalf("delete credentials: %v", err)
	}
	if _, err := service.ResolveRazorpay(ctx, cpo.ID); err == nil {
		t.Fatal("deleted credentials still resolved")
	} else if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context error: %v", err)
	}
}
