package cpo

import (
	"encoding/json"
	"errors"
	"net/http"
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
		GSTIN:        "19ABCDE1234F5Z6",
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
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

	for _, test := range []struct {
		name   string
		mutate func(*CreateRequest)
		code   string
	}{
		{name: "GSTIN", mutate: func(request *CreateRequest) { request.GSTIN = "" }, code: "invalid_gstin"},
		{name: "address", mutate: func(request *CreateRequest) { request.Address = "" }, code: "invalid_address"},
		{name: "city", mutate: func(request *CreateRequest) { request.City = "" }, code: "invalid_city"},
		{name: "state", mutate: func(request *CreateRequest) { request.State = "" }, code: "invalid_state"},
		{name: "pincode", mutate: func(request *CreateRequest) { request.Pincode = "" }, code: "invalid_pincode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			var apiErr *auth.APIError
			if err := validateCreateRequest(request); !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
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

	service := NewService(nil, nil, true, "dummy.connection.url")
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

	service := NewService(nil, nil, false, "dummy.connection.url")
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

	request := normalizeProfileRequest(UpdateProfileRequest{
		BusinessName: "  Example Charging  ",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        " 19abcde1234f5z6 ",
		Address:      " 1 Test Road ",
		City:         " Kolkata ",
		State:        " West Bengal ",
		Pincode:      " 700001 ",
	})
	if request.BusinessName != "Example Charging" ||
		request.City != "Kolkata" ||
		request.GSTIN != "19ABCDE1234F5Z6" ||
		request.Address != "1 Test Road" ||
		request.State != "West Bengal" ||
		request.Pincode != "700001" {
		t.Fatalf("unexpected normalized profile: %#v", request)
	}
	if err := validateProfileRequest(request); err != nil {
		t.Fatalf("valid profile was rejected: %v", err)
	}
	request.BusinessName = ""
	if err := validateProfileRequest(request); err == nil {
		t.Fatal("blank business name was accepted")
	}

	valid := normalizeProfileRequest(UpdateProfileRequest{
		BusinessName: "Example Charging",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        "19ABCDE1234F5Z6",
		Address:      "1 Test Road",
		City:         "Kolkata",
		State:        "West Bengal",
		Pincode:      "700001",
	})
	for _, test := range []struct {
		name   string
		mutate func(*UpdateProfileRequest)
		code   string
	}{
		{name: "GSTIN", mutate: func(request *UpdateProfileRequest) { request.GSTIN = "" }, code: "invalid_gstin"},
		{name: "address", mutate: func(request *UpdateProfileRequest) { request.Address = "" }, code: "invalid_address"},
		{name: "city", mutate: func(request *UpdateProfileRequest) { request.City = "" }, code: "invalid_city"},
		{name: "state", mutate: func(request *UpdateProfileRequest) { request.State = "" }, code: "invalid_state"},
		{name: "pincode", mutate: func(request *UpdateProfileRequest) { request.Pincode = "" }, code: "invalid_pincode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			var apiErr *auth.APIError
			if err := validateProfileRequest(candidate); !errors.As(err, &apiErr) || apiErr.Code != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestSlugAvailabilityRequiresPlatformBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, true, "dummy.connection.url")
	_, err := service.CheckSlugAvailability(t.Context(), auth.Principal{
		UserID: uuid.New(),
		Scope:  constants.AuthScopeCPO,
	}, "example-cpo")
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "forbidden" {
		t.Fatalf("got error %v, want platform authorization failure", err)
	}
}

func TestSlugAvailabilityNormalizesAndValidatesBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	if got := normalizeSlug("  Example-CPO  "); got != "example-cpo" {
		t.Fatalf("normalized slug %q, want example-cpo", got)
	}

	service := NewService(nil, nil, true, "dummy.connection.url")
	_, err := service.CheckSlugAvailability(t.Context(), auth.Principal{
		UserID: uuid.New(),
		Scope:  constants.AuthScopePlatform,
	}, "not valid")
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_slug" {
		t.Fatalf("got error %v, want invalid_slug", err)
	}
}

func TestCPORecoveryOperationsRequirePlatformBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, true, "dummy.connection.url")
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
		GSTIN:                 "19ABCDE1234F5Z6",
		Address:               "1 Test Road",
		City:                  "Kolkata",
		State:                 "West Bengal",
		Pincode:               "700001",
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

	longitude := 88.35
	negativeSanctionLoad := -0.01
	hub.Longitude = &longitude
	hub.SanctionLoad = &negativeSanctionLoad
	if err := validateCreateHubRequest(hub); err == nil {
		t.Fatal("hub with negative sanction load was accepted")
	}

	standaloneCharger := CreateChargerRequest{
		Vendor:       "Delta",
		Model:        "Standalone Wallbox",
		SerialNumber: "SN-1",
		MaxPowerKW:   7.4,
		Connectors: []CreateConnectorRequest{{
			ConnectorNumber: 1,
			ConnectorType:   "TYPE2",
		}},
	}
	if err := validateCreateChargerRequest(standaloneCharger); err != nil {
		t.Fatalf("standalone charger was rejected: %v", err)
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

func TestValidateTariffDateRangeRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(-1 * time.Hour)
	if err := validateTariffDateRange(&start, &end); err == nil {
		t.Fatal("invalid date range was accepted")
	}

	later := start.Add(48 * time.Hour)
	if err := validateTariffDateRange(&start, &later); err != nil {
		t.Fatalf("valid date range was rejected: %v", err)
	}
	if err := validateTariffDateRange(&start, nil); err == nil {
		t.Fatal("partial date range was accepted")
	}
	if err := validateTariffDateRange(nil, &later); err == nil {
		t.Fatal("partial date range was accepted")
	}
	if err := validateTariffDateRange(nil, nil); err != nil {
		t.Fatalf("open-ended tariff was rejected: %v", err)
	}
}

