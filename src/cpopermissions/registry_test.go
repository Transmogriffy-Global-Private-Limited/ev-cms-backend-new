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

func TestEffectivePermissionPrecedence(t *testing.T) {
	effective := Effective(constants.CPORoleViewer,
		[]string{StaffManage, ChargersRead},
		[]string{ChargersRead},
	)
	contains := func(key string) bool {
		for _, candidate := range effective {
			if candidate == key {
				return true
			}
		}
		return false
	}
	if !contains(StaffManage) {
		t.Fatal("explicit ALLOW must extend a role default")
	}
	if contains(ChargersRead) {
		t.Fatal("DENY must override both ALLOW and the role default")
	}
}

func TestCapabilityOverridesCoverReadAndManageRoles(t *testing.T) {
	contains := func(values []string, key string) bool {
		for _, value := range values {
			if value == key {
				return true
			}
		}
		return false
	}
	operatorRead := Effective(constants.CPORoleOperator, nil, nil)
	if !contains(operatorRead, ChargersRead) {
		t.Fatal("operator role default must retain a real read capability")
	}
	viewerManage := Effective(constants.CPORoleViewer, []string{HubsManage}, nil)
	if !contains(viewerManage, HubsManage) {
		t.Fatal("explicit ALLOW must let viewer use a real manage capability")
	}
	adminDenied := Effective(constants.CPORoleAdmin, nil, []string{HubsManage})
	if contains(adminDenied, HubsManage) {
		t.Fatal("explicit DENY must remove an administrator manage capability")
	}
	viewerDefault := Effective(constants.CPORoleViewer, nil, nil)
	if contains(viewerDefault, HubsManage) {
		t.Fatal("viewer without an explicit capability must not gain manage authority")
	}
}
