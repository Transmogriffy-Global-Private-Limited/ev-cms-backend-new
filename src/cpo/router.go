package cpo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Handler struct {
	service     *Service
	authService *auth.Service
}

func RegisterPlatformRoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{service: service, authService: authService}
	group.Use(cmsmiddleware.NoStore, authService.Authenticate(), auth.RequirePlatform())
	group.POST("", handler.create)
	group.GET("", handler.list)
	group.GET("/slug-availability", handler.slugAvailability)
	group.GET("/:cpo_id", handler.get)
	group.GET("/:cpo_id/operations/fleet", handler.getPlatformFleetOperations)
	group.GET("/:cpo_id/operations/chargers/:charger_id", handler.getPlatformOperationalCharger)
	group.GET("/:cpo_id/operations/events", handler.listPlatformOperationalEvents)
	group.GET("/:cpo_id/operations/realtime/stream", handler.platformOperationalStream)
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

func RegisterCPORoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{service: service, authService: authService}

	group.Use(
		cmsmiddleware.NoStore,
		authService.Authenticate(),
		auth.RequireCPOAppID(),
	)
	by := func(permission string) *gin.RouterGroup {
		return group.Group("", auth.RequireCPOPermission(service.database, permission))
	}
	access := group.Group("/access", auth.RequireActiveCPOMembership(service.database))
	organizationRead := by(cpopermissions.OrganizationRead)
	organizationManage := by(cpopermissions.OrganizationManage)
	staffRead := by(cpopermissions.StaffRead)
	staffManage := by(cpopermissions.StaffManage)
	staffPermissions := by(cpopermissions.StaffPermissionsManage)
	staffDelegation := group.Group("", auth.RequireCPOPermission(service.database, cpopermissions.StaffManage), auth.RequireCPOPermission(service.database, cpopermissions.StaffPermissionsManage))
	hubsRead := by(cpopermissions.HubsRead)
	hubsManage := by(cpopermissions.HubsManage)
	chargersRead := by(cpopermissions.ChargersRead)
	chargersManage := by(cpopermissions.ChargersManage)
	tariffsRead := by(cpopermissions.TariffsRead)
	tariffsManage := by(cpopermissions.TariffsManage)
	customersRead := by(cpopermissions.CustomersRead)
	sessionsRead := by(cpopermissions.ChargingSessionsRead)
	tracesRead := by(cpopermissions.ChargingTracesRead)
	analyticsRead := by(cpopermissions.AnalyticsRead)
	operations := by(cpopermissions.ChargersOperations)
	settingsRead := by(cpopermissions.SettingsRead)
	settingsManage := by(cpopermissions.SettingsManage)

	organizationRead.GET("/admin/profile", handler.getAdminProfile)
	organizationManage.PATCH("/admin/profile", handler.updateAdminProfile)
	organizationRead.GET("/organization", handler.getOrganization)
	organizationRead.GET("/subscription", handler.getSubscription)
	analyticsRead.GET("/analytics", handler.getAnalytics)
	operations.GET("/operations/fleet", handler.getFleetOperations)
	operations.GET("/operations/chargers/:charger_id", handler.getOperationalCharger)
	operations.GET("/operations/events", handler.listOperationalEvents)
	operations.GET("/operations/realtime/stream", handler.operationalStream)
	operations.GET("/operations/live-sessions", handler.liveChargingSessionsStream)
	operations.GET("/operations/live-sessions/snapshot", handler.listLiveChargingSessions)
	operations.GET("/operations/live-sessions/events", handler.listLiveChargingSessionEvents)
	operations.GET("/operations/live-sessions/realtime/stream", handler.liveChargingSessionsStream)
	staffRead.GET("/users/:user_id", handler.getUser)
	staffRead.GET("/permissions/catalog", handler.permissionCatalog)
	access.GET("/permissions", handler.permissionCatalog)
	access.GET("/me", handler.accessMe)
	staffRead.GET("/staff", handler.listStaff)
	staffDelegation.POST("/staff", handler.createStaff)
	staffRead.GET("/staff/:membership_id", handler.getStaff)
	staffPermissions.PATCH("/staff/:membership_id", handler.updateStaff)
	staffManage.POST("/staff/:membership_id/activate", handler.activateStaff)
	staffManage.POST("/staff/:membership_id/suspend", handler.suspendStaff)
	staffManage.POST("/staff/:membership_id/revoke", handler.revokeStaff)
	chargersManage.POST("/chargers", handler.createCharger)
	chargersRead.GET("/chargers", handler.listChargers)
	chargersRead.GET("/chargers/:charger_id", handler.getCharger)
	chargersRead.GET("/chargers/:charger_id/image", handler.getChargerImage)
	chargersManage.PATCH("/chargers/:charger_id", handler.updateCharger)
	chargersManage.DELETE("/chargers/:charger_id", handler.deleteCharger)
	hubsManage.POST("/hubs", handler.createHub)
	hubsRead.GET("/hubs", handler.listHubs)
	hubsRead.GET("/hubs/:hub_id", handler.getHub)
	hubsManage.PATCH("/hubs/:hub_id", handler.updateHub)
	hubsManage.DELETE("/hubs/:hub_id", handler.deleteHub)
	hubsManage.PUT("/hubs/:hub_id/customer-visibility", handler.updateHubCustomerVisibility)
	chargersManage.PUT("/chargers/:charger_id/customer-visibility", handler.updateChargerCustomerVisibility)
	hubsManage.POST("/hubs/:hub_id/chargers", handler.assignChargerToHub)
	hubsManage.POST("/hubs/:hub_id/gst", handler.assignGSTToHub)
	hubsRead.GET("/hubs/:hub_id/gst", handler.getGSTForHub)
	hubsManage.PATCH("/hubs/:hub_id/gst", handler.updateGSTForHub)
	hubsManage.DELETE("/hubs/:hub_id/gst", handler.unassignGSTFromHub)
	hubsRead.GET("/hubs/:hub_id/chargers", handler.listChargersByHub)

	// Hub tariffs
	tariffsManage.POST("/hubs/:hub_id/tariffs", handler.createHubTariff)
	tariffsRead.GET("/hubs/:hub_id/tariffs", handler.listHubTariffs)
	tariffsRead.GET("/hubs/:hub_id/tariffs/:tariff_id", handler.getHubTariff)
	tariffsManage.PATCH("/hubs/:hub_id/tariffs/:tariff_id", handler.updateHubTariff)
	tariffsManage.DELETE("/hubs/:hub_id/tariffs/:tariff_id", handler.deleteHubTariff)

	// Charger tariffs
	tariffsManage.POST("/chargers/:charger_id/tariffs", handler.createChargerTariff)
	tariffsRead.GET("/chargers/:charger_id/tariffs", handler.listChargerTariffs)
	tariffsRead.GET("/chargers/:charger_id/tariffs/:tariff_id", handler.getChargerTariff)
	tariffsManage.PATCH("/chargers/:charger_id/tariffs/:tariff_id", handler.updateChargerTariff)
	tariffsManage.DELETE("/chargers/:charger_id/tariffs/:tariff_id", handler.deleteChargerTariff)

	// User group tariffs
	tariffsManage.POST("/user-groups/:user_group_id/tariffs", handler.createUserGroupTariff)
	tariffsRead.GET("/user-groups/:user_group_id/tariffs", handler.listUserGroupTariffs)
	tariffsRead.GET("/user-groups/:user_group_id/tariffs/:tariff_id", handler.getUserGroupTariff)
	tariffsManage.PATCH("/user-groups/:user_group_id/tariffs/:tariff_id", handler.updateUserGroupTariff)
	tariffsManage.DELETE("/user-groups/:user_group_id/tariffs/:tariff_id", handler.deleteUserGroupTariff)

	tariffsManage.POST("/gsts", handler.createGST)
	tariffsRead.GET("/gsts", handler.listGSTs)
	tariffsRead.GET("/gsts/:gst_id", handler.getGST)
	tariffsManage.PATCH("/gsts/:gst_id", handler.updateGST)
	chargersManage.PUT("/chargers/:charger_id/status", handler.updateChargerStatus)
	chargersRead.GET("/chargers/:charger_id/status", handler.getChargerStatus)
	customersRead.GET("/customers", handler.listCustomers)
	customersRead.GET("/customers/:customer_id", handler.getCustomer)

	tariffsManage.POST("/user-groups", handler.createUserGroup)
	tariffsRead.GET("/user-groups", handler.listUserGroups)
	tariffsRead.GET("/user-groups/:user_group_id", handler.getUserGroup)
	tariffsManage.PATCH("/user-groups/:user_group_id", handler.updateUserGroup)
	tariffsManage.DELETE("/user-groups/:user_group_id", handler.deleteUserGroup)
	tariffsManage.POST("/user-groups/:user_group_id/members", handler.addMemberToUserGroup)
	tariffsManage.DELETE("/user-groups/:user_group_id/members/:customer_id", handler.removeMemberFromUserGroup)

	settingsRead.GET("/settings", handler.getSettings)
	settingsManage.POST("/settings", handler.createOrUpdateSettings)
	settingsManage.PUT("/settings", handler.createOrUpdateSettings)
	settingsRead.GET("/settings/invoice-logo", handler.getInvoiceLogo)
	sessionsRead.GET("/charging-sessions", handler.listChargingSessions)
	sessionsRead.GET("/charging-sessions/:session_id", handler.getChargingSession)
	tracesRead.GET("/charging-sessions/:session_id/trace", handler.getChargingSessionTrace)
	tracesRead.GET("/charging-traces/:trace_id", handler.getChargingTrace)
	tracesRead.GET("/charging-traces/:trace_id/stream", handler.chargingTraceStream)
	sessionsRead.GET("/charger-transactions", handler.listChargerTransactions)
	customersRead.GET("/wallet-transactions", handler.listWalletTransactions)
	customersRead.GET("/customers/:customer_id/wallet-transactions", handler.listCustomerWalletTransactions)
	analyticsRead.GET("/hubs/:hub_id/analytics", handler.getHubAnalytics)

}

