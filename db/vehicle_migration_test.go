package db

import (
	"strings"
	"testing"
)

func TestVehicleCustomerTenantIntegrityMigrationContract(t *testing.T) {
	t.Parallel()
	up, err := migrationFiles.ReadFile("migrations/000066_vehicle_customer_tenant_integrity.up.sql")
	if err != nil {
		t.Fatalf("read vehicle tenant-integrity up migration: %v", err)
	}
	down, err := migrationFiles.ReadFile("migrations/000066_vehicle_customer_tenant_integrity.down.sql")
	if err != nil {
		t.Fatalf("read vehicle tenant-integrity down migration: %v", err)
	}
	upSQL := strings.ToLower(string(up))
	downSQL := strings.ToLower(string(down))
	for _, required := range []string{"drop constraint if exists fk_vehicles_customer", "add constraint fk_vehicles_customer_tenant", "foreign key (cpo_id, customer_id)", "references customers(cpo_id, id)", "on update cascade", "on delete restrict"} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, required := range []string{"drop constraint if exists fk_vehicles_customer_tenant", "add constraint fk_vehicles_customer", "foreign key (customer_id)", "references customers(id)", "on update cascade", "on delete restrict"} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration missing %q", required)
		}
	}
}
