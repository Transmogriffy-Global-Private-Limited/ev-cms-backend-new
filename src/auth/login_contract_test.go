package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdministrativeLoginHeaderAndBodyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	appID := "cpo_dummy_0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name, scope, extra string
		headers            []string
		status             int
		code               string
	}{
		{"missing", "CPO", "", nil, 401, "invalid_credentials"},
		{"empty", "CPO", "", []string{""}, 401, "invalid_credentials"},
		{"malformed", "CPO", "", []string{"not valid"}, 401, "invalid_credentials"},
		{"uppercase", "CPO", "", []string{strings.ToUpper(appID)}, 401, "invalid_credentials"},
		{"too long", "CPO", "", []string{strings.Repeat("a", 101)}, 401, "invalid_credentials"},
		{"duplicate", "CPO", "", []string{appID, appID}, 401, "invalid_credentials"},
		{"combined", "CPO", "", []string{appID + "," + appID}, 401, "invalid_credentials"},
		{"body UUID", "CPO", `,"cpo_id":"c821a013-5041-42f7-80c8-aa153cf9d455"`, []string{appID}, 400, "invalid_request"},
		{"body null UUID", "CPO", `,"cpo_id":null`, []string{appID}, 400, "invalid_request"},
		{"body App ID", "CPO", `,"app_id":"` + appID + `"`, nil, 400, "invalid_request"},
		{"platform contradiction", "PLATFORM", "", []string{appID}, 401, "invalid_credentials"},
		{"platform empty header", "PLATFORM", "", []string{""}, 401, "invalid_credentials"},
		// With no database or mail configured, valid requests reach the existing
		// mail-availability gate. Shape failures above must fail before that gate.
		{"valid CPO shape", "CPO", "", []string{appID}, 503, "mail_unavailable"},
		{"valid platform shape", "PLATFORM", "", nil, 503, "mail_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterRoutes(router.Group("/auth"), &Service{})
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"admin@example.test","password":"test","scope":"`+test.scope+`"`+test.extra+`}`))
			request.Header.Set("Content-Type", "application/json")
			for _, header := range test.headers {
				request.Header.Add(CPOAppIDHeader, header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