func (handler *Handler) getChargingSessionTrace(ctx *gin.Context) {
	handler.getChargingTraceFor(ctx, true)
}
func (handler *Handler) getChargingTrace(ctx *gin.Context) { handler.getChargingTraceFor(ctx, false) }
func (handler *Handler) getChargingTraceFor(ctx *gin.Context, bySession bool) {
	var sessionID, traceID uuid.UUID
	var err error
	if bySession {
		sessionID, err = uuid.Parse(ctx.Param("session_id"))
	} else {
		traceID, err = uuid.Parse(ctx.Param("trace_id"))
	}
	if err != nil || (bySession && sessionID == uuid.Nil) || (!bySession && traceID == uuid.Nil) {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "A canonical UUID is required."})
		return
	}
	limit := 50
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Limit must be an integer between 1 and 100."})
			return
		}
		limit = parsed
	}
	var before *time.Time
	var beforeID *uuid.UUID
	if raw := ctx.Query("before_occurred_at"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		cursor, uuidErr := uuid.Parse(ctx.Query("before_event_id"))
		if parseErr != nil || uuidErr != nil || cursor == uuid.Nil {
			writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "A valid trace cursor is required."})
			return
		}
		before = &parsed
		beforeID = &cursor
	} else if strings.TrimSpace(ctx.Query("before_event_id")) != "" {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Before occurred at is required when before event ID is supplied."})
		return
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.GetChargingTrace(ctx.Request.Context(), principal, sessionID, traceID, before, beforeID, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) chargingTraceStream(ctx *gin.Context) {
	traceID, err := uuid.Parse(ctx.Param("trace_id"))
	if err != nil || traceID == uuid.Nil {
		writeError(ctx, invalid("trace_id", "A canonical trace UUID is required."))
		return
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	token, ok := auth.CurrentAccessToken(ctx)
	if !ok || principal.CPOID == nil {
		writeError(ctx, &auth.APIError{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "Authentication is required."})
		return
	}
	after := int64(0)
	raw := strings.TrimSpace(ctx.Query("after"))
	if raw == "" {
		raw = strings.TrimSpace(ctx.GetHeader("Last-Event-ID"))
	}
	if raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed < 0 {
			writeError(ctx, invalid("after", "After must be a non-negative trace replay cursor."))
			return
		}
		after = parsed
	}
	appID := ctx.GetHeader(auth.CPOAppIDHeader)
	poll, heartbeat, batch := handler.service.OperationalStreamTiming()
	page, err := handler.service.ListChargingTraceReplay(ctx.Request.Context(), principal, traceID, after, batch)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{})
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		for _, event := range page.Events {
			payload, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return
			}
			if _, writeErr := fmt.Fprintf(ctx.Writer, "id: %d\nevent: trace_event\ndata: %s\n\n", event.IngestionSequence, payload); writeErr != nil {
				return
			}
		}
		if len(page.Events) > 0 {
			after = page.NextCursor
			ctx.Writer.Flush()
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			page, err = handler.service.ListChargingTraceReplay(ctx.Request.Context(), principal, traceID, after, batch)
			if err != nil {
				return
			}
		case <-heartbeatTicker.C:
			refreshed, checkErr := handler.authService.ValidateAccess(ctx.Request.Context(), token)
			if checkErr != nil || !cpoStreamStillAuthorized(ctx.Request.Context(), handler.service.database, refreshed, *principal.CPOID, appID, cpopermissions.ChargingTracesRead) {
				return
			}
			if _, writeErr := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); writeErr != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func (handler *Handler) permissionCatalog(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.PermissionCatalog(principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) accessMe(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.AccessMe(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) listStaff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	record, err := handler.service.ListStaff(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) createStaff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request CreateStaffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalid("request", "The request body is invalid."))
		return
	}
	record, err := handler.service.CreateStaff(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, record)
}

