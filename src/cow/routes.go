package cow

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// These groups inherit their authentication, platform/CPO scope, and CPO App
// ID validation from cpo.RegisterPlatformRoutes / RegisterCPORoutes.
func RegisterPlatformRoutes(group *gin.RouterGroup, database *gorm.DB) {
	group.GET("/:cpo_id/modules/cow", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("cpo_id"))
		if err != nil || id == uuid.Nil {
			writeError(c, apiError(400, "invalid_cpo_id", "CPO ID is invalid."))
			return
		}
		principal, ok := auth.CurrentPrincipal(c)
		if !ok {
			writeError(c, forbidden())
			return
		}
		view, err := GetPlatformEntitlement(c.Request.Context(), database, principal, id)
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(200, view)
	})
	group.PUT("/:cpo_id/modules/cow", func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("cpo_id"))
		if err != nil || id == uuid.Nil {
			writeError(c, apiError(400, "invalid_cpo_id", "CPO ID is invalid."))
			return
		}
		principal, ok := auth.CurrentPrincipal(c)
		if !ok {
			writeError(c, forbidden())
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		var request ChangeRequest
		if err := decoder.Decode(&request); err != nil {
			writeError(c, apiError(400, "invalid_request", "The request body is invalid."))
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeError(c, apiError(400, "invalid_request", "The request body must contain one JSON object."))
			return
		}
		view, err := SetPlatformEntitlement(c.Request.Context(), database, principal, id, request)
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(200, view)
	})
}

func RegisterCPORoutes(group *gin.RouterGroup, database *gorm.DB) {
	group.GET("/cow/access", func(c *gin.Context) {
		principal, ok := auth.CurrentPrincipal(c)
		if !ok {
			writeError(c, forbidden())
			return
		}
		view, err := GetCPOAccess(c.Request.Context(), database, principal)
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(200, view)
	})
}

// RequirePermission is available to future CoW route groups; service code
// must also invoke Authorize before performing its protected action.
func RequirePermission(database *gorm.DB, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := auth.CurrentPrincipal(c)
		if !ok {
			writeError(c, forbidden())
			c.Abort()
			return
		}
		if err := Authorize(c.Request.Context(), database, principal, permission); err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

func writeError(c *gin.Context, err error) {
	var api *auth.APIError
	if !errors.As(err, &api) {
		api = &auth.APIError{Status: 500, Code: "internal_error", Message: "The request could not be completed."}
	}
	c.JSON(api.Status, gin.H{"error": gin.H{"code": api.Code, "message": api.Message}})
}
