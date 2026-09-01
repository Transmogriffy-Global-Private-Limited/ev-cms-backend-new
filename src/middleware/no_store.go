package middleware

import "github.com/gin-gonic/gin"

// NoStore protects authenticated response bodies from browser/proxy caching.
func NoStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
