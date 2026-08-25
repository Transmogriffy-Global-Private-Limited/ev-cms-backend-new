package support

import (
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RegisterCPORoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	group.Use(authService.Authenticate(), auth.RequireCPOAppID())
	group.GET("", func(c *gin.Context) { list(c, service) })
	group.POST("", func(c *gin.Context) {
		var r CreateRequest
		if !decode(c, &r) {
			return
		}
		p, _ := auth.CurrentPrincipal(c)
		v, e := service.Create(c.Request.Context(), p, r)
		write(c, http.StatusCreated, v, e)
	})
	group.GET("/:ticket_id", func(c *gin.Context) { get(c, service) })
	group.POST("/:ticket_id/replies", func(c *gin.Context) {
		var r ReplyRequest
		if !decode(c, &r) {
			return
		}
		p, _ := auth.CurrentPrincipal(c)
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.Reply(c.Request.Context(), p, id, r)
		write(c, http.StatusOK, v, e)
	})
}
func RegisterPlatformRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	group.Use(authService.Authenticate(), auth.RequirePlatform())
	group.GET("", func(c *gin.Context) { list(c, service) })
	group.GET("/:ticket_id", func(c *gin.Context) { get(c, service) })
	group.POST("/:ticket_id/replies", func(c *gin.Context) {
		var r ReplyRequest
		if !decode(c, &r) {
			return
		}
		p, _ := auth.CurrentPrincipal(c)
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.Reply(c.Request.Context(), p, id, r)
		write(c, http.StatusOK, v, e)
	})
	group.PATCH("/:ticket_id/status", func(c *gin.Context) {
		var r StatusRequest
		if !decode(c, &r) {
			return
		}
		p, _ := auth.CurrentPrincipal(c)
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.SetStatus(c.Request.Context(), p, id, r)
		write(c, http.StatusOK, v, e)
	})
}
func list(c *gin.Context, s *Service) {
	p, _ := auth.CurrentPrincipal(c)
	v, e := s.List(c.Request.Context(), p)
	write(c, http.StatusOK, v, e)
}
func get(c *gin.Context, s *Service) {
	p, _ := auth.CurrentPrincipal(c)
	id, ok := ticketID(c)
	if !ok {
		return
	}
	v, e := s.Get(c.Request.Context(), p, id)
	write(c, http.StatusOK, v, e)
}
func ticketID(c *gin.Context) (uuid.UUID, bool) {
	id, e := uuid.Parse(c.Param("ticket_id"))
	if e != nil || id == uuid.Nil {
		write(c, http.StatusBadRequest, nil, invalid())
		return uuid.Nil, false
	}
	return id, true
}
func decode(c *gin.Context, v any) bool {
	if c.ShouldBindJSON(v) != nil {
		write(c, http.StatusBadRequest, nil, invalid())
		return false
	}
	return true
}
func write(c *gin.Context, status int, v any, e error) {
	if e != nil {
		a, ok := e.(*auth.APIError)
		if ok {
			c.JSON(a.Status, gin.H{"error": gin.H{"code": a.Code, "message": a.Message}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The request could not be completed."}})
		return
	}
	c.JSON(status, v)
}
