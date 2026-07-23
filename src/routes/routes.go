package routes

import (
	"context"
	"net/http"
	"time"

	apidocs "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/docs/contracts/openapi"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	PingContext(context.Context) error
}

func New(
	database DatabasePinger,
	authService *auth.Service,
	cpoService *cpo.Service,
	integrationService *integrations.Service,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	_ = router.SetTrustedProxies(nil)
	apidocs.Register(router)

	router.GET("/health/live", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/health/ready", func(ctx *gin.Context) {
		pingContext, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		defer cancel()

		if database == nil || database.PingContext(pingContext) != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	if authService != nil {
		auth.RegisterRoutes(router.Group("/api/v1/auth"), authService)
	}
	if authService != nil && cpoService != nil {
		cpo.RegisterPlatformRoutes(
			router.Group("/api/v1/platform/cpos"),
			authService,
			cpoService,
		)
	}
	if authService != nil && integrationService != nil {
		integrations.RegisterRoutes(
			router.Group("/api/v1/cpo/integrations"),
			authService,
			integrationService,
		)
	}

	return router
}
