package customerauth

import (
	"context"
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

func TestCustomerVehicleCRUDWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	service := newVehicleTestService(t, database)
	cpo := createActiveTestCPO(t, database)
	otherCPO := createActiveTestCPO(t, database)
	first := createVehicleTestCustomer(t, database, cpo.ID, "vehicle-first-"+uuid.NewString()+"@example.com")
	second := createVehicleTestCustomer(t, database, cpo.ID, "vehicle-second-"+uuid.NewString()+"@example.com")
	other := createVehicleTestCustomer(t, database, otherCPO.ID, "vehicle-other-"+uuid.NewString()+"@example.com")
	principal := Principal{CPOID: cpo.ID, CustomerID: first.ID}
	secondPrincipal := Principal{CPOID: cpo.ID, CustomerID: second.ID}
	otherPrincipal := Principal{CPOID: otherCPO.ID, CustomerID: other.ID}

	base := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	service.now = func() time.Time { return base }
	created, err := service.CreateVehicle(ctx, principal, CreateVehicleRequest{VehicleNumber: " KA01AB1234 ", VehicleType: stringPointer(" Hatchback "), VehicleMake: stringPointer(" Tata "), VehicleModel: stringPointer(" Tiago EV ")})
	if err != nil {
		t.Fatalf("create vehicle: %v", err)
	}
	if created.VehicleNumber != "KA01AB1234" || created.VehicleType == nil || *created.VehicleType != "Hatchback" || created.LastCharged != nil || !created.DateAdded.Equal(base) {
		t.Fatalf("unexpected server-owned create view: %#v", created)
	}
	service.now = func() time.Time { return base.Add(time.Minute) }
	secondCreated, err := service.CreateVehicle(ctx, principal, CreateVehicleRequest{VehicleNumber: "MH02CD5678", VehicleType: stringPointer("SUV"), VehicleMake: stringPointer("Tata"), VehicleModel: stringPointer("Nexon EV")})
	if err != nil {
		t.Fatalf("create second vehicle: %v", err)
	}
	if _, err := service.CreateVehicle(ctx, secondPrincipal, CreateVehicleRequest{VehicleNumber: "DL03EF9012", VehicleMake: stringPointer("Tata")}); err != nil {
		t.Fatalf("create same-CPO foreign vehicle: %v", err)
	}
	if _, err := service.CreateVehicle(ctx, otherPrincipal, CreateVehicleRequest{VehicleNumber: "GJ04GH3456", VehicleMake: stringPointer("Tata")}); err != nil {
		t.Fatalf("create different-CPO foreign vehicle: %v", err)
	}

	page, err := service.ListVehicles(ctx, principal, CustomerVehicleListQuery{Limit: 1, Search: "tata", VehicleMake: " TATA "})
	if err != nil {
		t.Fatalf("list filtered vehicles: %v", err)
	}
	if len(page.Vehicles) != 1 || page.Vehicles[0].ID != secondCreated.ID || !page.HasMore || page.NextBefore == nil || page.NextBeforeID == nil {
		t.Fatalf("unexpected filtered first page: %#v", page)
	}
	next, err := service.ListVehicles(ctx, principal, CustomerVehicleListQuery{Limit: 1, Search: "tata", VehicleMake: "Tata", Before: page.NextBefore, BeforeID: page.NextBeforeID})
	if err != nil {
		t.Fatalf("list cursor page: %v", err)
	}
	if len(next.Vehicles) != 1 || next.Vehicles[0].ID != created.ID || next.HasMore {
		t.Fatalf("unexpected filtered next page: %#v", next)
	}
	blankSearch, err := service.ListVehicles(ctx, principal, CustomerVehicleListQuery{Search: "   "})
	if err != nil || len(blankSearch.Vehicles) != 2 {
		t.Fatalf("blank search did not behave as no search: %#v, %v", blankSearch, err)
	}

	if _, err := service.GetVehicle(ctx, secondPrincipal, created.ID); !isVehicleNotFound(err) {
		t.Fatalf("same-CPO foreign vehicle was not hidden: %v", err)
	}
	if _, err := service.GetVehicle(ctx, otherPrincipal, created.ID); !isVehicleNotFound(err) {
		t.Fatalf("different-CPO foreign vehicle was not hidden: %v", err)
	}
	service.now = func() time.Time { return base.Add(2 * time.Minute) }
	updated, err := service.UpdateVehicle(ctx, principal, created.ID, UpdateVehicleRequest{VehicleType: nil, VehicleMake: stringPointer("Tata Motors"), vehicleTypeSet: true, vehicleMakeSet: true})
	if err != nil {
		t.Fatalf("update vehicle: %v", err)
	}
	if updated.VehicleType != nil || updated.VehicleMake == nil || *updated.VehicleMake != "Tata Motors" {
		t.Fatalf("unexpected partial update: %#v", updated)
	}
	if _, err := service.UpdateVehicle(ctx, principal, created.ID, UpdateVehicleRequest{}); apiErrorCode(err) != "invalid_vehicle_update" {
		t.Fatalf("empty patch error = %v", err)
	}
	if _, err := service.UpdateVehicle(ctx, principal, created.ID, UpdateVehicleRequest{VehicleMake: stringPointer("Tata Motors"), vehicleMakeSet: true}); err != nil {
		t.Fatalf("no-op patch: %v", err)
	}
	var audits []models.AuditLog
	if err := database.Where("cpo_id = ? AND entity = ? AND entity_id = ?", cpo.ID, "VEHICLE", created.ID).Order("created_at ASC").Find(&audits).Error; err != nil {
		t.Fatalf("load vehicle audits: %v", err)
	}
	if len(audits) != 2 || audits[0].Action != "CUSTOMER_VEHICLE_CREATED" || audits[1].Action != "CUSTOMER_VEHICLE_UPDATED" {
		t.Fatalf("unexpected vehicle audit evidence: %#v", audits)
	}
	if err := service.DeleteVehicle(ctx, principal, created.ID); err != nil {
		t.Fatalf("delete vehicle: %v", err)
	}
	if _, err := service.GetVehicle(ctx, principal, created.ID); !isVehicleNotFound(err) {
		t.Fatalf("deleted vehicle remained readable: %v", err)
	}
}

func newVehicleTestService(t *testing.T, database *gorm.DB) *Service {
	t.Helper()
	box, err := security.NewSecretBox("vehicle-test", []byte(strings.Repeat("m", 32)))
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	tokens, err := security.NewTokenManager("vehicle-test", "vehicle-test-api", time.Minute, []byte(strings.Repeat("s", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	service, err := NewService(database, config.Auth{}, false, cmsmail.NewOutbox(box), tokens)
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	return service
}

func createVehicleTestCustomer(t *testing.T, database *gorm.DB, cpoID uuid.UUID, email string) models.Customer {
	t.Helper()
	now := time.Now().UTC()
	customer := models.Customer{ID: uuid.New(), CPOID: cpoID, Email: email, PasswordHash: "not-used", FullName: "Vehicle Test Customer", IsVerified: true, Status: constants.CustomerStatusActive, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	return customer
}

func isVehicleNotFound(err error) bool { return apiErrorCode(err) == "vehicle_not_found" }

func apiErrorCode(err error) string {
	if apiError, ok := err.(*APIError); ok {
		return apiError.Code
	}
	return ""
}
