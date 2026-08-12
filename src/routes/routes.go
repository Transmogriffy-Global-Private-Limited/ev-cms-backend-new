package routes

import (
	"context"
	"io"
	"net/http"
	"time"

	apidocs "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/docs/contracts/openapi"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/subscriptions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/superadmin"
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
	subscriptionService *subscriptions.Service,
	corsAllowAll bool,
	apiDocsEnabled bool,
	requestLogWriter io.Writer,
	debugLogging bool,
	superadminServices ...*superadmin.Service,
) *gin.Engine {
	router := gin.New()
	router.Use(cmsmiddleware.RequestLogger(requestLogWriter, debugLogging))
	router.Use(cmsmiddleware.Recovery(requestLogWriter))
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
		customerauth.RegisterRoutes(router.Group("/api/v1/app"), customerAuthService)
		halops.RegisterFactRoutes(router.Group("/v1"), customerAuthService.HALFactIngestor())
	}
	if authService != nil && cpoService != nil {
		cpo.RegisterPlatformRoutes(
			router.Group("/api/v1/platform/cpos"),
			authService,
			cpoService,
		)
	}
	if authService != nil && cpoService != nil {
		cpo.RegisterCPORoutes(
			router.Group("/api/v1/cpo"),
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
	if authService != nil && subscriptionService != nil {
		subscriptions.RegisterRoutes(
			router.Group("/api/v1/platform"),
			authService,
			subscriptionService,
		)
	}
	if authService != nil && len(superadminServices) > 0 && superadminServices[0] != nil {
		superadmin.RegisterRoutes(
			router.Group("/api/v1/platform"), authService, superadminServices[0],
		)
		superadmin.RegisterCPONotificationRoutes(
			router.Group("/api/v1/cpo"), authService, superadminServices[0],
		)
	}
	return router
}
