package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRequireCPOAppID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	cpoID := uuid.New()
	role := constants.CPORoleAdmin
	appID := "cpo_dummy_" + strings.Repeat("a", 32)
	basePrincipal := Principal{
		UserID:   uuid.New(),
		Scope:    constants.AuthScopeCPO,
		CPOID:    &cpoID,
		Role:     &role,
		CPOAppID: &appID,
	}

	tests := []struct {
		name       string
		principal  Principal
		header     string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "matching app ID",
			principal:  basePrincipal,
			header:     appID,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing app ID",
			principal:  basePrincipal,
			wantStatus: http.StatusBadRequest,
			wantCode:   "missing_cpo_app_id",
		},
		{
			name:       "wrong app ID",
			principal:  basePrincipal,
			header:     "live_wrong_application_id",
			wantStatus: http.StatusForbidden,
			wantCode:   "cpo_app_id_mismatch",
		},
		{
			name: "temporary password blocks business APIs",
			principal: func() Principal {
				principal := basePrincipal
				principal.User.MustChangePassword = true
				return principal
			}(),
			header:     appID,
			wantStatus: http.StatusForbidden,
			wantCode:   "password_change_required",
		},
		{
			name:       "platform scope cannot use tenant boundary",
			principal:  Principal{UserID: uuid.New(), Scope: constants.AuthScopePlatform},
			header:     appID,
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			router := gin.New()
			router.Use(func(ctx *gin.Context) {
				ctx.Set(principalContextKey, test.principal)
				ctx.Next()
			})
			router.GET("/", RequireCPOAppID(), func(ctx *gin.Context) {
				ctx.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				request.Header.Set(CPOAppIDHeader, test.header)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("got status %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantCode != "" && !strings.Contains(
				recorder.Body.String(),
				`"code":"`+test.wantCode+`"`,
			) {
				t.Fatalf("response %s does not contain error code %q", recorder.Body.String(), test.wantCode)
			}
		})
	}
}

func TestCPOAuthorizationFailureClassification(t *testing.T) {
	t.Parallel()
	if err := CPOAuthorizationError(ErrCPOAccessDenied); err.Status != http.StatusForbidden || err.Code != "forbidden" {
		t.Fatalf("expected authorization-domain miss to be forbidden, got %#v", err)
	}
	if err := CPOAuthorizationError(errors.New("database unavailable")); err.Status != http.StatusInternalServerError || err.Code != "internal_error" {
		t.Fatalf("expected infrastructure failure to be safe internal error, got %#v", err)
	}
}

func TestCPOAuthorizationMiddlewareFailsClosedWithAccurateStatus(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	cpoID := uuid.New()
	role := constants.CPORoleViewer
	principal := Principal{UserID: uuid.New(), Scope: constants.AuthScopeCPO, CPOID: &cpoID, Role: &role}
	unavailableDB, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable connect_timeout=1"), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("construct unavailable database handle: %v", err)
	}

	tests := []struct {
		name       string
		principal  Principal
		middleware gin.HandlerFunc
		wantStatus int
		wantCode   string
	}{
		{
			name:       "permission infrastructure failure is internal error",
			principal:  principal,
			middleware: RequireCPOPermission(unavailableDB, "support.read"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
		{
			name:       "active membership infrastructure failure is internal error",
			principal:  principal,
			middleware: RequireActiveCPOMembership(unavailableDB),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
		{
			name:       "invalid CPO principal remains forbidden",
			principal:  Principal{UserID: uuid.New(), Scope: constants.AuthScopePlatform},
			middleware: RequireActiveCPOMembership(&gorm.DB{}),
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(ctx *gin.Context) {
				ctx.Set(principalContextKey, test.principal)
				ctx.Next()
			})
			router.GET("/", test.middleware, func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("status=%d body=%s, want %d %s", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}
