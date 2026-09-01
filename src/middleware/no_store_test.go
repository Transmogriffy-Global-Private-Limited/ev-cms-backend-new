package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNoStoreProtectsSuccessAndErrorResponses(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	for _, status := range []int{http.StatusNoContent, http.StatusForbidden} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			router := gin.New()
			router.Use(NoStore)
			router.GET("/", func(ctx *gin.Context) { ctx.Status(status) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Pragma") != "no-cache" {
				t.Fatalf("cache headers = %#v", recorder.Header())
			}
		})
	}
}
