package superadmin

import (
	"encoding/json"
	"errors"
	"io"
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

type Handler struct{ service *Service }

func RegisterRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	handler := &Handler{service: service}
	group.Use(noStore, authService.Authenticate(), auth.RequirePlatform())
	group.GET("/administrators", handler.listAdministrators)
	group.POST("/administrators", handler.inviteAdministrator)
	group.POST("/administrators/:user_id/activate", handler.activateAdministrator)
	group.POST("/administrators/:user_id/deactivate", handler.deactivateAdministrator)
	group.GET("/security/locked-identities", handler.lockedIdentities)
	group.GET("/security/events", handler.securityEvents)
	group.POST("/security/users/:user_id/unlock", handler.unlockIdentity)
	group.POST("/security/users/:user_id/sessions/revoke", handler.revokeUserSessions)
	group.GET("/mail/jobs", handler.mailJobs)
	group.GET("/mail/jobs/:job_id", handler.mailJob)
	group.POST("/mail/jobs/:job_id/retry", handler.retryMailJob)
	group.POST("/mail/jobs/:job_id/cancel", handler.cancelMailJob)
	group.GET("/mail/metrics", handler.mailMetrics)
	group.POST("/mail/reconcile", handler.reconcileMailJobs)
	group.POST("/mail/retention", handler.retainMailJobs)
	group.GET("/announcements", handler.announcements)
	group.POST("/announcements", handler.createAnnouncement)
	group.GET("/notifications", handler.platformNotifications)
	group.POST("/notifications/:notification_id/read", handler.markNotificationRead)
	group.GET("/overview", handler.overview)
	group.GET("/status", handler.status)
	group.GET("/cpo-assets", handler.cpoAssets)
	group.GET("/cpos/:cpo_id/customer-intelligence", handler.customerIntelligence)
}

func RegisterCPONotificationRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	handler := &Handler{service: service}
	group.Use(noStore, authService.Authenticate(), auth.RequireCPOAppID())
	group.GET("/notifications", handler.cpoNotifications)
	group.POST("/notifications/:notification_id/read", handler.markNotificationRead)
}

