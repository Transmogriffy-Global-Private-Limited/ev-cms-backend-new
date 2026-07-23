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
