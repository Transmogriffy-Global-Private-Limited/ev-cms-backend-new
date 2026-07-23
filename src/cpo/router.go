package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func RegisterPlatformRoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{service: service}
	group.Use(noStore, authService.Authenticate(), auth.RequirePlatform())
	group.POST("", handler.create)
	group.GET("", handler.list)
	group.GET("/:cpo_id", handler.get)
	group.POST("/:cpo_id/activate", handler.activate)
	group.POST("/:cpo_id/suspend", handler.suspend)
	group.PUT("/:cpo_id/app-id", handler.setAppID)
}

func (handler *Handler) create(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request CreateRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	response, err := handler.service.Create(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}

func (handler *Handler) list(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	records, err := handler.service.List(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"cpos": records})
}

func (handler *Handler) get(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.Get(ctx.Request.Context(), principal, cpoID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) activate(ctx *gin.Context) {
	handler.transition(ctx, handler.service.Activate)
}

func (handler *Handler) suspend(ctx *gin.Context) {
	handler.transition(ctx, handler.service.Suspend)
}

func (handler *Handler) transition(
	ctx *gin.Context,
	operation func(context.Context, auth.Principal, uuid.UUID) (View, error),
) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	record, err := operation(ctx.Request.Context(), principal, cpoID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) setAppID(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	var request SetAppIDRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.SetLiveAppID(
		ctx.Request.Context(),
		principal,
		cpoID,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func parseCPOID(ctx *gin.Context) (uuid.UUID, bool) {
	cpoID, err := uuid.Parse(ctx.Param("cpo_id"))
	if err != nil || cpoID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_cpo_id",
			Message: "The CPO ID is invalid.",
		})
		return uuid.Nil, false
	}
	return cpoID, true
}

func decodeJSON(ctx *gin.Context, destination any) error {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeError(ctx *gin.Context, err error) {
	var apiError *auth.APIError
	if errors.As(err, &apiError) {
		ctx.JSON(apiError.Status, gin.H{
			"error": gin.H{"code": apiError.Code, "message": apiError.Message},
		})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":    "internal_error",
			"message": "The request could not be completed.",
		},
	})
}

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
