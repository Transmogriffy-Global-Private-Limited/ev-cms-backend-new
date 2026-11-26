package customerauth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func activateCowAddressTestCPO(t *testing.T, database *gorm.DB, cpoID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	actor := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", PasswordHash: "not-used", FullName: "CoW Address Test Actor", IsActive: true, IsVerified: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&actor).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO cpo_module_entitlements
       (cpo_id,module_key,enabled,version,reason,changed_by_user_id,changed_at)
       VALUES(?, 'cow',true,1,'Address integration test',?,?)`, cpoID, actor.ID, now).Error; err != nil {
		t.Fatal(err)
	}
}
func TestCowSavedAddressLifecyclePostgres(t *testing.T) {
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
	service := newVehicleTestService(t, database)
	cpo := createActiveTestCPO(t, database)
	otherCPO := createActiveTestCPO(t, database)
	customer := createVehicleTestCustomer(t, database, cpo.ID, "cow-address-"+uuid.NewString()+"@example.com")
	foreign := createVehicleTestCustomer(t, database, otherCPO.ID, "cow-address-other-"+uuid.NewString()+"@example.com")
	p := Principal{CPOID: cpo.ID, CustomerID: customer.ID}
	wrong := Principal{CPOID: otherCPO.ID, CustomerID: foreign.ID}
	if _, err := service.CreateCowAddress(ctx, p, CreateCowAddressRequest{CowAddressFields: validCowFields()}); apiErrorCode(err) != "cow_unavailable" {
		t.Fatalf("default-off must deny: %v", err)
	}
	activateCowAddressTestCPO(t, database, cpo.ID)
	activateCowAddressTestCPO(t, database, otherCPO.ID)
	first, err := service.CreateCowAddress(ctx, p, CreateCowAddressRequest{CowAddressFields: validCowFields()})
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsDefault || first.Version != 1 || first.CountryCode != "IN" {
		t.Fatalf("first should be default: %+v", first)
	}
	secondFields := validCowFields()
	secondFields.Label = "Office"
	secondFields.Line1 = "1 Office Lane"
	second, err := service.CreateCowAddress(ctx, p, CreateCowAddressRequest{CowAddressFields: secondFields, IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	if !second.IsDefault {
		t.Fatal("selected new default not committed")
	}
	reloaded, err := service.GetCowAddress(ctx, p, first.ID)
	if err != nil || reloaded.IsDefault || reloaded.Version != 2 {
		t.Fatalf("old default should be updated: %+v %v", reloaded, err)
	}
	if _, err := service.GetCowAddress(ctx, wrong, first.ID); apiErrorCode(err) != "address_not_found" {
		t.Fatalf("cross-CPO address exposed: %v", err)
	}
	page, err := service.ListCowAddresses(ctx, p, 1, nil, nil)
	if err != nil || len(page.Addresses) != 1 || !page.HasMore || page.NextBefore == nil {
		t.Fatalf("first list page: %+v %v", page, err)
	}
	next, err := service.ListCowAddresses(ctx, p, 1, page.NextBefore, page.NextBeforeID)
	if err != nil || len(next.Addresses) != 1 || next.HasMore {
		t.Fatalf("next list page: %+v %v", next, err)
	}
	patch := CowAddressPatch{ExpectedVersion: &second.Version, Changes: map[string]json.RawMessage{"landmark": json.RawMessage(`"Near Main Road"`)}}
	changed, err := service.UpdateCowAddress(ctx, p, second.ID, patch)
	if err != nil || changed.Version != 2 || changed.Landmark != "Near Main Road" {
		t.Fatalf("patch: %+v %v", changed, err)
	}
	if _, err := service.UpdateCowAddress(ctx, p, second.ID, patch); apiErrorCode(err) != "address_version_conflict" {
		t.Fatalf("stale patch accepted: %v", err)
	}
	v, err := service.CreateVehicle(ctx, p, CreateVehicleRequest{VehicleNumber: "WB02ABC1234"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.BuildCowServiceContext(ctx, p, CowServiceContextInput{SavedVehicleID: &v.ID, SavedAddressID: &changed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Address.Landmark != "Near Main Road" || snapshot.Address.Source != "SAVED" || snapshot.Vehicle.Source != "SAVED" {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	updatedMake := "Tata"
	if _, err := service.UpdateVehicle(ctx, p, v.ID, UpdateVehicleRequest{VehicleMake: &updatedMake, vehicleMakeSet: true}); err != nil {
		t.Fatal(err)
	}
	if snapshot.Vehicle.VehicleMake != nil {
		t.Fatal("saved vehicle snapshot mutated with later master-data update")
	}
	patch.ExpectedVersion = &changed.Version
	patch.Changes["landmark"] = json.RawMessage(`"Edited location"`)
	if _, err := service.UpdateCowAddress(ctx, p, changed.ID, patch); err != nil {
		t.Fatal(err)
	}
	if snapshot.Address.Landmark != "Near Main Road" {
		t.Fatal("snapshot mutated with master update")
	}
	temporary := validCowFields()
	temporary.Label = "Temporary"
	temporarySnapshot, err := service.BuildCowServiceContext(ctx, p, CowServiceContextInput{TemporaryVehicle: &CreateVehicleRequest{VehicleNumber: "WB02EV0001"}, TemporaryAddress: &temporary})
	if err != nil {
		t.Fatal(err)
	}
	if temporarySnapshot.Vehicle.Source != "TEMPORARY" || temporarySnapshot.Address.SavedAddressID != nil {
		t.Fatalf("temporary snapshot unexpectedly persisted: %+v", temporarySnapshot)
	}
	// The default's version advanced via PATCH. Delete it and deterministically promote first.
	defaultLatest, err := service.GetCowAddress(ctx, p, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteCowAddress(ctx, p, second.ID, defaultLatest.Version); err != nil {
		t.Fatal(err)
	}
	firstAgain, err := service.GetCowAddress(ctx, p, first.ID)
	if err != nil || !firstAgain.IsDefault {
		t.Fatalf("default promotion failed: %+v %v", firstAgain, err)
	}
	if _, err := service.GetCowAddress(ctx, p, second.ID); apiErrorCode(err) != "address_not_found" {
		t.Fatalf("soft deletion exposed: %v", err)
	}
	var defaults int64
	if err := database.Table("cow_customer_addresses").Where("cpo_id=? AND customer_id=? AND is_active=true AND is_default=true", p.CPOID, p.CustomerID).Count(&defaults).Error; err != nil || defaults != 1 {
		t.Fatalf("default invariant: %d %v", defaults, err)
	}
	var count int64
	if err := database.Table("audit_logs").Where("cpo_id=? AND entity='COW_ADDRESS'", p.CPOID).Count(&count).Error; err != nil || count != 5 {
		t.Fatalf("audit count %d error %v", count, err)
	}
}

func TestCowAddressDefaultConcurrencyPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	dbHandle, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	service := newVehicleTestService(t, dbHandle)
	cpo := createActiveTestCPO(t, dbHandle)
	customer := createVehicleTestCustomer(t, dbHandle, cpo.ID, "cow-concurrency-"+uuid.NewString()+"@example.com")
	activateCowAddressTestCPO(t, dbHandle, cpo.ID)
	p := Principal{CPOID: cpo.ID, CustomerID: customer.ID}
	var addresses []CowAddressView
	for i := 0; i < 5; i++ {
		f := validCowFields()
		f.Label = uuid.NewString()
		a, err := service.CreateCowAddress(ctx, p, CreateCowAddressRequest{CowAddressFields: f})
		if err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, a)
	}
	start := make(chan struct{})
	results := make(chan error, len(addresses)-1)
	var wg sync.WaitGroup
	for i := 1; i < len(addresses); i++ {
		wg.Add(1)
		a := addresses[i]
		go func() {
			defer wg.Done()
			<-start
			view, err := service.SetCowAddressDefault(ctx, p, a.ID, a.Version)
			if err == nil && (!view.IsDefault || view.ID != a.ID || view.Version != a.Version+1) {
				err = fmt.Errorf("unexpected committed default view: %+v", view)
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent default request did not succeed: %v", err)
		}
	}
	var defaults int64
	if err := dbHandle.Table("cow_customer_addresses").Where("cpo_id=? AND customer_id=? AND is_active=true AND is_default=true", p.CPOID, p.CustomerID).Count(&defaults).Error; err != nil || defaults != 1 {
		t.Fatalf("concurrent calls left %d defaults, err=%v", defaults, err)
	}
}
