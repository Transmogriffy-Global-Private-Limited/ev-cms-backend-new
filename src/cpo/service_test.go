package cpo

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
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

func TestTenantOperationsCurrentlyRequireAdmin(t *testing.T) {
	t.Parallel()

	cpoID := uuid.New()
	principal := func(role constants.CPORole) auth.Principal {
		return auth.Principal{
			UserID: uuid.New(),
			Scope:  constants.AuthScopeCPO,
			CPOID:  &cpoID,
			Role:   &role,
		}
	}

	if err := requireCPOAdminAccess(principal(constants.CPORoleAdmin)); err != nil {
		t.Fatalf("administrator access was rejected: %v", err)
	}
	for _, role := range []constants.CPORole{
		constants.CPORoleOwner,
		constants.CPORoleOperator,
		constants.CPORoleViewer,
	} {
		if err := requireCPOAdminAccess(principal(role)); err == nil {
			t.Fatalf("dormant %s role was allowed to use CPO operations", role)
		}
	}
}

func TestOrganizationViewContainsTenantSafeFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	actorID := uuid.New()
	record := models.CPO{
		ID:                    uuid.New(),
		Slug:                  "example-cpo",
		BusinessName:          "Example Charging",
		CompanyType:           constants.CPOCompanyTypeCompany,
		Status:                constants.CPOStatusActive,
		StatusReason:          "Internal platform decision",
		StatusChangedAt:       now,
		StatusChangedByUserID: &actorID,
		AppID:                 "example_live_app_id",
		AppIDMode:             constants.CPOAppIDModeLive,
		AppIDUpdatedAt:        now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	got := organizationView(record)
	if got.ID != record.ID || got.BusinessName != record.BusinessName ||
		got.Status != record.Status || got.AppID != record.AppID {
		t.Fatalf("unexpected organization projection: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal organization projection: %v", err)
	}
	for _, forbiddenField := range []string{"status_reason", "status_changed_by_user_id"} {
		if strings.Contains(string(encoded), forbiddenField) {
			t.Fatalf("organization projection exposed %s: %s", forbiddenField, encoded)
		}
	}
}

func TestTenantOperationValidation(t *testing.T) {
	t.Parallel()

	tariff := normalizeCreateTariffRequest(CreateTariffRequest{
		HubID:       uuid.New(),
		PricePerKWh: decimal.RequireFromString("18.5"),
	})
	if tariff.Currency != "INR" {
		t.Fatalf("blank currency normalized to %q, want INR", tariff.Currency)
	}
	if err := validateCreateTariffRequest(tariff); err != nil {
		t.Fatalf("valid tariff was rejected: %v", err)
	}

	latitude := 22.57
	hub := CreateHubRequest{
		Name:      "Central Hub",
		Address:   "1 Test Road",
		Latitude:  &latitude,
		Longitude: nil,
	}
	if err := validateCreateHubRequest(hub); err == nil {
		t.Fatal("hub without longitude was accepted")
	}

	nine := decimal.NewFromInt(9)
	eighteen := decimal.NewFromInt(18)
	gst := CreateGSTRequest{
		Name:     "Standard GST",
		SGSTRate: &nine,
		CGSTRate: &nine,
		IGSTRate: &eighteen,
	}
	if err := validateCreateGSTRequest(gst); err != nil {
		t.Fatalf("valid GST was rejected: %v", err)
	}
	gst.IGSTRate = nil
	if err := validateCreateGSTRequest(gst); err == nil {
		t.Fatal("GST without IGST rate was accepted")
	}
}

func TestChargerListRejectsInvalidCursorBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	cpoID := uuid.New()
	role := constants.CPORoleAdmin
	service := NewService(nil, nil, false)
	_, err := service.ListChargers(
		t.Context(),
		auth.Principal{
			UserID: uuid.New(),
			Scope:  constants.AuthScopeCPO,
			CPOID:  &cpoID,
			Role:   &role,
		},
		TenantListQuery{BeforeID: &cpoID},
	)
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_cursor" {
		t.Fatalf("got error %v, want invalid_cursor", err)
	}
}

func TestMapChargerDeleteErrorRecognizesPostgresDependencyViolations(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"23001", "23503"} {
		t.Run(code, func(t *testing.T) {
			err := mapChargerDeleteError(&pgconn.PgError{Code: code})
			var apiErr *auth.APIError
			if !errors.As(err, &apiErr) || apiErr.Status != 409 || apiErr.Code != "charger_in_use" {
				t.Fatalf("got error %v, want 409 charger_in_use", err)
			}
		})
	}
}
