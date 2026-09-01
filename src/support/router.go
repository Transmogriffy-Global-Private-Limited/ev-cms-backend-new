package support

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RegisterCPORoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	group.Use(cmsmiddleware.NoStore, authService.Authenticate(), auth.RequireCPOAppID())
	read := group.Group("", auth.RequireCPOPermission(service.database, cpopermissions.SupportRead))
	create := group.Group("", auth.RequireCPOPermission(service.database, cpopermissions.SupportCreate))
	reply := group.Group("", auth.RequireCPOPermission(service.database, cpopermissions.SupportRead), auth.RequireCPOPermission(service.database, cpopermissions.SupportReply))
	read.GET("", func(c *gin.Context) { list(c, service) })
	create.POST("", func(c *gin.Context) {
		var r CreateRequest
		if !decode(c, &r) {
			return
		}
		p, _ := auth.CurrentPrincipal(c)
		v, e := service.Create(c.Request.Context(), p, r)
		write(c, http.StatusCreated, v, e)
	})
	read.GET("/:ticket_id", func(c *gin.Context) { get(c, service) })
	reply.POST("/:ticket_id/replies", func(c *gin.Context) {
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
	group.Use(cmsmiddleware.NoStore, authService.Authenticate(), auth.RequirePlatform())
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
	query, err := listQuery(c)
	if err != nil {
		write(c, http.StatusBadRequest, nil, err)
		return
	}
	v, e := s.List(c.Request.Context(), p, query)
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	decoder := json.NewDecoder(c.Request.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil || len(raw) == 0 || raw[0] != '{' || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		write(c, http.StatusBadRequest, nil, invalid())
		return false
	}
	objectDecoder := json.NewDecoder(bytes.NewReader(raw))
	objectDecoder.DisallowUnknownFields()
	if err := objectDecoder.Decode(v); err != nil {
		write(c, http.StatusBadRequest, nil, invalid())
		return false
	}
	return true
}

func listQuery(c *gin.Context) (ListQuery, error) {
	query := ListQuery{
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("q")),
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_limit", Message: "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_cursor", Message: "The before cursor timestamp is invalid."}
		}
		query.Before = &value
	}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_cursor", Message: "The before cursor ID is invalid."}
		}
		query.BeforeID = &value
	}
	if raw := strings.TrimSpace(c.Query("cpo_id")); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_cpo_id", Message: "The CPO filter is invalid."}
		}
		query.CPOID = &value
	}
	return normalizeListQuery(query)
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
