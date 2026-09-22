package support

import (
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
)

func RegisterCustomerRoutes(group *gin.RouterGroup, customerAuth *customerauth.Service, service *Service) {
	group.Use(cmsmiddleware.NoStore, customerAuth.Authenticate(), customerauth.RequireAppID())
	group.GET("/support/tickets", func(c *gin.Context) {
		p, ok := customerauth.CurrentPrincipal(c)
		if !ok {
			customerWrite(c, http.StatusUnauthorized, nil, &customerauth.APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Authentication is required."})
			return
		}
		q, e := listQuery(c)
		if e != nil {
			customerWrite(c, http.StatusBadRequest, nil, e)
			return
		}
		v, e := service.ListCustomer(c.Request.Context(), p, q)
		customerWrite(c, http.StatusOK, v, e)
	})
	group.POST("/support/tickets", func(c *gin.Context) {
		var r CreateRequest
		if !customerDecode(c, &r) {
			return
		}
		p, ok := customerauth.CurrentPrincipal(c)
		if !ok {
			return
		}
		v, e := service.CreateCustomer(c.Request.Context(), p, r)
		customerWrite(c, http.StatusCreated, v, e)
	})
	group.GET("/support/tickets/:ticket_id", func(c *gin.Context) {
		p, ok := customerauth.CurrentPrincipal(c)
		if !ok {
			return
		}
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.GetCustomer(c.Request.Context(), p, id)
		customerWrite(c, http.StatusOK, v, e)
	})
	group.POST("/support/tickets/:ticket_id/replies", func(c *gin.Context) {
		var r ReplyRequest
		if !customerDecode(c, &r) {
			return
		}
		p, ok := customerauth.CurrentPrincipal(c)
		if !ok {
			return
		}
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.ReplyCustomer(c.Request.Context(), p, id, r)
		customerWrite(c, http.StatusOK, v, e)
	})
}
func RegisterCPOCustomerRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	group.Use(cmsmiddleware.NoStore, authService.Authenticate(), auth.RequireCPOAppID())
	group.GET("", func(c *gin.Context) {
		p, _ := auth.CurrentPrincipal(c)
		q, e := listQuery(c)
		if e != nil {
			write(c, http.StatusBadRequest, nil, e)
			return
		}
		v, e := service.ListCPOCustomer(c.Request.Context(), p, q)
		write(c, http.StatusOK, v, e)
	})
	group.GET("/:ticket_id", func(c *gin.Context) {
		p, _ := auth.CurrentPrincipal(c)
		id, ok := ticketID(c)
		if !ok {
			return
		}
		v, e := service.GetCPOCustomer(c.Request.Context(), p, id)
		write(c, http.StatusOK, v, e)
	})
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
		v, e := service.ReplyCPOCustomer(c.Request.Context(), p, id, r)
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
		v, e := service.SetCPOCustomerStatus(c.Request.Context(), p, id, r)
		write(c, http.StatusOK, v, e)
	})
}
func customerDecode(c *gin.Context, v any) bool {
	if !decode(c, v) {
		return false
	}
	return true
}
func customerWrite(c *gin.Context, status int, v any, e error) {
	if e == nil {
		c.JSON(status, v)
		return
	}
	if api, ok := e.(*auth.APIError); ok {
		c.JSON(api.Status, gin.H{"error": gin.H{"code": api.Code, "message": api.Message}})
		return
	}
	if api, ok := e.(*customerauth.APIError); ok {
		c.JSON(api.Status, gin.H{"error": gin.H{"code": api.Code, "message": api.Message}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The request could not be completed."}})
}
