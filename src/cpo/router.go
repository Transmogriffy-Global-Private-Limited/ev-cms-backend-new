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

func RegisterCPORoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{
		service: service,
	}

	group.Use(
		noStore,
		authService.Authenticate(),
		auth.RequireCPOAppID(),
	)

	group.POST("/profile", handler.createProfile)
	group.GET("/profile", handler.getProfile)
	group.PATCH("/profile", handler.updateProfile)
	group.POST("/chargers", handler.createCharger)
	group.GET("/chargers", handler.listChargers)
	group.GET("/chargers/:charger_id", handler.getCharger)
	group.PATCH("/chargers/:charger_id", handler.updateCharger)
	group.DELETE("/chargers", handler.deleteCharger)
	group.POST("/hubs", handler.createHub)
	group.GET("/hubs/:hub_id", handler.getHub)
	group.PATCH("/hubs/:hub_id", handler.updateHub)
	group.POST("/tariffs", handler.createTariff)
	group.GET("/tariffs/:tariff_id", handler.getTariff)
	group.PATCH("/tariffs/:tariff_id", handler.updateTariff)
}

func (handler *Handler) createProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateProfileRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateProfile(
		ctx.Request.Context(),
		principal,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) getProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	record, err := handler.service.GetProfile(
		ctx.Request.Context(),
		principal,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request UpdateProfileRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateProfile(
		ctx.Request.Context(),
		principal,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}
func (handler *Handler) createCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateChargerRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateCharger(
		ctx.Request.Context(),
		principal,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) listChargers(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	records, err := handler.service.ListChargers(
		ctx.Request.Context(),
		principal,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"chargers": records,
	})
}

func (handler *Handler) getCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	record, err := handler.service.GetCharger(
		ctx.Request.Context(),
		principal,
		ctx.Param("charger_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request UpdateChargerRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateCharger(
		ctx.Request.Context(),
		principal,
		ctx.Param("charger_id"),
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) deleteCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request DeleteChargerRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	if err := handler.service.DeleteCharger(
		ctx.Request.Context(),
		principal,
		request,
	); err != nil {
		writeError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) createHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateHubRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateHub(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) getHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetHub(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	var request UpdateHubRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateHub(ctx.Request.Context(), principal, hubID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func parseHubID(ctx *gin.Context) (uuid.UUID, bool) {
	hubID, err := uuid.Parse(ctx.Param("hub_id"))
	if err != nil || hubID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_hub_id",
			Message: "The hub ID is invalid.",
		})
		return uuid.Nil, false
	}
	return hubID, true
}

func (handler *Handler) createTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateTariffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateTariff(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) getTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetTariff(ctx.Request.Context(), principal, tariffID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}

	var request UpdateTariffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateTariff(ctx.Request.Context(), principal, tariffID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func parseTariffID(ctx *gin.Context) (uuid.UUID, bool) {
	tariffID, err := uuid.Parse(ctx.Param("tariff_id"))
	if err != nil || tariffID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_tariff_id",
			Message: "The tariff ID is invalid.",
		})
		return uuid.Nil, false
	}
	return tariffID, true
}