func (handler *Handler) getStaff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	membershipID, ok := parseMembershipID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetStaff(ctx.Request.Context(), principal, membershipID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) updateStaff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	membershipID, ok := parseMembershipID(ctx)
	if !ok {
		return
	}
	var request UpdateStaffRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalid("request", "The request body is invalid."))
		return
	}
	record, err := handler.service.UpdateStaff(ctx.Request.Context(), principal, membershipID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) activateStaff(ctx *gin.Context) {
	handler.transitionStaff(ctx, constants.MembershipStatusActive)
}
func (handler *Handler) suspendStaff(ctx *gin.Context) {
	handler.transitionStaff(ctx, constants.MembershipStatusSuspended)
}
func (handler *Handler) revokeStaff(ctx *gin.Context) {
	handler.transitionStaff(ctx, constants.MembershipStatusRevoked)
}

func (handler *Handler) transitionStaff(ctx *gin.Context, status constants.MembershipStatus) {
	principal, _ := auth.CurrentPrincipal(ctx)
	membershipID, ok := parseMembershipID(ctx)
	if !ok {
		return
	}
	var request StaffLifecycleRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalid("request", "The request body is invalid."))
		return
	}
	record, err := handler.service.TransitionStaff(ctx.Request.Context(), principal, membershipID, status, request.Reason)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func parseMembershipID(ctx *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(ctx.Param("membership_id"))
	if err != nil || id == uuid.Nil {
		writeError(ctx, invalid("membership_id", "Membership ID is invalid."))
		return uuid.Nil, false
	}
	return id, true
}

