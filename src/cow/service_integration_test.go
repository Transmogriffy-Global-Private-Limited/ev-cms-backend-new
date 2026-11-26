package cow

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// Uses the same explicit disposable TEST_DATABASE_URL gate as existing CMS
// integration tests. Never runs schema changes against the normal application DB.
func TestCoWEntitlementLifecyclePostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mkCPO := func(gstin string) models.CPO {
		id := uuid.New()
		cpo := models.CPO{
			ID: id, Slug: "cow-" + strings.ToLower(id.String()),
			BusinessName: "CoW Auth Test", CompanyType: constants.CPOCompanyTypeCompany,
			GSTIN: gstin, Address: "1 Test Road", City: "Kolkata",
			State: constants.WestBengal, Pincode: "700001", Status: constants.CPOStatusActive,
			StatusReason: "test", StatusChangedAt: now,
			AppID:     "cpo_dummy_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := gormDB.Create(&cpo).Error; err != nil {
			t.Fatal(err)
		}
		return cpo
	}
	cpo := mkCPO("19ABCDE1234F1ZX")
	other := mkCPO("19ABCDE1235F1ZW")
	mkUser := func() models.User {
		id := uuid.New()
		user := models.User{
			ID: id, Email: id.String() + "@example.com", PasswordHash: "not-used",
			FullName: "CoW Test User", IsActive: true, IsVerified: true,
			PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := gormDB.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		return user
	}
	admin := mkUser()
	if err := gormDB.Create(&models.PlatformAdmin{
		UserID: admin.ID, IsActive: true, StatusReason: "test", StatusChangedAt: now,
		CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	platform := auth.Principal{Scope: constants.AuthScopePlatform, UserID: admin.ID}
	owner := mkUser()
	membership := models.CPOMembership{
		ID: uuid.New(), CPOID: cpo.ID, UserID: owner.ID, Role: constants.CPORoleOwner,
		Status: constants.MembershipStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := gormDB.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{Scope: constants.AuthScopeCPO, UserID: owner.ID, CPOID: &cpo.ID}

	before, err := GetPlatformEntitlement(ctx, gormDB, platform, cpo.ID)
	if err != nil || before.Enabled || before.Version != 0 {
		t.Fatalf("new CoW must be disabled: %#v %v", before, err)
	}
	if err := Authorize(ctx, gormDB, principal, cpopermissions.CowJobsRead); err == nil {
		t.Fatal("default-off must deny CoW")
	}
	trueValue := true
	falseValue := false
	v0, v1 := int64(0), int64(1)
	changed, err := SetPlatformEntitlement(ctx, gormDB, platform, cpo.ID, ChangeRequest{
		Enabled: &trueValue, ExpectedVersion: &v0, Reason: "Enable initial CoW pilot",
	})
	if err != nil || !changed.Enabled || changed.Version != 1 {
		t.Fatalf("enable failed: %#v %v", changed, err)
	}
	if err := Authorize(ctx, gormDB, principal, cpopermissions.CowJobsRead); err != nil {
		t.Fatalf("eligible owner denied: %v", err)
	}
	if err := Authorize(ctx, gormDB, principal, "cow.*"); err == nil {
		t.Fatal("wildcard must never authorize")
	}
	otherView, err := GetPlatformEntitlement(ctx, gormDB, platform, other.ID)
	if err != nil || otherView.Enabled || otherView.Version != 0 {
		t.Fatalf("enable leaked to another CPO: %#v %v", otherView, err)
	}
	wrongTenant := auth.Principal{Scope: constants.AuthScopeCPO, UserID: owner.ID, CPOID: &other.ID}
	if err := Authorize(ctx, gormDB, wrongTenant, cpopermissions.CowJobsRead); err == nil {
		t.Fatal("cross-CPO membership must deny")
	}
	if err := gormDB.Model(&models.CPOMembership{}).Where("id = ?", membership.ID).
		Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	if err := Authorize(ctx, gormDB, principal, cpopermissions.CowJobsRead); err == nil {
		t.Fatal("suspended membership must deny")
	}
	if err := gormDB.Model(&models.CPOMembership{}).Where("id = ?", membership.ID).
		Update("status", constants.MembershipStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Create(&models.CPOMembershipPermissionOverride{
		ID: uuid.New(), MembershipID: membership.ID, Permission: cpopermissions.CowJobsRead,
		Effect: "DENY", CreatedBy: admin.ID, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Authorize(ctx, gormDB, principal, cpopermissions.CowJobsRead); err == nil {
		t.Fatal("explicit DENY must override owner")
	}
	access, err := GetCPOAccess(ctx, gormDB, principal)
	if err != nil || !access.Eligible {
		t.Fatalf("access projection failed: %#v %v", access, err)
	}
	for _, name := range access.EffectivePermissions {
		if name == cpopermissions.CowJobsRead {
			t.Fatal("DENY permission leaked into effective access")
		}
	}
	if _, err := SetPlatformEntitlement(ctx, gormDB, platform, cpo.ID, ChangeRequest{
		Enabled: &falseValue, ExpectedVersion: &v0, Reason: "Outdated panel",
	}); err == nil {
		t.Fatal("stale admin overwrite was accepted")
	} else {
		var api *auth.APIError
		if !errors.As(err, &api) || api.Status != 409 {
			t.Fatalf("expected version conflict, got %v", err)
		}
	}
	// No-op changes must not add another audit event.
	unchanged, err := SetPlatformEntitlement(ctx, gormDB, platform, cpo.ID, ChangeRequest{
		Enabled: &trueValue, ExpectedVersion: &v1, Reason: "Retry same requested state",
	})
	if err != nil || unchanged.Version != 1 {
		t.Fatalf("idempotent retry failed: %#v %v", unchanged, err)
	}
	var count int64
	if err := gormDB.Table("cpo_module_entitlement_events").Where("cpo_id = ?", cpo.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("unexpected event count %d: %v", count, err)
	}
	disabled, err := SetPlatformEntitlement(ctx, gormDB, platform, cpo.ID, ChangeRequest{
		Enabled: &falseValue, ExpectedVersion: &v1, Reason: "Revoke CoW pilot access",
	})
	if err != nil || disabled.Enabled || disabled.Version != 2 {
		t.Fatalf("revoke failed: %#v %v", disabled, err)
	}
	if err := Authorize(ctx, gormDB, principal, cpopermissions.CowFleetRead); err == nil {
		t.Fatal("revoke must deny new CoW operations")
	}
	if err := gormDB.Table("cpo_module_entitlement_events").Where("cpo_id = ?", cpo.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("event history incomplete: %d %v", count, err)
	}
}
