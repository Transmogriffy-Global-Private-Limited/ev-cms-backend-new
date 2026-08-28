package cpo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func TestTariffRequestBodiesRejectTargetIDs(t *testing.T) {
	t.Parallel()

	for _, destination := range []any{&CreateTariffRequest{}, &UpdateTariffRequest{}} {
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"hub_id":"a4cb9e30-cb12-4a92-a6d8-1d4adba84a62"}`))
		if err := decodeJSON(ctx, destination); err == nil {
			t.Fatalf("%T accepted a route-derived target ID", destination)
		}
	}
}

func TestTariffRequestBodiesRejectLegacyPricePerKWh(t *testing.T) {
	t.Parallel()

	for _, destination := range []any{&CreateTariffRequest{}, &UpdateTariffRequest{}} {
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"price_per_kwh":"10.00"}`))
		if err := decodeJSON(ctx, destination); err == nil {
			t.Fatalf("%T accepted the retired price_per_kwh request field", destination)
		}
	}
}

func TestInitialHubVisibilityRequiresExistingRootTariff(t *testing.T) {
	t.Parallel()

	if err := validateInitialHubVisibility(false); err != nil {
		t.Fatalf("hidden Hub creation validation: %v", err)
	}
	err := validateInitialHubVisibility(true)
	var apiError *auth.APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusConflict || apiError.Code != "hub_tariff_root_required" {
		t.Fatalf("visible Hub creation error=%v, want 409 hub_tariff_root_required", err)
	}
}

func TestCreateHubRejectsVisibleRequestBeforeDatabaseWrite(t *testing.T) {
	t.Parallel()

	latitude, longitude := 22.55, 88.35
	visible := true
	cpoID := uuid.New()
	role := constants.CPORoleAdmin
	_, err := (&Service{}).CreateHub(context.Background(), auth.Principal{
		UserID: uuid.New(), Scope: constants.AuthScopeCPO, CPOID: &cpoID, Role: &role,
	}, CreateHubRequest{
		Name: "Hidden first", Address: "1 Test Road", State: constants.IndianState("West Bengal"),
		Latitude: &latitude, Longitude: &longitude, CustomerVisible: &visible,
	})
	var apiError *auth.APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusConflict || apiError.Code != "hub_tariff_root_required" {
		t.Fatalf("CreateHub visible error=%v, want 409 hub_tariff_root_required", err)
	}
}

func TestMapHubWriteErrorMapsCustomerVisibleRootGuard(t *testing.T) {
	t.Parallel()

	err := mapHubWriteError(&pgconn.PgError{
		Code:    "23514",
		Message: "customer-visible hub requires one enabled unbounded hub tariff",
	}, "create hub")
	var apiError *auth.APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusConflict || apiError.Code != "hub_tariff_root_required" {
		t.Fatalf("root guard error=%v, want 409 hub_tariff_root_required", err)
	}
}

