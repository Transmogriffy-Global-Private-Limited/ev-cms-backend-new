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
	group.DELETE("/hubs/:hub_id", handler.deleteHub)
	group.PUT("/hubs/:hub_id/customer-visibility", handler.updateHubCustomerVisibility)
	group.POST("/hubs/:hub_id/chargers", handler.assignChargerToHub)

	// Hub tariffs
	group.POST("/hubs/:hub_id/tariffs", handler.createHubTariff)
	group.GET("/hubs/:hub_id/tariffs", handler.listHubTariffs)
	group.GET("/hubs/:hub_id/tariffs/:tariff_id", handler.getHubTariff)
	group.PATCH("/hubs/:hub_id/tariffs/:tariff_id", handler.updateHubTariff)

	// Charger tariffs
	group.POST("/chargers/:charger_id/tariffs", handler.createChargerTariff)
	group.GET("/chargers/:charger_id/tariffs", handler.listChargerTariffs)
	group.GET("/chargers/:charger_id/tariffs/:tariff_id", handler.getChargerTariff)
	group.PATCH("/chargers/:charger_id/tariffs/:tariff_id", handler.updateChargerTariff)

	// User group tariffs
	group.POST("/user-groups/:user_group_id/tariffs", handler.createUserGroupTariff)
	group.GET("/user-groups/:user_group_id/tariffs", handler.listUserGroupTariffs)
	group.GET("/user-groups/:user_group_id/tariffs/:tariff_id", handler.getUserGroupTariff)
	group.PATCH("/user-groups/:user_group_id/tariffs/:tariff_id", handler.updateUserGroupTariff)

	group.POST("/gsts", handler.createGST)
	group.GET("/gsts", handler.listGSTs)
	group.GET("/gsts/:gst_id", handler.getGST)
	group.PATCH("/gsts/:gst_id", handler.updateGST)
	group.PUT("/chargers/:charger_id/status", handler.updateChargerStatus)
	group.GET("/chargers/:charger_id/status", handler.getChargerStatus)
	group.GET("/customers", handler.listCustomers)
	group.GET("/customers/:customer_id", handler.getCustomer)

	group.POST("/user-groups", handler.createUserGroup)
	group.GET("/user-groups", handler.listUserGroups)
	group.GET("/user-groups/:user_group_id", handler.getUserGroup)
	group.PATCH("/user-groups/:user_group_id", handler.updateUserGroup)
	group.DELETE("/user-groups/:user_group_id", handler.deleteUserGroup)
	group.POST("/user-groups/:user_group_id/members", handler.addMemberToUserGroup)
	group.DELETE("/user-groups/:user_group_id/members/:customer_id", handler.removeMemberFromUserGroup)

	group.GET("/settings", handler.getSettings)
	group.POST("/settings", handler.createOrUpdateSettings)
	group.PUT("/settings", handler.createOrUpdateSettings)
}