func (handler *Handler) listChargingSessions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseChargingSessionListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListChargingSessions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

func (handler *Handler) listLiveChargingSessions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseLiveChargingSessionListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

func (handler *Handler) listChargerTransactions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseChargerTransactionListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListChargerTransactions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

func parseChargerTransactionListQuery(ctx *gin.Context) (ChargerTransactionListQuery, bool) {
	query := ChargerTransactionListQuery{}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return ChargerTransactionListQuery{}, false
		}
		query.Limit = limit
	}
	if beforeText := strings.TrimSpace(ctx.Query("before")); beforeText != "" {
		before, err := time.Parse(time.RFC3339, beforeText)
		if err != nil {
			writeError(
				ctx,
				invalid("before", "Before must be an RFC3339 timestamp."),
			)
			return ChargerTransactionListQuery{}, false
		}
		query.Before = &before
	}
	if beforeIDText := strings.TrimSpace(ctx.Query("before_id")); beforeIDText != "" {
		beforeID, err := uuid.Parse(beforeIDText)
		if err != nil || beforeID == uuid.Nil {
			writeError(
				ctx,
				invalid("before_id", "Before ID must be a non-zero UUID."),
			)
			return ChargerTransactionListQuery{}, false
		}
		query.BeforeID = &beforeID
	}
	if chargerIDText := strings.TrimSpace(ctx.Query("charger_id")); chargerIDText != "" {
		chargerID, err := uuid.Parse(chargerIDText)
		if err != nil || chargerID == uuid.Nil {
			writeError(
				ctx,
				invalid("charger_id", "Charger ID must be a non-zero UUID."),
			)
			return ChargerTransactionListQuery{}, false
		}
		query.ChargerID = &chargerID
	}
	if customerIDText := strings.TrimSpace(ctx.Query("customer_id")); customerIDText != "" {
		customerID, err := uuid.Parse(customerIDText)
		if err != nil || customerID == uuid.Nil {
			writeError(
				ctx,
				invalid("customer_id", "Customer ID must be a non-zero UUID."),
			)
			return ChargerTransactionListQuery{}, false
		}
		query.CustomerID = &customerID
	}
	return query, true
}

// @Summary Get CPO analytics
// @Description Get CPO analytics data.
// @Tags CPO Network
// @Produce json
// @Success 200 {object} AnalyticsResponse "Returns CPO analytics data, including total chargers, connectors, revenue, energy usage, and number of sessions."
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Security BearerAuth
// @Security CPOAppID
// @Router /cpo/analytics [get]
// getAnalytics handles GET /api/v1/cpo/analytics
func (handler *Handler) getAnalytics(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	// Parse optional period/date query parameters
	var query AnalyticsQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		writeError(ctx, invalid("query", "Invalid query parameters"))
		return
	}

	// Call service with the parsed query
	record, err := handler.service.GetAnalytics(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (handler *Handler) getChargingSession(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	sessionID, ok := parseSessionID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetChargingSession(ctx.Request.Context(), principal, sessionID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func parseSessionID(ctx *gin.Context) (uuid.UUID, bool) {
	sessionID, err := uuid.Parse(ctx.Param("session_id"))
	if err != nil || sessionID == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_session_id",
			Message: "The session ID is invalid.",
		})
		return uuid.Nil, false
	}
	return sessionID, true
}

func parseChargingSessionListQuery(ctx *gin.Context) (ChargingSessionListQuery, bool) {
	query := ChargingSessionListQuery{}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return ChargingSessionListQuery{}, false
		}
		query.Limit = limit
	}
	if beforeText := strings.TrimSpace(ctx.Query("before")); beforeText != "" {
		before, err := time.Parse(time.RFC3339, beforeText)
		if err != nil {
			writeError(
				ctx,
				invalid("before", "Before must be an RFC3339 timestamp."),
			)
			return ChargingSessionListQuery{}, false
		}
		query.Before = &before
	}
	if beforeIDText := strings.TrimSpace(ctx.Query("before_id")); beforeIDText != "" {
		beforeID, err := uuid.Parse(beforeIDText)
		if err != nil || beforeID == uuid.Nil {
			writeError(
				ctx,
				invalid("before_id", "Before ID must be a non-zero UUID."),
			)
			return ChargingSessionListQuery{}, false
		}
		query.BeforeID = &beforeID
	}
	if statusText := strings.TrimSpace(ctx.Query("status")); statusText != "" {
		status := constants.SessionStatus(strings.ToUpper(statusText))
		if !status.Valid() {
			writeError(ctx, invalid("status", "Status is invalid."))
			return ChargingSessionListQuery{}, false
		}
		query.Status = &status
	}
	if chargerIDText := strings.TrimSpace(ctx.Query("charger_id")); chargerIDText != "" {
		chargerID, err := uuid.Parse(chargerIDText)
		if err != nil || chargerID == uuid.Nil {
			writeError(
				ctx,
				invalid("charger_id", "Charger ID must be a non-zero UUID."),
			)
			return ChargingSessionListQuery{}, false
		}
		query.ChargerID = &chargerID
	}
	if customerIDText := strings.TrimSpace(ctx.Query("customer_id")); customerIDText != "" {
		customerID, err := uuid.Parse(customerIDText)
		if err != nil || customerID == uuid.Nil {
			writeError(
				ctx,
				invalid("customer_id", "Customer ID must be a non-zero UUID."),
			)
			return ChargingSessionListQuery{}, false
		}
		query.CustomerID = &customerID
	}
	return query, true
}

