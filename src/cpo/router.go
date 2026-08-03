package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
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
	group.GET("/slug-availability", handler.slugAvailability)
	group.GET("/:cpo_id", handler.get)
	group.PUT("/:cpo_id/profile", handler.updateProfile)
	group.POST("/:cpo_id/activate", handler.activate)
	group.POST("/:cpo_id/suspend", handler.suspend)
	group.PUT("/:cpo_id/app-id", handler.setAppID)
	group.GET("/:cpo_id/primary-admin", handler.primaryAdmin)
	group.PUT("/:cpo_id/primary-admin", handler.setPrimaryAdmin)
	group.POST(
		"/:cpo_id/primary-admin/resend-onboarding",
		handler.resendPrimaryAdminOnboarding,
	)
	group.POST(
		"/:cpo_id/administrative-sessions/revoke",
		handler.revokeAdministrativeSessions,
	)
}

func (handler *Handler) slugAvailability(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.CheckSlugAvailability(
		ctx.Request.Context(),
		principal,
		ctx.Query("slug"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
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
	query, ok := parseListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.List(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
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
	operation func(
		context.Context,
		auth.Principal,
		uuid.UUID,
		LifecycleRequest,
	) (View, error),
) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	var request LifecycleRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := operation(ctx.Request.Context(), principal, cpoID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
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
		cpoID,
		request,
	)
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

func (handler *Handler) primaryAdmin(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetPrimaryAdmin(
		ctx.Request.Context(),
		principal,
		cpoID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) setPrimaryAdmin(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	var request PrimaryAdminRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.SetPrimaryAdmin(
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

func (handler *Handler) resendPrimaryAdminOnboarding(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	request, ok := decodeReasonRequest(ctx)
	if !ok {
		return
	}
	record, err := handler.service.ResendPrimaryAdminOnboarding(
		ctx.Request.Context(),
		principal,
		cpoID,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, record)
}

func (handler *Handler) revokeAdministrativeSessions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	request, ok := decodeReasonRequest(ctx)
	if !ok {
		return
	}
	response, err := handler.service.RevokeAdministrativeSessions(
		ctx.Request.Context(),
		principal,
		cpoID,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func decodeReasonRequest(ctx *gin.Context) (ReasonRequest, bool) {
	var request ReasonRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return ReasonRequest{}, false
	}
	return request, true
}

func parseListQuery(ctx *gin.Context) (ListQuery, bool) {
	query := ListQuery{Search: strings.TrimSpace(ctx.Query("q"))}
	if statusText := strings.TrimSpace(ctx.Query("status")); statusText != "" {
		status := constants.CPOStatus(strings.ToUpper(statusText))
		query.Status = &status
	}
	if modeText := strings.TrimSpace(ctx.Query("app_id_mode")); modeText != "" {
		mode := constants.CPOAppIDMode(strings.ToUpper(modeText))
		query.AppMode = &mode
	}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return ListQuery{}, false
		}
		query.Limit = limit
	}
	beforeText := strings.TrimSpace(ctx.Query("before"))
	beforeIDText := strings.TrimSpace(ctx.Query("before_id"))
	if beforeText != "" {
		before, err := time.Parse(time.RFC3339, beforeText)
		if err != nil {
			writeError(
				ctx,
				invalid("before", "Before must be an RFC3339 timestamp."),
			)
			return ListQuery{}, false
		}
		query.Before = &before
	}
	if beforeIDText != "" {
		beforeID, err := uuid.Parse(beforeIDText)
		if err != nil || beforeID == uuid.Nil {
			writeError(
				ctx,
				invalid("before_id", "Before ID must be a non-zero UUID."),
			)
			return ListQuery{}, false
		}
		query.BeforeID = &beforeID
	}
	return query, true
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

func parseUserID(ctx *gin.Context) (uuid.UUID, bool) {
	userID, err := uuid.Parse(ctx.Param("user_id"))
	if err != nil || userID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_user_id",
			Message: "The user ID is invalid.",
		})
		return uuid.Nil, false
	}
	return userID, true
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
		auth.RequireCPORoles(constants.CPORoleAdmin),
	)

	group.GET("/admin/profile", handler.getAdminProfile)
	group.PATCH("/admin/profile", handler.updateAdminProfile)
	group.GET("/organization", handler.getOrganization)
	group.GET("/users/:user_id", handler.getUser)
	group.POST("/chargers", handler.createCharger)
	group.GET("/chargers", handler.listChargers)
	group.GET("/chargers/:charger_id", handler.getCharger)
	group.PATCH("/chargers/:charger_id", handler.updateCharger)
	group.DELETE("/chargers/:charger_id", handler.deleteCharger)
	group.POST("/hubs", handler.createHub)
	group.GET("/hubs", handler.listHubs)
	group.GET("/hubs/:hub_id", handler.getHub)
	group.PATCH("/hubs/:hub_id", handler.updateHub)
	group.POST("/tariffs", handler.createTariff)
	group.GET("/tariffs", handler.listTariffs)
	group.GET("/tariffs/:tariff_id", handler.getTariff)
	group.PATCH("/tariffs/:tariff_id", handler.updateTariff)
	group.POST("/gsts", handler.createGST)
	group.GET("/gsts", handler.listGSTs)
	group.GET("/gsts/:gst_id", handler.getGST)
	group.PATCH("/gsts/:gst_id", handler.updateGST)
}

func (handler *Handler) getAdminProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.GetAdminProfile(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateAdminProfile(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request UpdateAdminProfileRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.UpdateAdminProfile(
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

func (handler *Handler) getOrganization(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.GetOrganization(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) getUser(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userID, ok := parseUserID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetUser(ctx.Request.Context(), principal, userID)
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
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}

	records, err := handler.service.ListChargers(
		ctx.Request.Context(),
		principal,
		query,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, records)
}

func parseTenantListQuery(ctx *gin.Context) (TenantListQuery, bool) {
	query := TenantListQuery{}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return TenantListQuery{}, false
		}
		query.Limit = limit
	}
	beforeText := strings.TrimSpace(ctx.Query("before"))
	beforeIDText := strings.TrimSpace(ctx.Query("before_id"))
	if beforeText != "" {
		before, err := time.Parse(time.RFC3339, beforeText)
		if err != nil {
			writeError(ctx, invalid("before", "Before must be an RFC3339 timestamp."))
			return TenantListQuery{}, false
		}
		query.Before = &before
	}
	if beforeIDText != "" {
		beforeID, err := uuid.Parse(beforeIDText)
		if err != nil || beforeID == uuid.Nil {
			writeError(ctx, invalid("before_id", "Before ID must be a non-zero UUID."))
			return TenantListQuery{}, false
		}
		query.BeforeID = &beforeID
	}
	return query, true
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

	if err := handler.service.DeleteCharger(
		ctx.Request.Context(),
		principal,
		ctx.Param("charger_id"),
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

func (handler *Handler) listHubs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListHubs(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
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

func (handler *Handler) listTariffs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListTariffs(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
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

func (handler *Handler) createGST(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateGSTRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateGST(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) listGSTs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListGSTs(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

func (handler *Handler) getGST(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	gstID, ok := parseGSTID(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetGST(ctx.Request.Context(), principal, gstID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateGST(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	gstID, ok := parseGSTID(ctx)
	if !ok {
		return
	}

	var request UpdateGSTRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateGST(ctx.Request.Context(), principal, gstID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func parseGSTID(ctx *gin.Context) (uuid.UUID, bool) {
	gstID, err := uuid.Parse(ctx.Param("gst_id"))
	if err != nil || gstID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_gst_id",
			Message: "The GST ID is invalid.",
		})
		return uuid.Nil, false
	}
	return gstID, true
}