func (handler *Handler) getSettings(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	settings, err := handler.service.GetSettings(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

func (handler *Handler) createOrUpdateSettings(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	settings, err := handler.service.CreateOrUpdateSettings(ctx, principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

// @Summary Remove a member from a user group
// @Description Removes a customer from a user group.
// @Tags CPO Network - User Groups
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param customer_id path string true "Customer ID"
// @Success 204 "Successfully removed member from user group"
// @Failure 400 {object} auth.APIError "Invalid parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group or customer not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/members/{customer_id} [delete]
func (handler *Handler) removeMemberFromUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	customerID, ok := parseCustomerID(ctx)
	if !ok {
		return
	}

	err := handler.service.RemoveMemberFromUserGroup(ctx.Request.Context(), principal, userGroupID, customerID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// @Summary Add a member to a user group
// @Description Adds a customer to a user group.
// @Tags CPO Network - User Groups
// @Accept json
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param member body AddMemberToUserGroupRequest true "Customer to add"
// @Success 204 "Successfully added member to user group"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group or customer not found"
// @Failure 409 {object} auth.APIError "Customer is already in a user group"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/members [post]
func (handler *Handler) addMemberToUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}

	var request AddMemberToUserGroupRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	err := handler.service.AddMemberToUserGroup(ctx.Request.Context(), principal, userGroupID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
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

func (handler *Handler) deleteHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	if err := handler.service.DeleteHub(ctx.Request.Context(), principal, hubID); err != nil {
		writeError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
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

// @Summary Create a GST record
// @Description Creates a new GST record for the CPO.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param gst body CreateGSTRequest true "GST creation data"
// @Success 201 {object} GSTView "Successfully created GST record"
// @Failure 400 {object} auth.APIError "Invalid request body"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/gsts [post]
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

// @Summary List GST records
// @Description Lists all GST records for the CPO.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param limit query int false "Number of records to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} GSTListResponse "Successfully retrieved GST records"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/gsts [get]
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

// @Summary Get a GST record
// @Description Retrieves a specific GST record by its ID.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param gst_id path string true "GST ID"
// @Success 200 {object} GSTView "Successfully retrieved GST record"
// @Failure 400 {object} auth.APIError "Invalid GST ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "GST record not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/gsts/{gst_id} [get]
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

// @Summary List CPO customers
// @Description Returns a paginated list of customers for the authenticated CPO, ordered by newest first.
// @Tags CPO Operations - Account & Notifications
// @Produce json
// @Param q query string false "Case-insensitive substring search across customer name, email, and phone"
// @Param status query string false "Filter by customer status (ACTIVE or BLOCKED)"
// @Param limit query int false "Number of records to return (1-200, default 50)"
// @Param before query string false "RFC3339 creation timestamp for keyset pagination"
// @Param before_id query string false "UUID tie-breaker to be paired with before"
// @Success 200 {object} CPOAdminCustomerListResponse "Successfully retrieved customer list"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Security BearerAuth
// @Security CPOAppID
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/customers [get]
func (handler *Handler) listCustomers(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseCPOAdminCustomerListQuery(ctx)
	if !ok {
		return
	}

	records, err := handler.service.ListCustomers(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, records)
}

// @Summary Get CPO customer details
// @Description Returns a single customer belonging to the authenticated CPO.
// @Tags CPO Operations - Account & Notifications
// @Produce json
// @Param customer_id path string true "Customer ID"
// @Success 200 {object} CPOAdminCustomerView "Successfully retrieved customer details"
// @Failure 400 {object} auth.APIError "Invalid customer ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Customer not found"
// @Security BearerAuth
// @Security CPOAppID
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/customers/{customer_id} [get]
func (handler *Handler) getCustomer(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	customerID, ok := parseCustomerID(ctx)
	if !ok {
		return
	}

	record, err := handler.service.GetCustomer(ctx.Request.Context(), principal, customerID)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

func parseCPOAdminCustomerListQuery(ctx *gin.Context) (CPOAdminCustomerListQuery, bool) {
	query := CPOAdminCustomerListQuery{Search: strings.TrimSpace(ctx.Query("q"))}
	if statusText := strings.TrimSpace(ctx.Query("status")); statusText != "" {
		status := constants.CustomerStatus(strings.ToUpper(statusText))
		query.Status = &status
	}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return CPOAdminCustomerListQuery{}, false
		}
		query.Limit = limit
	}
	// Reuse parseTenantListQuery's logic for before/before_id
	tenantQuery, ok := parseTenantListQuery(ctx)
	if !ok {
		return CPOAdminCustomerListQuery{}, false
	}
	query.Before = tenantQuery.Before
	query.BeforeID = tenantQuery.BeforeID
	return query, true
}

// @Summary Update a GST record
// @Description Updates an existing GST record.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param gst_id path string true "GST ID"
// @Param gst body UpdateGSTRequest true "GST update data"
// @Success 200 {object} GSTView "Successfully updated GST record"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "GST record not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/gsts/{gst_id} [patch]
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

func parseCustomerID(ctx *gin.Context) (uuid.UUID, bool) {
	customerID, err := uuid.Parse(ctx.Param("customer_id"))
	if err != nil || customerID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_customer_id",
			Message: "The customer ID is invalid.",
		})
		return uuid.Nil, false
	}
	return customerID, true
}

func parseUserGroupID(ctx *gin.Context) (uuid.UUID, bool) {
	userGroupID, err := uuid.Parse(ctx.Param("user_group_id"))
	if err != nil || userGroupID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_user_group_id",
			Message: "The user group ID is invalid.",
		})
		return uuid.Nil, false
	}
	return userGroupID, true
}