func TestMapTariffWriteErrorRecognizesScheduleExclusion(t *testing.T) {
	t.Parallel()

	err := mapTariffWriteError(&pgconn.PgError{Code: "23P01"}, "create tariff")
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || apiErr.Code != "tariff_schedule_conflict" {
		t.Fatalf("got %v, want tariff_schedule_conflict", err)
	}
}

func TestChargerListRejectsInvalidCursorBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	cpoID := uuid.New()
	role := constants.CPORoleAdmin
	service := NewService(nil, nil, false, "dummy.connection.url")
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

func TestMapWriteErrorExplainsKnownCPOUniquenessConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		constraint string
		code       string
		message    string
	}{
		{
			constraint: "uq_cpos_slug_normalized",
			code:       "cpo_slug_conflict",
			message:    "The CPO slug is already in use.",
		},
		{
			constraint: "uq_cpos_gstin_normalized",
			code:       "cpo_gstin_conflict",
			message:    "The GSTIN is already assigned to another CPO.",
		},
		{
			constraint: "uq_cpos_app_id",
			code:       "cpo_app_id_conflict",
			message:    "The CPO app ID is already in use.",
		},
		{
			constraint: "uq_users_email_normalized",
			code:       "admin_identity_conflict",
			message:    "An administrator identity with this email was created concurrently. Retry the request.",
		},
		{
			constraint: "uq_cpo_membership",
			code:       "cpo_admin_membership_conflict",
			message:    "The administrator already has a membership for this CPO.",
		},
		{
			constraint: "uq_cpo_memberships_primary_admin",
			code:       "cpo_primary_admin_conflict",
			message:    "The CPO already has a primary administrator.",
		},
	}
	for _, test := range tests {
		t.Run(test.constraint, func(t *testing.T) {
			err := mapWriteError(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: test.constraint,
			}, "test write")
			var apiErr *auth.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("got error %v, want API error", err)
			}
			if apiErr.Status != 409 || apiErr.Code != test.code || apiErr.Message != test.message {
				t.Fatalf("got %#v, want 409 %s with message %q", apiErr, test.code, test.message)
			}
		})
	}
}

func TestMapWriteErrorKeepsGenericFallbackForUnknownUniquenessConflict(t *testing.T) {
	t.Parallel()

	err := mapWriteError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "unknown_constraint",
	}, "test write")
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 || apiErr.Code != "cpo_conflict" {
		t.Fatalf("got error %v, want 409 cpo_conflict", err)
	}
}
