package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	PingContext(context.Context) error
}

func New(database DatabasePinger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

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

	return router
}
