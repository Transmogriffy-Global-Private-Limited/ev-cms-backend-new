package cow

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/gin-gonic/gin"
)

func TestCoWRoutesFailClosedWithoutPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterPlatformRoutes(router.Group("/api/v1/platform/cpos"), nil)
	RegisterCPORoutes(router.Group("/api/v1/cpo"), nil)
	tests := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/platform/cpos/11111111-1111-1111-1111-111111111111/modules/cow"},
		{http.MethodPut, "/api/v1/platform/cpos/11111111-1111-1111-1111-111111111111/modules/cow"},
		{http.MethodGet, "/api/v1/cpo/cow/access"},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s: want 403, got %d", tt.method, tt.path, recorder.Code)
		}
	}
	middleware := gin.New()
	middleware.GET("/protected", RequirePermission(nil, cpopermissions.CowJobsRead), func(c *gin.Context) { c.Status(200) })
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("unguarded middleware allowed request: %d", response.Code)
	}
}
