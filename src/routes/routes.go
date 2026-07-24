package routes

import (
	"context"
	"net/http"
	"time"

	apidocs "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/docs/contracts/openapi"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	PingContext(context.Context) error
}

func New(
	database DatabasePinger,
	authService *auth.Service,
	customerAuthService *customerauth.Service,
	cpoService *cpo.Service,
	integrationService *integrations.Service,
	platformService *platformops.Service,
	corsAllowAll bool,
	apiDocsEnabled bool,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(permissiveCORSMiddleware(corsAllowAll))
	_ = router.SetTrustedProxies(nil)
	if apiDocsEnabled {
		apidocs.Register(router)
	}

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
		if platformService != nil {
			ready, err := platformService.RequiredWorkersReady(pingContext)
			if err != nil || !ready {
				ctx.JSON(
					http.StatusServiceUnavailable,
					gin.H{"status": "not_ready"},
				)
				return
			}
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	if authService != nil {
		auth.RegisterRoutes(router.Group("/api/v1/auth"), authService)
	}
	if customerAuthService != nil {
		customerauth.RegisterRoutes(router.Group("/api/v1/app/auth"), customerAuthService)
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
	if authService != nil && platformService != nil {
		platformops.RegisterRoutes(
			router.Group("/api/v1/platform"),
			authService,
			platformService,
		)
	}
	return router
}
