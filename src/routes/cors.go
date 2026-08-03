package routes

import (
	"net/http"
	"strings"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
)

const allowedCORSMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

func permissiveCORSMiddleware(enabled bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !enabled {
			ctx.Next()
			return
		}

		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", allowedCORSMethods)
		ctx.Header("Access-Control-Expose-Headers", cmsmiddleware.RequestIDHeader)
		ctx.Header("Access-Control-Max-Age", "600")

		requestedHeaders := strings.TrimSpace(
			ctx.GetHeader("Access-Control-Request-Headers"),
		)
		if requestedHeaders == "" {
			requestedHeaders = "Authorization, Content-Type, X-CPO-App-ID"
		}
		ctx.Header("Access-Control-Allow-Headers", requestedHeaders)

		if ctx.Request.Method == http.MethodOptions {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		ctx.Next()
	}
}
