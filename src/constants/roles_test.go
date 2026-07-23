package constants

import "testing"

func TestCPORoleValid(t *testing.T) {
	t.Parallel()

	valid := []CPORole{
		CPORoleOwner,
		CPORoleAdmin,
		CPORoleOperator,
		CPORoleViewer,
	}
	for _, role := range valid {
		if !role.Valid() {
			t.Fatalf("expected role %q to be valid", role)
		}
	}

	if CPORole("SUPER_ADMIN").Valid() {
		t.Fatal("platform superadmin must not be a CPO membership role")
	}
}
