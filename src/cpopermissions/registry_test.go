package cpopermissions

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

func TestRoleDefaultsKeepStaffManagementAdministrative(t *testing.T) {
	if !RoleAllows(constants.CPORoleAdmin, StaffManage) {
		t.Fatal("admin must manage staff")
	}
	if RoleAllows(constants.CPORoleOperator, StaffManage) || RoleAllows(constants.CPORoleViewer, StaffManage) {
		t.Fatal("non-administrative roles must not manage staff by default")
	}
	if RoleAllows(constants.CPORoleViewer, "unknown.permission") {
		t.Fatal("unknown permissions must never be allowed")
	}
}

func TestAdminRoleIncludesEveryRegisteredCapability(t *testing.T) {
	for _, permission := range Catalog() {
		if !RoleAllows(constants.CPORoleAdmin, permission.Key) {
			t.Fatalf("admin is missing registered permission %q", permission.Key)
		}
	}
}