func parseLiveChargingSessionListQuery(ctx *gin.Context) (LiveChargingSessionListQuery, bool) {
	query := LiveChargingSessionListQuery{}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return LiveChargingSessionListQuery{}, false
		}
		query.Limit = limit
	}
	if afterStartedAtText := strings.TrimSpace(ctx.Query("after_started_at")); afterStartedAtText != "" {
		afterStartedAt, err := time.Parse(time.RFC3339, afterStartedAtText)
		if err != nil {
			writeError(ctx, invalid("after_started_at", "After started at must be an RFC3339 timestamp."))
			return LiveChargingSessionListQuery{}, false
		}
		query.AfterStartedAt = &afterStartedAt
	}
	if afterIDText := strings.TrimSpace(ctx.Query("after_id")); afterIDText != "" {
		afterID, err := uuid.Parse(afterIDText)
		if err != nil || afterID == uuid.Nil {
			writeError(ctx, invalid("after_id", "After ID must be a non-zero UUID."))
			return LiveChargingSessionListQuery{}, false
		}
		query.AfterID = &afterID
	}
	if query.AfterID != nil && query.AfterStartedAt == nil {
		writeError(ctx, invalid("after_started_at", "After started at is required when after ID is supplied."))
		return LiveChargingSessionListQuery{}, false
	}
	return query, true
}

