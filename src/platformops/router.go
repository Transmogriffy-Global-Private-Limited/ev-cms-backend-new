package platformops

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service     *Service
	authService *auth.Service
}

func RegisterRoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{service: service, authService: authService}
	group.Use(noStore, authService.Authenticate(), auth.RequirePlatform())
	group.GET("/events", handler.events)
	group.GET("/realtime/stream", handler.stream)
	group.GET("/audit-logs", handler.auditLogs)
	group.GET("/workers", handler.workers)
}

func (handler *Handler) events(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListEvents(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) auditLogs(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseAuditQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListAudit(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) workers(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.ListWorkers(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) stream(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	query, ok := parseEventQuery(ctx)
	if !ok {
		return
	}
	token, ok := auth.CurrentAccessToken(ctx)
	if !ok {
		writeError(ctx, &auth.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "authentication_required",
			Message: "Authentication is required.",
		})
		return
	}
	page, err := handler.service.ListEvents(ctx.Request.Context(), principal, query)
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
	if err := writeEvents(ctx, page.Events); err != nil {
		return
	}
	ctx.Writer.Flush()

	poll := time.NewTicker(handler.service.config.RealtimePoll)
	heartbeat := time.NewTicker(handler.service.config.RealtimeHeartbeat)
	defer poll.Stop()
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-poll.C:
			page, err := handler.service.ListEvents(
				ctx.Request.Context(),
				principal,
				EventQuery{
					AfterID: cursor,
					Limit:   handler.service.config.RealtimeBatchSize,
					Type:    query.Type,
				},
			)
			if err != nil {
				return
			}
			if len(page.Events) == 0 {
				continue
			}
			if err := writeEvents(ctx, page.Events); err != nil {
				return
			}
			cursor = page.NextCursor
			ctx.Writer.Flush()
		case <-heartbeat.C:
			refreshed, err := handler.authService.ValidateAccess(
				ctx.Request.Context(),
				token,
			)
			if err != nil || refreshed.Scope != "PLATFORM" {
				return
			}
			if _, err := fmt.Fprintf(
				ctx.Writer,
				": heartbeat %s\n\n",
				time.Now().UTC().Format(time.RFC3339),
			); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func writeEvents(ctx *gin.Context, events []models.PlatformEvent) error {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			ctx.Writer,
			"id: %d\nevent: %s\ndata: %s\n\n",
			event.ID,
			event.EventType,
			payload,
		); err != nil {
			return err
		}
	}
	return nil
}

func parseEventQuery(ctx *gin.Context) (EventQuery, bool) {
	afterText := strings.TrimSpace(ctx.Query("after_id"))
	if afterText == "" {
		afterText = strings.TrimSpace(ctx.GetHeader("Last-Event-ID"))
	}
	afterID, ok := parseNonNegativeInt64(ctx, "after_id", afterText)
	if !ok {
		return EventQuery{}, false
	}
	limit, ok := parseLimit(ctx)
	if !ok {
		return EventQuery{}, false
	}
	return EventQuery{
		AfterID: afterID,
		Limit:   limit,
		Type:    strings.TrimSpace(ctx.Query("type")),
	}, true
}

func parseAuditQuery(ctx *gin.Context) (AuditQuery, bool) {
	limit, ok := parseLimit(ctx)
	if !ok {
		return AuditQuery{}, false
	}
	before, ok := parseOptionalTime(ctx, "before")
	if !ok {
		return AuditQuery{}, false
	}
	beforeID, ok := parseOptionalUUID(ctx, "before_id")
	if !ok {
		return AuditQuery{}, false
	}
	actor, ok := parseOptionalUUID(ctx, "actor_user_id")
	if !ok {
		return AuditQuery{}, false
	}
	cpoID, ok := parseOptionalUUID(ctx, "cpo_id")
	if !ok {
		return AuditQuery{}, false
	}
	return AuditQuery{
		Before:      before,
		BeforeID:    beforeID,
		Limit:       limit,
		Action:      strings.TrimSpace(ctx.Query("action")),
		Entity:      strings.TrimSpace(ctx.Query("entity")),
		ActorUserID: actor,
		CPOID:       cpoID,
	}, true
}

func parseLimit(ctx *gin.Context) (int, bool) {
	text := strings.TrimSpace(ctx.Query("limit"))
	if text == "" {
		return defaultPageSize, true
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || value > maxPageSize {
		writeError(ctx, invalid("limit", "limit must be between 1 and 500."))
		return 0, false
	}
	return value, true
}

func parseNonNegativeInt64(ctx *gin.Context, field, text string) (int64, bool) {
	if text == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		writeError(ctx, invalid(field, field+" must be zero or a positive integer."))
		return 0, false
	}
	return value, true
}

func parseOptionalTime(ctx *gin.Context, field string) (*time.Time, bool) {
	text := strings.TrimSpace(ctx.Query(field))
	if text == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		writeError(ctx, invalid(field, field+" must be an RFC3339 timestamp."))
		return nil, false
	}
	return &value, true
}

func parseOptionalUUID(ctx *gin.Context, field string) (*uuid.UUID, bool) {
	text := strings.TrimSpace(ctx.Query(field))
	if text == "" {
		return nil, true
	}
	value, err := uuid.Parse(text)
	if err != nil || value == uuid.Nil {
		writeError(ctx, invalid(field, field+" must be a non-zero UUID."))
		return nil, false
	}
	return &value, true
}

func writeError(ctx *gin.Context, err error) {
	var apiError *auth.APIError
	if errors.As(err, &apiError) {
		cmsmiddleware.LogHandledError(
			ctx, "platform_operations", apiError.Code, apiError.Status, err,
		)
		ctx.JSON(apiError.Status, gin.H{
			"error": gin.H{"code": apiError.Code, "message": apiError.Message},
		})
		return
	}
	cmsmiddleware.LogHandledError(
		ctx, "platform_operations", "internal_error", http.StatusInternalServerError, err,
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
