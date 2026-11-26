package cpopermissions

import (
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

func TestCowCapabilityCatalogIsExplicitAndDenyWins(t *testing.T) {
	seen := make(map[string]bool)
	count := 0
	for _, d := range Catalog() {
		if !strings.HasPrefix(d.Key, "cow.") {
			continue
		}
		count++
		if seen[d.Key] || d.Module != "cow" || !Known(d.Key) {
			t.Fatalf("invalid/duplicate CoW permission %q", d.Key)
		}
		seen[d.Key] = true
		if !RoleAllows(constants.CPORoleAdmin, d.Key) || !RoleAllows(constants.CPORoleOwner, d.Key) {
			t.Fatalf("owner/admin must receive explicit CoW capability %s", d.Key)
		}
	}
	if count != len(cowCatalog) || count < 40 {
		t.Fatalf("CoW catalog incomplete: %d", count)
	}
	if RoleAllows(constants.CPORoleViewer, CowJobsManage) {
		t.Fatal("viewer must not default to managing jobs")
	}
	for _, key := range Effective(constants.CPORoleOwner, []string{CowJobsManage}, []string{CowJobsManage}) {
		if key == CowJobsManage {
			t.Fatal("explicit DENY must defeat even Owner role")
		}
	}
	if Known("cow.*") || Known("cow.not_real") {
		t.Fatal("CoW wildcard/unknown permissions must not authorize")
	}
}
