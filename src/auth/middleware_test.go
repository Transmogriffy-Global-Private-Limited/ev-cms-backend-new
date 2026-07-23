package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
