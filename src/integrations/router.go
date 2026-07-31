package integrations

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	handler := &Handler{service: service}
	group.Use(
		noStore,
		authService.Authenticate(),
		auth.RequireCPOAppID(),
		auth.RequireCPORoles(constants.CPORoleAdmin),
	)
	group.GET("", handler.list)
	group.GET("/:provider", handler.get)
	group.PUT("/:provider", handler.put)
	group.DELETE("/:provider", handler.delete)
}

func (handler *Handler) list(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	records, err := handler.service.List(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"integrations": records})
}

func (handler *Handler) get(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.Get(
		ctx.Request.Context(),
		principal,
		ctx.Param("provider"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) put(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request RazorpayCredentials
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status: http.StatusBadRequest, Code: "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.PutRazorpay(
		ctx.Request.Context(),
		principal,
		ctx.Param("provider"),
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) delete(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	if err := handler.service.Delete(
		ctx.Request.Context(),
		principal,
		ctx.Param("provider"),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func writeError(ctx *gin.Context, err error) {
	var apiErr *auth.APIError
	if errors.As(err, &apiErr) {
		ctx.JSON(apiErr.Status, gin.H{
			"error": gin.H{"code": apiErr.Code, "message": apiErr.Message},
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

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