func TestUpdateTariffRequestNullableFieldsPreserveJSONIntent(t *testing.T) {
	t.Parallel()

	var omitted UpdateTariffRequest
	if err := json.Unmarshal([]byte(`{"price_per_unit":"12.50"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.Units.Present() || omitted.StartDate.Present() || omitted.EndDate.Present() {
		t.Fatalf("omitted nullable fields were marked present: %+v", omitted)
	}

	var cleared UpdateTariffRequest
	if err := json.Unmarshal([]byte(`{"units":null,"start_date":null,"end_date":null}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared.Units.Present() || cleared.Units.Value() != nil || !cleared.StartDate.Present() || cleared.StartDate.Value() != nil || !cleared.EndDate.Present() || cleared.EndDate.Value() != nil {
		t.Fatalf("explicit null did not preserve clear intent: %+v", cleared)
	}

	var supplied UpdateTariffRequest
	if err := json.Unmarshal([]byte(`{"units":"kwh","start_date":"2026-08-20T10:00:00Z","end_date":"2026-08-21T10:00:00Z"}`), &supplied); err != nil {
		t.Fatal(err)
	}
	if units := supplied.Units.Value(); !supplied.Units.Present() || units == nil || *units != constants.UnitKWh {
		t.Fatalf("units value intent=%v/%v, want kwh", supplied.Units.Present(), units)
	}
	if start, end := supplied.StartDate.Value(), supplied.EndDate.Value(); start == nil || end == nil || !start.Before(*end) {
		t.Fatalf("schedule value intent start=%v end=%v", start, end)
	}
}

func TestApplyTariffUpdateValidatesResultingTariffState(t *testing.T) {
	t.Parallel()

	fixed := constants.TariffTypeFixed
	energy, timePrice, session := constants.PriceTypeEnergy, constants.PriceTypeTime, constants.PriceTypeSession
	kwh, minutes := constants.UnitKWh, constants.UnitMinutes
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	baseline := func() models.Tariff {
		return models.Tariff{PricePerUnit: decimal.NewFromInt(10), Currency: "INR", IsActive: true, TariffType: &fixed, PriceType: &energy, Units: &kwh}
	}

	for _, test := range []struct {
		name  string
		patch UpdateTariffRequest
		valid bool
	}{
		{name: "energy to session clears units", patch: UpdateTariffRequest{PriceType: &session, Units: PatchNull[constants.Unit]()}, valid: true},
		{name: "energy to time sets minutes", patch: UpdateTariffRequest{PriceType: &timePrice, Units: PatchValue(minutes)}, valid: true},
		{name: "energy remains canonical kwh", patch: UpdateTariffRequest{PriceType: &energy, Units: PatchValue(kwh)}, valid: true},
		{name: "session without clearing prior units is rejected", patch: UpdateTariffRequest{PriceType: &session}},
		{name: "open-ended fallback is accepted", patch: UpdateTariffRequest{StartDate: PatchValue(start)}, valid: true},
		{name: "end-only schedule is rejected", patch: UpdateTariffRequest{EndDate: PatchValue(end)}},
		{name: "complete schedule is accepted", patch: UpdateTariffRequest{StartDate: PatchValue(start), EndDate: PatchValue(end)}, valid: true},
		{name: "schedule clears only as a pair", patch: UpdateTariffRequest{StartDate: PatchNull[time.Time](), EndDate: PatchNull[time.Time]()}, valid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tariff := baseline()
			updates, changed := applyTariffUpdate(&tariff, test.patch)
			if len(updates) == 0 || len(changed) == 0 {
				t.Fatal("patch did not produce persistence/audit projections")
			}
			err := validateTariffCommercial(tariff.TariffType, tariff.PriceType, tariff.Units, tariff.IdleFeePerMin, tariff.IsActive)
			if err == nil {
				err = validateTariffDateRange(tariff.StartDate, tariff.EndDate)
			}
			if (err == nil) != test.valid {
				t.Fatalf("resulting tariff validation err=%v, want valid=%t", err, test.valid)
			}
			if !tariff.PricePerUnit.Equal(decimal.NewFromInt(10)) {
				t.Fatalf("unpatched price changed to %s", tariff.PricePerUnit)
			}
		})
	}

	tariff := baseline()
	applyTariffUpdate(&tariff, UpdateTariffRequest{StartDate: PatchValue(start), EndDate: PatchValue(end)})
	updates, _ := applyTariffUpdate(&tariff, UpdateTariffRequest{StartDate: PatchNull[time.Time](), EndDate: PatchNull[time.Time]()})
	if tariff.StartDate != nil || tariff.EndDate != nil || updates["start_date"] != nil || updates["end_date"] != nil {
		t.Fatalf("explicit schedule clear did not persist nil dates: tariff=%+v updates=%+v", tariff, updates)
	}
}

func TestOptionalNonNegativeWholeCurrencyFormValue(t *testing.T) {
	makeContext := func(body string) *gin.Context {
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return ctx
	}

	value, err := optionalNonNegativeWholeCurrencyFormValue(makeContext("wallet_min_balance=500"), "wallet_min_balance")
	if err != nil || value == nil || *value != 500 {
		t.Fatalf("valid wallet minimum value=%v err=%v, want 500", value, err)
	}
	if value, err := optionalNonNegativeWholeCurrencyFormValue(makeContext(""), "wallet_min_balance"); err != nil || value != nil {
		t.Fatalf("omitted wallet minimum value=%v err=%v, want nil", value, err)
	}
	for _, body := range []string{"wallet_min_balance=-1", "wallet_min_balance=20.5", "wallet_min_balance=not-a-number"} {
		if _, err := optionalNonNegativeWholeCurrencyFormValue(makeContext(body), "wallet_min_balance"); err == nil {
			t.Fatalf("invalid wallet minimum form %q was accepted", body)
		}
	}
}

func TestValidateCreateRequest(t *testing.T) {
	t.Parallel()

	valid := CreateRequest{
		Slug:         "example-cpo",
		BusinessName: "Example Charging",
		CompanyType:  constants.CPOCompanyTypeCompany,
		GSTIN:        "19ABCDE1234F1ZX",
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
		{name: "GSTIN required", mutate: func(request *CreateRequest) { request.GSTIN = "" }, code: "invalid_gstin"},
		{name: "GSTIN checksum", mutate: func(request *CreateRequest) { request.GSTIN = "19ABCDE1234F1ZZ" }, code: "invalid_gstin"},
		{name: "GSTIN state mismatch", mutate: func(request *CreateRequest) { request.State = constants.Maharashtra }, code: "invalid_gstin_state_mismatch"},
		{name: "address", mutate: func(request *CreateRequest) { request.Address = "" }, code: "invalid_address"},
		{name: "city", mutate: func(request *CreateRequest) { request.City = "" }, code: "invalid_city"},
		{name: "state", mutate: func(request *CreateRequest) { request.State = "" }, code: "invalid_state"},
		{name: "pincode", mutate: func(request *CreateRequest) { request.Pincode = "70001A" }, code: "invalid_pincode"},
		{name: "business name", mutate: func(request *CreateRequest) { request.BusinessName = "---" }, code: "invalid_business_name"},
		{name: "administrator name", mutate: func(request *CreateRequest) { request.Admin.FullName = "123" }, code: "invalid_admin_full_name"},
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

func TestGSTINChecksumAndStateValidation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		gstin string
		state constants.IndianState
		code  string
	}{
		{name: "known valid West Bengal GSTIN", gstin: "19ABCDE1234F1ZX", state: constants.WestBengal},
		{name: "known valid Maharashtra GSTIN", gstin: "27AAPFU0939F1ZV", state: constants.Maharashtra},
		{name: "invalid checksum", gstin: "19ABCDE1234F1ZZ", state: constants.WestBengal, code: "invalid_gstin"},
		{name: "state mismatch", gstin: "19ABCDE1234F1ZX", state: constants.Maharashtra, code: "invalid_gstin_state_mismatch"},
		{name: "unsupported state code", gstin: "97ABCDE1234F1ZQ", state: constants.WestBengal, code: "invalid_gstin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateGSTIN(test.gstin, test.state)
			if test.code == "" {
				if err != nil {
					t.Fatalf("valid GSTIN was rejected: %v", err)
				}
				return
			}
			var apiErr *auth.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != test.code {
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
		GSTIN:        " 19abcde1234f1zx ",
		Address:      " 1 Test Road ",
		City:         " Kolkata ",
		State:        " West Bengal ",
		Pincode:      " 700001 ",
	})
	if request.BusinessName != "Example Charging" ||
		request.City != "Kolkata" ||
		request.GSTIN != "19ABCDE1234F1ZX" ||
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
		GSTIN:        "19ABCDE1234F1ZX",
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
		{name: "GSTIN required", mutate: func(request *UpdateProfileRequest) { request.GSTIN = "" }, code: "invalid_gstin"},
		{name: "GSTIN checksum", mutate: func(request *UpdateProfileRequest) { request.GSTIN = "19ABCDE1234F1ZZ" }, code: "invalid_gstin"},
		{name: "GSTIN state mismatch", mutate: func(request *UpdateProfileRequest) { request.State = constants.Maharashtra }, code: "invalid_gstin_state_mismatch"},
		{name: "address", mutate: func(request *UpdateProfileRequest) { request.Address = "" }, code: "invalid_address"},
		{name: "city", mutate: func(request *UpdateProfileRequest) { request.City = "" }, code: "invalid_city"},
		{name: "state", mutate: func(request *UpdateProfileRequest) { request.State = "" }, code: "invalid_state"},
		{name: "pincode", mutate: func(request *UpdateProfileRequest) { request.Pincode = "70001A" }, code: "invalid_pincode"},
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

func TestTenantServiceGuardOnlyChecksTrustedCPOContext(t *testing.T) {
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

	for _, role := range []constants.CPORole{
		constants.CPORoleAdmin,
		constants.CPORoleOwner,
		constants.CPORoleOperator,
		constants.CPORoleViewer,
	} {
		if err := requireCPOAdminAccess(principal(role)); err != nil {
			t.Fatalf("CPO context for %s was rejected: %v", role, err)
		}
	}
}

func TestAdminProfileViewReturnsActualMembershipRole(t *testing.T) {
	t.Parallel()
	user := models.User{ID: uuid.New(), Email: "viewer@example.com", FullName: "Viewer"}
	view := adminProfileView(user, uuid.New(), constants.CPORoleViewer)
	if view.Role != constants.CPORoleViewer {
		t.Fatalf("profile role = %q, want VIEWER", view.Role)
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
		GSTIN:                 "19ABCDE1234F1ZX",
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

func TestMapWriteErrorExplainsCPOIdentityChecks(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		constraint string
		code       string
	}{
		{constraint: "chk_cpos_gstin", code: "invalid_gstin"},
		{constraint: "chk_cpos_gstin_state_matches", code: "invalid_gstin_state_mismatch"},
		{constraint: "chk_cpos_pincode_format", code: "invalid_pincode"},
	} {
		t.Run(test.constraint, func(t *testing.T) {
			err := mapWriteError(&pgconn.PgError{Code: "23514", ConstraintName: test.constraint}, "test write")
			var apiErr *auth.APIError
			if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Code != test.code {
				t.Fatalf("got error %v, want 400 %s", err, test.code)
			}
		})
	}
}

func TestHubViewIncludesState(t *testing.T) {
	t.Parallel()

	view := hubView(models.Hub{State: "West Bengal"})
	if view.State != "West Bengal" {
		t.Fatalf("hub state=%q, want West Bengal", view.State)
	}
}

func TestValidateGSTForHubUsesCompleteResultingRelationship(t *testing.T) {
	t.Parallel()
	nine, eighteen, zero := decimal.NewFromInt(9), decimal.NewFromInt(18), decimal.Zero
	hub := models.Hub{State: constants.WestBengal}

	for _, tc := range []struct {
		name  string
		gst   models.GST
		valid bool
	}{
		{"same state split tax", models.GST{IsActive: true, State: constants.WestBengal, SGSTRate: &nine, CGSTRate: &nine, IGSTRate: &zero}, true},
		{"same state IGST", models.GST{IsActive: true, State: constants.WestBengal, SGSTRate: &nine, CGSTRate: &nine, IGSTRate: &eighteen}, false},
		{"interstate IGST", models.GST{IsActive: true, State: constants.Maharashtra, SGSTRate: &zero, CGSTRate: &zero, IGSTRate: &eighteen}, true},
		{"inactive profile", models.GST{IsActive: false, State: constants.WestBengal, SGSTRate: &nine, CGSTRate: &nine, IGSTRate: &zero}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGSTForHub(hub, tc.gst)
			if (err == nil) != tc.valid {
				t.Fatalf("validateGSTForHub() error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestGSTRequestValidationRejectsMixedTaxComponents(t *testing.T) {
	t.Parallel()
	nine, eighteen := decimal.NewFromInt(9), decimal.NewFromInt(18)
	err := validateCreateGSTRequest(CreateGSTRequest{
		Name:     "mixed",
		State:    constants.WestBengal,
		SGSTRate: &nine,
		CGSTRate: &nine,
		IGSTRate: &eighteen,
	})
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_gst_components" {
		t.Fatalf("mixed GST components error=%v, want invalid_gst_components", err)
	}
}

func TestTariffValidation(t *testing.T) {
	t.Parallel()

	validTariffType := constants.TariffTypeFixed
	validPriceType := constants.PriceTypeEnergy
	validUnits := constants.UnitKWh

	// --- Create ---
	validCreateReq := CreateTariffRequest{
		TariffType: &validTariffType,
		PriceType:  &validPriceType,
		Units:      &validUnits,
		Currency:   "INR",
	}

	if err := validateCreateTariffRequest(validCreateReq); err != nil {
		t.Fatalf("valid create request was rejected: %v", err)
	}
	for _, request := range []CreateTariffRequest{
		{TariffType: &validTariffType, PriceType: func() *constants.PriceType { value := constants.PriceTypeTime; return &value }(), Units: func() *constants.Unit { value := constants.UnitMinutes; return &value }(), Currency: "INR"},
		{TariffType: &validTariffType, PriceType: func() *constants.PriceType { value := constants.PriceTypeSession; return &value }(), Currency: "INR"},
	} {
		if err := validateCreateTariffRequest(request); err != nil {
			t.Fatalf("supported tariff request was rejected: %v", err)
		}
	}

	for _, tc := range []struct {
		name    string
		req     CreateTariffRequest
		errCode string
	}{
		{
			"invalid tariff type",
			func() CreateTariffRequest {
				req := validCreateReq
				invalid := constants.TariffType("invalid")
				req.TariffType = &invalid
				return req
			}(),
			"invalid_tariff_type",
		},
		{
			"invalid price type",
			func() CreateTariffRequest {
				req := validCreateReq
				invalid := constants.PriceType("invalid")
				req.PriceType = &invalid
				return req
			}(),
			"invalid_price_type",
		},
		{
			"invalid units",
			func() CreateTariffRequest {
				req := validCreateReq
				invalid := constants.Unit("invalid")
				req.Units = &invalid
				return req
			}(),
			"invalid_units",
		},
		{
			"retired watt hour units",
			func() CreateTariffRequest {
				req := validCreateReq
				legacy := constants.LegacyUnitWattHour
				req.Units = &legacy
				return req
			}(),
			"invalid_units",
		},
		{
			"active idle fee unsupported",
			func() CreateTariffRequest {
				req := validCreateReq
				req.IdleFeePerMin = decimal.NewFromInt(1)
				return req
			}(),
			"idle_fee_unsupported",
		},
		{
			"unsupported time unit",
			func() CreateTariffRequest {
				req := validCreateReq
				timePriceType := constants.PriceTypeTime
				req.PriceType = &timePriceType
				return req
			}(),
			"unsupported_tariff_pricing",
		},
		{
			"missing tariff semantics",
			CreateTariffRequest{Currency: "INR"},
			"unsupported_tariff_pricing",
		},
	} {
		t.Run("create_"+tc.name, func(t *testing.T) {
			var apiErr *auth.APIError
			err := validateCreateTariffRequest(tc.req)
			if !errors.As(err, &apiErr) || apiErr.Code != tc.errCode {
				t.Fatalf("got %v, want %s", err, tc.errCode)
			}
		})
	}

	// --- Update ---
	validUpdateReq := UpdateTariffRequest{
		TariffType: &validTariffType,
		PriceType:  &validPriceType,
		Units:      PatchValue(validUnits),
	}

	if err := validateUpdateTariffRequest(validUpdateReq); err != nil {
		t.Fatalf("valid update request was rejected: %v", err)
	}

	for _, tc := range []struct {
		name    string
		req     UpdateTariffRequest
		errCode string
	}{
		{
			"invalid tariff type",
			func() UpdateTariffRequest {
				req := validUpdateReq
				invalid := constants.TariffType("invalid")
				req.TariffType = &invalid
				return req
			}(),
			"invalid_tariff_type",
		},
		{
			"invalid price type",
			func() UpdateTariffRequest {
				req := validUpdateReq
				invalid := constants.PriceType("invalid")
				req.PriceType = &invalid
				return req
			}(),
			"invalid_price_type",
		},
		{
			"invalid units",
			func() UpdateTariffRequest {
				req := validUpdateReq
				invalid := constants.Unit("invalid")
				req.Units = PatchValue(invalid)
				return req
			}(),
			"invalid_units",
		},
		{
			"empty update",
			UpdateTariffRequest{},
			"invalid_tariff",
		},
	} {
		t.Run("update_"+tc.name, func(t *testing.T) {
			var apiErr *auth.APIError
			err := validateUpdateTariffRequest(tc.req)
			if !errors.As(err, &apiErr) || apiErr.Code != tc.errCode {
				t.Fatalf("got %v, want %s", err, tc.errCode)
			}
		})
	}
}
