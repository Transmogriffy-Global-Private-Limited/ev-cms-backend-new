package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
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

func parseChargerID(ctx *gin.Context) (uuid.UUID, bool) {
	chargerID, err := uuid.Parse(ctx.Param("charger_id"))
	if err != nil || chargerID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		})
		return uuid.Nil, false
	}
	return chargerID, true
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
		cmsmiddleware.LogHandledError(ctx, "cpo", apiError.Code, apiError.Status, err)
		ctx.JSON(apiError.Status, gin.H{
			"error": gin.H{"code": apiError.Code, "message": apiError.Message},
		})
		return
	}
	cmsmiddleware.LogHandledError(
		ctx, "cpo", "internal_error", http.StatusInternalServerError, err,
	)
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
	group.GET("/subscription", handler.getSubscription)
	group.GET("/users/:user_id", handler.getUser)
	group.POST("/chargers", handler.createCharger)
	group.GET("/chargers", handler.listChargers)
	group.GET("/chargers/:charger_id", handler.getCharger)
	group.GET("/chargers/:charger_id/image", handler.getChargerImage)
	group.PATCH("/chargers/:charger_id", handler.updateCharger)
	group.DELETE("/chargers/:charger_id", handler.deleteCharger)
	group.POST("/hubs", handler.createHub)
	group.GET("/hubs", handler.listHubs)
	group.GET("/hubs/:hub_id", handler.getHub)
	group.PATCH("/hubs/:hub_id", handler.updateHub)
	group.PUT("/hubs/:hub_id/customer-visibility", handler.updateHubCustomerVisibility)
	group.POST("/hubs/:hub_id/chargers", handler.assignChargerToHub)
	group.POST("/tariffs", handler.createTariff)
	group.GET("/tariffs", handler.listTariffs)
	group.GET("/tariffs/:tariff_id", handler.getTariff)
	group.PATCH("/tariffs/:tariff_id", handler.updateTariff)
	group.POST("/gsts", handler.createGST)
	group.GET("/gsts", handler.listGSTs)
	group.GET("/gsts/:gst_id", handler.getGST)
	group.PATCH("/gsts/:gst_id", handler.updateGST)
	group.PUT("/chargers/:charger_id/status", handler.updateChargerStatus)
	group.GET("/chargers/:charger_id/status", handler.getChargerStatus)
}

// @Summary Update charger status
// @Description Update the status of a specific charger.
// @Tags CPO Network
// @Accept json
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param status body UpdateChargerStatusRequest true "Charger status update data"
// @Success 200 {object} ChargerStatusResponse "Successfully updated charger status"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 409 {object} auth.APIError "OCPP identity mismatch"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/status [put]
func (handler *Handler) updateChargerStatus(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}

	var request UpdateChargerStatusRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateChargerStatus(ctx.Request.Context(), principal, chargerID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

// @Summary Get charger status
// @Description Get the current status of a specific charger.
// @Tags CPO Network
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Success 200 {object} ChargerStatusResponse "Successfully retrieved charger status"
// @Failure 400 {object} auth.APIError "Invalid charger ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/status [get]
func (handler *Handler) getChargerStatus(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetChargerStatus(ctx.Request.Context(), principal, chargerID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

// @Summary Update hub customer visibility
// @Description Update the customer visibility of a specific hub.
// @Tags CPO Network
// @Accept json
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param visibility body UpdateHubCustomerVisibilityRequest true "Hub customer visibility update data"
// @Success 200 {object} HubView "Successfully updated hub customer visibility"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/customer-visibility [put]
func (handler *Handler) updateHubCustomerVisibility(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	var request UpdateHubCustomerVisibilityRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateHubCustomerVisibility(ctx.Request.Context(), principal, hubID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}


func (handler *Handler) getSubscription(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.GetSubscription(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
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

// @Summary Create a new charger
// @Description Create a new charger with connectors. This endpoint uses multipart/form-data to allow for charger image uploads.
// @Tags CPO Network
// @Accept multipart/form-data
// @Produce json
// @Param data formData cpo.CreateChargerRequest true "Charger creation data in JSON format"
// @Param charger_image formData file false "Charger image file"
// @Success 201 {object} cpo.ChargerResponse "Successfully created charger"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers [post]
func (handler *Handler) createCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	record, err := handler.service.CreateCharger(
		ctx,
		principal,
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

// @Summary Get a charger by ID
// @Description Retrieves details for a specific charger, including connector information and the email of the associated user.
// @Tags CPO Network
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Success 200 {object} cpo.ChargerResponse "Successfully retrieved charger details"
// @Failure 400 {object} auth.APIError "Invalid charger ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id} [get]
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

func (handler *Handler) getChargerImage(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID := ctx.Param("charger_id")

	download, err := handler.service.DownloadChargerImage(ctx.Request.Context(), principal, chargerID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	defer download.Content.(io.Closer).Close()

	disposition := mime.FormatMediaType("inline", map[string]string{"filename": download.OriginalName})
	if disposition == "" {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "The request could not be completed.",
		})
		return
	}

	ctx.Header("Content-Disposition", disposition)
	ctx.Header("Content-Type", download.DetectedMIME)
	ctx.Header("Accept-Ranges", "bytes")
	ctx.Header("Cache-Control", "public, max-age=3600")
	ctx.Header("X-Content-Type-Options", "nosniff")

	http.ServeContent(ctx.Writer, ctx.Request, download.OriginalName, download.ModTime, download.Content)
}

// @Summary Update a charger
// @Description Update an existing charger's details. This endpoint uses multipart/form-data to allow for charger image uploads.
// @Tags CPO Network
// @Accept multipart/form-data
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param data formData cpo.UpdateChargerRequest true "Charger update data in JSON format"
// @Param charger_image formData file false "New charger image file"
// @Success 200 {object} cpo.ChargerResponse "Successfully updated charger"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger or associated hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id} [patch]
func (handler *Handler) updateCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	record, err := handler.service.UpdateCharger(
		ctx,
		principal,
		ctx.Param("charger_id"),
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

// @Summary Get a hub by ID
// @Description Get a hub by ID, with an optional paginated list of chargers.
// @Tags CPO Network
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param limit query int false "Number of chargers to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} cpo.HubResponse "Successfully retrieved hub"
// @Failure 400 {object} auth.APIError "Invalid request"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Router /cpo/hubs/{hub_id} [get]
func (handler *Handler) getHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetHub(ctx.Request.Context(), principal, hubID, query)
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

func (handler *Handler) assignChargerToHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	var request AssignChargerRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.AssignChargerToHub(ctx.Request.Context(), principal, hubID, request.ChargerID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
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