func (handler *Handler) cpoAssets(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.CPOAssets(ctx, principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) listAdministrators(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseAdministratorQuery(ctx)
	if !ok {
		return
	}
	response, err := handler.service.ListAdministrators(ctx, principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) inviteAdministrator(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request InviteAdministratorRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.InviteOrGrant(ctx, principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}

func (handler *Handler) activateAdministrator(ctx *gin.Context) {
	handler.setAdministratorStatus(ctx, true)
}
func (handler *Handler) deactivateAdministrator(ctx *gin.Context) {
	handler.setAdministratorStatus(ctx, false)
}
func (handler *Handler) setAdministratorStatus(ctx *gin.Context, active bool) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userID, ok := parsePathUUID(ctx, "user_id")
	if !ok {
		return
	}
	var request ReasonRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.SetAdministratorStatus(ctx, principal, userID, active, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) lockedIdentities(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseSecurityQuery(ctx)
	if !ok {
		return
	}
	response, err := handler.service.ListLockedIdentities(ctx, principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) securityEvents(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parsePageQuery(ctx)
	if !ok {
		return
	}
	response, err := handler.service.ListSecurityEvents(ctx, principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) unlockIdentity(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userID, ok := parsePathUUID(ctx, "user_id")
	if !ok {
		return
	}
	var request ReasonRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	if err := handler.service.UnlockIdentity(ctx, principal, userID, request); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) revokeUserSessions(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	userID, ok := parsePathUUID(ctx, "user_id")
	if !ok {
		return
	}
	var request SessionRevocationRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.RevokeUserSessions(ctx, principal, userID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) mailJobs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseMailQuery(ctx)
	if !ok {
		return
	}
	response, err := handler.service.ListMailJobs(ctx, principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) mailJob(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	jobID, ok := parsePathUUID(ctx, "job_id")
	if !ok {
		return
	}
	response, err := handler.service.GetMailJob(ctx, principal, jobID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) retryMailJob(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	jobID, ok := parsePathUUID(ctx, "job_id")
	if !ok {
		return
	}
	response, err := handler.service.RetryMailJob(ctx, principal, jobID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) cancelMailJob(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	jobID, ok := parsePathUUID(ctx, "job_id")
	if !ok {
		return
	}
	var request ReasonRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.CancelMailJob(ctx, principal, jobID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) mailMetrics(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.MailMetrics(ctx, principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) reconcileMailJobs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request ReasonRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.ReconcileMailJobs(ctx, principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) retainMailJobs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request MailRetentionRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.RetainMailJobs(ctx, principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) announcements(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parsePageQuery(ctx)
	if !ok {
		return
	}
	response, err := handler.service.ListAnnouncements(ctx, principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) createAnnouncement(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request AnnouncementRequest
	if !decodeJSON(ctx, &request) {
		return
	}
	response, err := handler.service.CreateAnnouncement(ctx, principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}
func (handler *Handler) platformNotifications(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parsePageQuery(ctx)
	if !ok {
		return
	}
	unread, err := parseBoolQuery(ctx, "unread_only")
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListNotifications(ctx, principal, query, unread)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) cpoNotifications(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parsePageQuery(ctx)
	if !ok {
		return
	}
	unread, err := parseBoolQuery(ctx, "unread_only")
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListNotifications(ctx, principal, query, unread)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) markNotificationRead(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	notificationID, ok := parsePathUUID(ctx, "notification_id")
	if !ok {
		return
	}
	if err := handler.service.MarkNotificationRead(ctx, principal, notificationID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
func (handler *Handler) overview(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.Overview(ctx, principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (handler *Handler) status(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.Status(ctx, principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func parseAdministratorQuery(ctx *gin.Context) (AdministratorQuery, bool) {
	page, ok := parsePageQuery(ctx)
	if !ok {
		return AdministratorQuery{}, false
	}
	include, err := parseBoolQuery(ctx, "include_inactive")
	if err != nil {
		writeError(ctx, err)
		return AdministratorQuery{}, false
	}
	return AdministratorQuery{PageQuery: page, IncludeInactive: include}, true
}
func parseSecurityQuery(ctx *gin.Context) (SecurityQuery, bool) {
	page, ok := parsePageQuery(ctx)
	if !ok {
		return SecurityQuery{}, false
	}
	return SecurityQuery{PageQuery: page}, true
}
func parseMailQuery(ctx *gin.Context) (MailQuery, bool) {
	page, ok := parsePageQuery(ctx)
	if !ok {
		return MailQuery{}, false
	}
	query := MailQuery{PageQuery: page, Status: constants.MailOutboxStatus(strings.TrimSpace(ctx.Query("status"))), Template: strings.TrimSpace(ctx.Query("template"))}
	var err error
	query.CPOID, err = parseOptionalUUID(ctx.Query("cpo_id"))
	if err != nil {
		writeError(ctx, invalid("cpo_id", "cpo_id must be a valid UUID."))
		return MailQuery{}, false
	}
	query.UserID, err = parseOptionalUUID(ctx.Query("user_id"))
	if err != nil {
		writeError(ctx, invalid("user_id", "user_id must be a valid UUID."))
		return MailQuery{}, false
	}
	return query, true
}
func parsePageQuery(ctx *gin.Context) (PageQuery, bool) {
	limit := defaultPageSize
	if text := strings.TrimSpace(ctx.Query("limit")); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > maxPageSize {
			writeError(ctx, invalid("limit", "limit must be between 1 and 500."))
			return PageQuery{}, false
		}
		limit = value
	}
	before, err := parseOptionalTime(ctx.Query("before"))
	if err != nil {
		writeError(ctx, invalid("before", "before must be an RFC3339 timestamp."))
		return PageQuery{}, false
	}
	beforeID, err := parseOptionalUUID(ctx.Query("before_id"))
	if err != nil {
		writeError(ctx, invalid("before_id", "before_id must be a valid UUID."))
		return PageQuery{}, false
	}
	return PageQuery{Limit: limit, Before: before, BeforeID: beforeID}, true
}
func parseOptionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return &parsed, err
}
func parseOptionalUUID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil {
		return nil, errors.New("invalid UUID")
	}
	return &parsed, nil
}
func parseBoolQuery(ctx *gin.Context, field string) (bool, error) {
	value := strings.TrimSpace(ctx.Query(field))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, invalid(field, field+" must be true or false.")
	}
	return parsed, nil
}
func parsePathUUID(ctx *gin.Context, field string) (uuid.UUID, bool) {
	value, err := uuid.Parse(ctx.Param(field))
	if err != nil || value == uuid.Nil {
		writeError(ctx, invalid(field, field+" must be a valid UUID."))
		return uuid.Nil, false
	}
	return value, true
}

func decodeJSON(ctx *gin.Context, destination any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(ctx, &auth.APIError{Status: 400, Code: "invalid_request", Message: "The request body is invalid."})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(ctx, &auth.APIError{Status: 400, Code: "invalid_request", Message: "The request body must contain one JSON object."})
		return false
	}
	return true
}

func writeError(ctx *gin.Context, err error) {
	var apiErr *auth.APIError
	if !errors.As(err, &apiErr) {
		cmsmiddleware.LogHandledError(ctx, "superadmin", "internal_error", 500, err)
		ctx.JSON(500, gin.H{"error": gin.H{"code": "internal_error", "message": "The request could not be completed."}})
		return
	}
	cmsmiddleware.LogHandledError(ctx, "superadmin", apiErr.Code, apiErr.Status, err)
	ctx.JSON(apiErr.Status, gin.H{"error": gin.H{"code": apiErr.Code, "message": apiErr.Message}})
}
func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}

func (handler *Handler) customerIntelligence(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	cpoIDStr := ctx.Param("cpo_id") // changed from "cpoId"
	cpoID, err := uuid.Parse(cpoIDStr)
	if err != nil {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_cpo_id",
			Message: "CPO ID must be a valid UUID.",
		})
		return
	}

	resp, err := handler.service.CustomerIntelligence(ctx.Request.Context(), principal, cpoID)
	if err != nil {
		// If the error indicates CPO not found, return a 404; otherwise let writeError handle it (500)
		if strings.Contains(err.Error(), "cpo not found") {
			writeError(ctx, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "not_found",
				Message: "CPO not found.",
			})
		} else {
			writeError(ctx, err)
		}
		return
	}
	ctx.JSON(http.StatusOK, resp)
}