func (handler *Handler) getPlatformFleetOperations(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	response, err := handler.service.GetPlatformFleetOperations(ctx.Request.Context(), principal, cpoID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getPlatformOperationalCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	response, err := handler.service.GetPlatformOperationalCharger(ctx.Request.Context(), principal, cpoID, ctx.Param("charger_id"))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getFleetOperations(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.GetFleetOperations(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getOperationalCharger(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.GetOperationalCharger(ctx.Request.Context(), principal, ctx.Param("charger_id"))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) listOperationalEvents(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListOperationalEvents(ctx.Request.Context(), principal, after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) listLiveChargingSessionEvents(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListLiveChargingSessionEvents(ctx.Request.Context(), principal, after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) listPlatformOperationalEvents(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListPlatformOperationalEvents(ctx.Request.Context(), principal, cpoID, after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) operationalStream(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	token, ok := auth.CurrentAccessToken(ctx)
	if !ok || principal.CPOID == nil {
		writeError(ctx, &auth.APIError{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "Authentication is required."})
		return
	}
	appID := ctx.GetHeader(auth.CPOAppIDHeader)
	handler.streamOperationalEvents(ctx, func(after int64, limit int) (operationalrealtime.Page, error) {
		return handler.service.ListOperationalEvents(ctx.Request.Context(), principal, after, limit)
	}, func() bool {
		refreshed, err := handler.authService.ValidateAccess(ctx.Request.Context(), token)
		return err == nil && cpoStreamStillAuthorized(ctx.Request.Context(), handler.service.database, refreshed, *principal.CPOID, appID, cpopermissions.ChargersOperations)
	})
}

func (handler *Handler) liveChargingSessionsStream(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	token, ok := auth.CurrentAccessToken(ctx)
	if !ok || principal.CPOID == nil {
		writeError(ctx, &auth.APIError{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "Authentication is required."})
		return
	}
	query, ok := parseLiveChargingSessionStreamQuery(ctx)
	if !ok {
		return
	}
	appID := ctx.GetHeader(auth.CPOAppIDHeader)

	// Establish the watermark before reading state. A commit after this point
	// can cause a harmless redundant projection refresh, but cannot be skipped
	// between the initial snapshot and the first event query.
	cursor, err := handler.service.LatestLiveChargingSessionEventID(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	// A live-session SSE connection is a full, replaceable CMS projection, not
	// an invalidation feed that asks the client to reconstruct operational state.
	// The durable event log is used only to notice committed projection changes.
	snapshot, err := handler.service.ListLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	lastFingerprint, err := cpoLiveSessionSnapshotFingerprint(snapshot)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{})
	if err := writeLiveSessionSnapshot(ctx.Writer, "snapshot", cursor, snapshot); err != nil {
		return
	}
	ctx.Writer.Flush()

	poll, heartbeat, batchSize := handler.service.OperationalStreamTiming()
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			page, err := handler.service.ListLiveChargingSessionEvents(ctx.Request.Context(), principal, cursor, batchSize)
			if err != nil || len(page.Events) == 0 {
				continue
			}
			cursor = page.NextCursor
			snapshot, lastFingerprint, err = handler.refreshCPOLiveSessionSnapshot(ctx, principal, query, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
		case <-heartbeatTicker.C:
			refreshed, err := handler.authService.ValidateAccess(ctx.Request.Context(), token)
			if err != nil || !cpoStreamStillAuthorized(ctx.Request.Context(), handler.service.database, refreshed, *principal.CPOID, appID, cpopermissions.ChargersOperations) {
				return
			}
			snapshot, lastFingerprint, err = handler.refreshCPOLiveSessionSnapshot(ctx, principal, query, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func (handler *Handler) refreshCPOLiveSessionSnapshot(ctx *gin.Context, principal auth.Principal, query LiveChargingSessionListQuery, cursor int64, previous LiveChargingSessionFinancialListResponse, previousFingerprint []byte) (LiveChargingSessionFinancialListResponse, []byte, error) {
	next, err := handler.service.ListLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal, query)
	if err != nil {
		return previous, previousFingerprint, err
	}
	fingerprint, err := cpoLiveSessionSnapshotFingerprint(next)
	if err != nil {
		return previous, previousFingerprint, err
	}
	if bytes.Equal(previousFingerprint, fingerprint) {
		return next, fingerprint, nil
	}
	if err := writeLiveSessionSnapshot(ctx.Writer, "live_sessions", cursor, next); err != nil {
		return previous, previousFingerprint, err
	}
	ctx.Writer.Flush()
	return next, fingerprint, nil
}

func cpoLiveSessionSnapshotFingerprint(snapshot LiveChargingSessionFinancialListResponse) ([]byte, error) {
	snapshot.AsOf = time.Time{}
	return json.Marshal(snapshot)
}

func parseLiveChargingSessionStreamQuery(ctx *gin.Context) (LiveChargingSessionListQuery, bool) {
	query, ok := parseLiveChargingSessionListQuery(ctx)
	if !ok {
		return LiveChargingSessionListQuery{}, false
	}
	if query.AfterStartedAt != nil || query.AfterID != nil {
		writeError(ctx, invalid("cursor", "The live-session stream always sends the current snapshot and does not accept a page cursor."))
		return LiveChargingSessionListQuery{}, false
	}
	return query, true
}

func writeLiveSessionSnapshot(writer io.Writer, eventType string, eventID int64, snapshot any) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", eventID, eventType, payload)
	return err
}

func cpoStreamStillAuthorized(ctx context.Context, database *gorm.DB, principal auth.Principal, cpoID uuid.UUID, appID, permission string) bool {
	if principal.Scope != constants.AuthScopeCPO || principal.CPOID == nil || *principal.CPOID != cpoID || principal.CPOAppID == nil || *principal.CPOAppID != appID {
		return false
	}
	_, allowed, err := auth.EvaluateCPOPermission(ctx, database, principal, permission)
	return err == nil && allowed
}

func (handler *Handler) platformOperationalStream(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoID, ok := parseCPOID(ctx)
	if !ok {
		return
	}
	token, ok := auth.CurrentAccessToken(ctx)
	if !ok {
		writeError(ctx, &auth.APIError{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "Authentication is required."})
		return
	}
	handler.streamOperationalEvents(ctx, func(after int64, limit int) (operationalrealtime.Page, error) {
		return handler.service.ListPlatformOperationalEvents(ctx.Request.Context(), principal, cpoID, after, limit)
	}, func() bool {
		refreshed, err := handler.authService.ValidateAccess(ctx.Request.Context(), token)
		return err == nil && refreshed.Scope == "PLATFORM"
	})
}

func (handler *Handler) streamOperationalEvents(ctx *gin.Context, list func(int64, int) (operationalrealtime.Page, error), revalidate func() bool) {
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := list(after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{})
	cursor := page.NextCursor
	if err := operationalrealtime.WriteSSE(ctx.Writer, page.Events); err != nil {
		return
	}
	ctx.Writer.Flush()
	poll, heartbeat, batchSize := handler.service.OperationalStreamTiming()
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			page, err := list(cursor, batchSize)
			if err != nil || len(page.Events) == 0 {
				continue
			}
			if err := operationalrealtime.WriteSSE(ctx.Writer, page.Events); err != nil {
				return
			}
			cursor = page.NextCursor
			ctx.Writer.Flush()
		case <-heartbeatTicker.C:
			if !revalidate() {
				return
			}
			if _, err := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func parseOperationalEventQuery(ctx *gin.Context) (int64, int, bool) {
	after := ctx.Query("after_id")
	if strings.TrimSpace(after) == "" {
		after = ctx.GetHeader("Last-Event-ID")
	}
	parsedAfter, limit, err := operationalrealtime.ParseCursor(after, ctx.Query("limit"))
	if err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_event_query", Message: "The event cursor or limit is invalid."})
		return 0, 0, false
	}
	return parsedAfter, limit, true
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

	correlationID := uuid.NewString()
	if requestID, ok := cmsmiddleware.RequestID(ctx); ok {
		correlationID = requestID
	}
	record, err := handler.service.updateChargerStatus(ctx.Request.Context(), principal, chargerID, request, correlationID)
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

// getInvoiceLogo serves the invoice logo image.
func (handler *Handler) getInvoiceLogo(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)

	download, err := handler.service.DownloadInvoiceLogo(ctx.Request.Context(), principal)
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

func (handler *Handler) assignGSTToHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	var request AssignGSTToHubRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.AssignGSTToHub(ctx.Request.Context(), principal, hubID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Get the assigned GST for a hub
// @Description Retrieves the assigned GST for a specific hub.
// @Tags CPO Network
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Success 200 {object} GSTView "Successfully retrieved GST"
// @Failure 400 {object} auth.APIError "Invalid hub ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub or GST not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/gst [get]
func (handler *Handler) getGSTForHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.GetGSTForHub(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Update the assigned GST for a hub
// @Description Updates the assigned GST for a specific hub.
// @Tags CPO Network
// @Accept json
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param gst body AssignGSTToHubRequest true "GST assignment data"
// @Success 200 {object} HubView "Successfully updated GST assignment"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub or GST not found"
// @Failure 409 {object} auth.APIError "GST already assigned to another hub"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/gst [patch]
func (handler *Handler) updateGSTForHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	var request AssignGSTToHubRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}
	record, err := handler.service.UpdateGSTForHub(ctx.Request.Context(), principal, hubID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary Unassign GST from a hub
// @Description Unassigns the GST from a specific hub.
// @Tags CPO Network
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Success 200 {object} HubView "Successfully unassigned GST"
// @Failure 400 {object} auth.APIError "Invalid hub ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/gst [delete]
func (handler *Handler) unassignGSTFromHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	record, err := handler.service.UnassignGSTFromHub(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

// @Summary List chargers by hub
// @Description Get a list of chargers for a specific hub.
// @Tags CPO Network
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Success 200 {object} ChargerListResponse "Successfully retrieved chargers"
// @Failure 400 {object} auth.APIError "Invalid hub ID format"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/hubs/{hub_id}/chargers [get]
func (handler *Handler) listChargersByHub(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListChargersByHub(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
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

	records, err := handler.service.ListCustomersWithCurrentUsage(ctx.Request.Context(), principal, query)
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

	record, err := handler.service.GetCustomerWithCurrentUsage(ctx.Request.Context(), principal, customerID)
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

// @Summary Delete a hub tariff
// @Description Deletes one unreferenced hub tariff after validating the resulting temporal hierarchy and published-Hub tariff floor.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param hub_id path string true "Hub ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 204 "Tariff deleted"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Hub or tariff not found"
// @Failure 409 {object} auth.APIError "Tariff topology, floor, or history conflict"
// @Router /cpo/hubs/{hub_id}/tariffs/{tariff_id} [delete]
func (handler *Handler) deleteHubTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteHubTariff(ctx.Request.Context(), principal, hubID, tariffID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
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

// @Summary Delete a charger tariff
// @Description Deletes one unreferenced charger tariff after validating the resulting temporal hierarchy.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 204 "Tariff deleted"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger or tariff not found"
// @Failure 409 {object} auth.APIError "Tariff topology or history conflict"
// @Router /cpo/chargers/{charger_id}/tariffs/{tariff_id} [delete]
func (handler *Handler) deleteChargerTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteChargerTariff(ctx.Request.Context(), principal, chargerID, tariffID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
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

// @Summary Delete a user-group tariff
// @Description Deletes one unreferenced user-group tariff after validating the resulting temporal hierarchy.
// @Tags CPO Network - Tariffs
// @Produce json
// @Param user_group_id path string true "User Group ID"
// @Param tariff_id path string true "Tariff ID"
// @Success 204 "Tariff deleted"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "User group or tariff not found"
// @Failure 409 {object} auth.APIError "Tariff topology or history conflict"
// @Router /cpo/user-groups/{user_group_id}/tariffs/{tariff_id} [delete]
func (handler *Handler) deleteUserGroupTariff(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userGroupID, ok := parseUserGroupID(ctx)
	if !ok {
		return
	}
	tariffID, ok := parseTariffID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteUserGroupTariff(ctx.Request.Context(), principal, userGroupID, tariffID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// @Summary Update charger customer visibility
// @Description Update the customer visibility of a specific charger.
// @Tags CPO Network
// @Accept json
// @Produce json
// @Param charger_id path string true "Charger ID"
// @Param visibility body UpdateChargerCustomerVisibilityRequest true "Charger customer visibility update data"
// @Success 200 {object} ChargerResponse "Successfully updated charger customer visibility"
// @Failure 400 {object} auth.APIError "Invalid request body or parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Failure 404 {object} auth.APIError "Charger not found"
// @Failure 500 {object} auth.APIError "Internal server error"
// @Router /cpo/chargers/{charger_id}/customer-visibility [put]
func (handler *Handler) updateChargerCustomerVisibility(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	chargerID, ok := parseChargerID(ctx)
	if !ok {
		return
	}

	var request UpdateChargerCustomerVisibilityRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		})
		return
	}

	record, err := handler.service.UpdateChargerCustomerVisibility(
		ctx.Request.Context(),
		principal,
		chargerID,
		request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, record)
}

// Add the new handler function after the existing listChargerTransactions function (around line 229)
// @Summary List wallet transactions
// @Description Returns a paginated list of wallet transactions for all customers of the authenticated CPO.
// @Tags CPO Network
// @Produce json
// @Param limit query int false "Number of records to return (1-200, default 50)"
// @Param before query string false "RFC3339 timestamp for keyset pagination"
// @Param before_id query string false "UUID tie‑breaker for pagination"
// @Param customer_id query string false "Filter by customer UUID"
// @Success 200 {object} WalletTransactionListResponse "Successfully retrieved wallet transactions"
// @Failure 400 {object} auth.APIError "Invalid query parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Security BearerAuth
// @Security CPOAppID
// @Router /cpo/wallet-transactions [get]
// listWalletTransactions – returns wallet transactions for all customers,
// optionally filtered by customer_id query parameter.
// @Summary List wallet transactions
// @Description Returns a paginated list of wallet transactions for all customers of the authenticated CPO.
// @Tags CPO Operations - Charging Sessions
// @Produce json
// @Param limit query int false "Number of records to return (1-200, default 50)"
// @Param before query string false "RFC3339 timestamp for keyset pagination"
// @Param before_id query string false "UUID tie‑breaker for pagination"
// @Param customer_id query string false "Filter by customer UUID"
// @Success 200 {object} WalletTransactionListResponse "Successfully retrieved wallet transactions"
// @Failure 400 {object} auth.APIError "Invalid parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Security BearerAuth
// @Security CPOAppID
// @Router /cpo/wallet-transactions [get]
func (handler *Handler) listWalletTransactions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseWalletTransactionListQuery(ctx)
	if !ok {
		return
	}
	records, err := handler.service.ListWalletTransactions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// listCustomerWalletTransactions – returns wallet transactions for a single,
// specific customer (identified by the path parameter).
// @Summary List wallet transactions for a specific customer
// @Description Returns a paginated list of wallet transactions for a specific customer of the authenticated CPO.
// @Tags CPO Operations - Charging Sessions
// @Produce json
// @Param customer_id path string true "Customer ID"
// @Param limit query int false "Number of records to return (1-200, default 50)"
// @Param before query string false "RFC3339 timestamp for keyset pagination"
// @Param before_id query string false "UUID tie‑breaker for pagination"
// @Success 200 {object} WalletTransactionListResponse "Successfully retrieved wallet transactions"
// @Failure 400 {object} auth.APIError "Invalid parameters"
// @Failure 401 {object} auth.APIError "Unauthorized"
// @Failure 403 {object} auth.APIError "Forbidden"
// @Security BearerAuth
// @Security CPOAppID
// @Router /cpo/customers/{customer_id}/wallet-transactions [get]
func (handler *Handler) listCustomerWalletTransactions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	customerID, ok := parseCustomerID(ctx)
	if !ok {
		return
	}
	query, ok := parseWalletTransactionListQuery(ctx)
	if !ok {
		return
	}
	// Force the customer filter to the one from the path
	query.CustomerID = &customerID
	records, err := handler.service.ListWalletTransactions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, records)
}

// Add the query parser function after the handler (around line 250)
func parseWalletTransactionListQuery(ctx *gin.Context) (WalletTransactionListQuery, bool) {
	query := WalletTransactionListQuery{}
	if limitText := strings.TrimSpace(ctx.Query("limit")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return WalletTransactionListQuery{}, false
		}
		query.Limit = limit
	}
	if beforeText := strings.TrimSpace(ctx.Query("before")); beforeText != "" {
		before, err := time.Parse(time.RFC3339, beforeText)
		if err != nil {
			writeError(ctx, invalid("before", "Before must be an RFC3339 timestamp."))
			return WalletTransactionListQuery{}, false
		}
		query.Before = &before
	}
	if beforeIDText := strings.TrimSpace(ctx.Query("before_id")); beforeIDText != "" {
		beforeID, err := uuid.Parse(beforeIDText)
		if err != nil || beforeID == uuid.Nil {
			writeError(ctx, invalid("before_id", "Before ID must be a non-zero UUID."))
			return WalletTransactionListQuery{}, false
		}
		query.BeforeID = &beforeID
	}
	if customerIDText := strings.TrimSpace(ctx.Query("customer_id")); customerIDText != "" {
		customerID, err := uuid.Parse(customerIDText)
		if err != nil || customerID == uuid.Nil {
			writeError(ctx, invalid("customer_id", "Customer ID must be a non-zero UUID."))
			return WalletTransactionListQuery{}, false
		}
		query.CustomerID = &customerID
	}
	return query, true
}

func (handler *Handler) getHubAnalytics(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	hubID, ok := parseHubID(ctx)
	if !ok {
		return
	}

	var query AnalyticsQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		writeError(ctx, invalid("query", "Invalid query parameters"))
		return
	}

	record, err := handler.service.GetHubAnalytics(ctx.Request.Context(), principal, hubID, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}
