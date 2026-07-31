package cpo

import (
	"errors"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

func TestValidateCreateRequest(t *testing.T) {
	t.Parallel()

	valid := CreateRequest{
		Slug:         "example-cpo",
		BusinessName: "Example Charging",
		CompanyType:  constants.CPOCompanyTypeCompany,
		Admin: InitialAdminRequest{
			Email:    "admin@example.com",
			FullName: "CPO Admin",
		},
	}
	if err := validateCreateRequest(valid); err != nil {
		t.Fatalf("valid request was rejected: %v", err)
	}

	invalid := valid
	invalid.Admin.Email = "not-an-email"
	if err := validateCreateRequest(invalid); err == nil {
		t.Fatal("invalid administrator email was accepted")
	}
}

func TestTemporaryPasswordMeetsPasswordPolicy(t *testing.T) {
	t.Parallel()

	first, err := generateTemporaryPassword()
	if err != nil {
		t.Fatalf("generate first temporary password: %v", err)
	}
	second, err := generateTemporaryPassword()
	if err != nil {
		t.Fatalf("generate second temporary password: %v", err)
	}
	if first == second {
		t.Fatal("temporary password generator repeated a value")
	}
	if !strings.HasPrefix(first, "Tmp-") || len(first) < 20 {
		t.Fatalf("unexpected temporary password shape")
	}
}

func TestCreateRequiresPlatformScopeBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, true)
	_, err := service.Create(
		t.Context(),
		auth.Principal{
			UserID: uuid.New(),
			Scope:  constants.AuthScopeCPO,
		},
		CreateRequest{},
	)
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "forbidden" {
		t.Fatalf("got error %v, want platform authorization failure", err)
	}
}

func TestCreateRejectsMailDisabledBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, false)
	_, err := service.Create(
		t.Context(),
		auth.Principal{
			UserID: uuid.New(),
			Scope:  constants.AuthScopePlatform,
		},
		CreateRequest{},
	)
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "mail_unavailable" {
		t.Fatalf("got error %v, want mail availability failure", err)
	}
}

func TestValidateReason(t *testing.T) {
	t.Parallel()

	for _, reason := range []string{
		"Approved after onboarding review",
		"  Access recovery requested by CPO owner  ",
	} {
		if err := validateReason(reason); err != nil {
			t.Fatalf("valid reason %q was rejected: %v", reason, err)
		}
	}
	for _, reason := range []string{"", "  ", "no"} {
		if err := validateReason(reason); err == nil {
			t.Fatalf("invalid reason %q was accepted", reason)
		}
	}
}

func TestNormalizeAndValidateProfileRequest(t *testing.T) {
	t.Parallel()

	blankGSTIN := " "
	request := normalizeProfileRequest(UpdateProfileRequest{
		BusinessName: "  Example Charging  ",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        &blankGSTIN,
		City:         " Kolkata ",
	})
	if request.BusinessName != "Example Charging" ||
		request.City != "Kolkata" ||
		request.GSTIN != nil {
		t.Fatalf("unexpected normalized profile: %#v", request)
	}
	if err := validateProfileRequest(request); err != nil {
		t.Fatalf("valid profile was rejected: %v", err)
	}
	request.BusinessName = ""
	if err := validateProfileRequest(request); err == nil {
		t.Fatal("blank business name was accepted")
	}
}

func TestCPORecoveryOperationsRequirePlatformBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, true)
	principal := auth.Principal{
		UserID: uuid.New(),
		Scope:  constants.AuthScopeCPO,
	}
	cpoID := uuid.New()
	if _, err := service.GetPrimaryAdmin(t.Context(), principal, cpoID); err == nil {
		t.Fatal("CPO principal read the primary administrator")
	}
	if _, err := service.SetPrimaryAdmin(
		t.Context(),
		principal,
		cpoID,
		PrimaryAdminRequest{},
	); err == nil {
		t.Fatal("CPO principal replaced the primary administrator")
	}
	if _, err := service.RevokeAdministrativeSessions(
		t.Context(),
		principal,
		cpoID,
		ReasonRequest{},
	); err == nil {
		t.Fatal("CPO principal revoked administrative sessions")
	}
}