// @Summary Create a user group
// @Description Creates a new user group for the CPO.
// @Tags CPO Network - User Groups
// @Accept json
// @Produce json
// @Param user_group body CreateUserGroupRequest true "User group creation data"
// @Success 201 {object} UserGroupView "Successfully created user group"
// @Failure 400 {object} auth.APIError "Invalid request body"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups [post]
func (handler *Handler) createUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	var request CreateUserGroupRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.CreateUserGroup(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, record)
}

// @Summary List user groups
// @Description Lists all user groups for the CPO.
// @Tags CPO Network - User Groups
// @Produce json
// @Param limit query int false "Number of records to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} UserGroupListResponse "Successfully retrieved user groups"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups [get]
func (handler *Handler) listUserGroups(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListUserGroups(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// @Summary Get a user group
// @Description Retrieves a specific user group by its ID.
// @Tags CPO Network - User Groups
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Success 200 {object} UserGroupView "Successfully retrieved user group"
// @Failure 400 {object} auth.APIError "Invalid user group ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id} [get]
func (handler *Handler) getUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetUserGroup(ctx.Request.Context(), principal, userGroupID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Update a user group
// @Description Updates an existing user group.
// @Tags CPO Network - User Groups
// @Accept json
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param user_group body UpdateUserGroupRequest true "User group update data"
// @Success 200 {object} UserGroupView "Successfully updated user group"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id} [patch]
func (handler *Handler) updateUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	var request UpdateUserGroupRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.UpdateUserGroup(ctx.Request.Context(), principal, userGroupID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Delete a user group
// @Description Deletes a user group by its ID.
// @Tags CPO Network - User Groups
// @Param user_group_id path string true "User Group ID"
// @Success 204 "Successfully deleted user group"
// @Failure 400 {object} auth.APIError "Invalid user group ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id} [delete]
func (handler *Handler) deleteUserGroup(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	err := handler.service.DeleteUserGroup(ctx.Request.Context(), principal, userGroupID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// @Summary Create a hub tariff
// @Description Creates a new tariff for a specific hub.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param tariff body CreateTariffRequest true "Tariff creation data"
// @Success 201 {object} TariffView "Successfully created hub tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/tariffs [post]
func (handler *Handler) createHubTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	var request CreateTariffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.CreateHubTariff(ctx.Request.Context(), principal, hubID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, record)
}

// @Summary List hub tariffs
// @Description Lists all tariffs for a specific hub.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param limit query int false "Number of records to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} TariffListResponse "Successfully retrieved hub tariffs"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/tariffs [get]
func (handler *Handler) listHubTariffs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListHubTariffs(ctx.Request.Context(), principal, hubID, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// @Summary Get a hub tariff
// @Description Retrieves a specific tariff by its ID for a specific hub.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 200 {object} TariffView "Successfully retrieved hub tariff"
// @Failure 400 {object} auth.APIError "Invalid ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/tariffs/{tariff_id} [get]
func (handler *Handler) getHubTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetHubTariff(ctx.Request.Context(), principal, hubID, tariffID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Update a hub tariff
// @Description Updates an existing tariff for a specific hub.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param tariff_id path string true "Tariff ID"
// @Param tariff body UpdateTariffRequest true "Tariff update data"
// @Success 200 {object} TariffView "Successfully updated hub tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/tariffs/{tariff_id} [patch]
func (handler *Handler) updateHubTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
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
	record, err := handler.service.UpdateHubTariff(ctx.Request.Context(), principal, hubID, tariffID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Create a charger tariff
// @Description Creates a new tariff for a specific charger.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param tariff body CreateTariffRequest true "Tariff creation data"
// @Success 201 {object} TariffView "Successfully created charger tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/tariffs [post]
func (handler *Handler) createChargerTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}
	var request CreateTariffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.CreateChargerTariff(ctx.Request.Context(), principal, chargerID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, record)
}

// @Summary List charger tariffs
// @Description Lists all tariffs for a specific charger.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param limit query int false "Number of records to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} TariffListResponse "Successfully retrieved charger tariffs"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/tariffs [get]
func (handler *Handler) listChargerTariffs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListChargerTariffs(ctx.Request.Context(), principal, chargerID, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// @Summary Get a charger tariff
// @Description Retrieves a specific tariff by its ID for a specific charger.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 200 {object} TariffView "Successfully retrieved charger tariff"
// @Failure 400 {object} auth.APIError "Invalid ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/tariffs/{tariff_id} [get]
func (handler *Handler) getChargerTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetChargerTariff(ctx.Request.Context(), principal, chargerID, tariffID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Update a charger tariff
// @Description Updates an existing tariff for a specific charger.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param tariff_id path string true "Tariff ID"
// @Param tariff body UpdateTariffRequest true "Tariff update data"
// @Success 200 {object} TariffView "Successfully updated charger tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/tariffs/{tariff_id} [patch]
func (handler *Handler) updateChargerTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}
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
	record, err := handler.service.UpdateChargerTariff(ctx.Request.Context(), principal, chargerID, tariffID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Create a user group tariff
// @Description Creates a new tariff for a specific user group.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param tariff body CreateTariffRequest true "Tariff creation data"
// @Success 201 {object} TariffView "Successfully created user group tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/tariffs [post]
func (handler *Handler) createUserGroupTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	var request CreateTariffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.CreateUserGroupTariff(ctx.Request.Context(), principal, userGroupID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, record)
}

// @Summary List user group tariffs
// @Description Lists all tariffs for a specific user group.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param limit query int false "Number of records to return"
// @Param before query string false "Timestamp for pagination"
// @Param before_id query string false "ID for pagination"
// @Success 200 {object} TariffListResponse "Successfully retrieved user group tariffs"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/tariffs [get]
func (handler *Handler) listUserGroupTariffs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	query, ok := parseTenantListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListUserGroupTariffs(ctx.Request.Context(), principal, userGroupID, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// @Summary Get a user group tariff
// @Description Retrieves a specific tariff by its ID for a specific user group.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 200 {object} TariffView "Successfully retrieved user group tariff"
// @Failure 400 {object} auth.APIError "Invalid ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/tariffs/{tariff_id} [get]
func (handler *Handler) getUserGroupTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetUserGroupTariff(ctx.Request.Context(), principal, userGroupID, tariffID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Update a user group tariff
// @Description Updates an existing tariff for a specific user group.
// @Tags CPO Network - Tariffs
// @Accept json
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param tariff_id path string true "Tariff ID"
// @Param tariff body UpdateTariffRequest true "Tariff update data"
// @Success 200 {object} TariffView "Successfully updated user group tariff"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group or tariff not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/user-groups/{user_group_id}/tariffs/{tariff_id} [patch]
func (handler *Handler) updateUserGroupTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
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
	record, err := handler.service.UpdateUserGroupTariff(ctx.Request.Context(), principal, userGroupID, tariffID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}
