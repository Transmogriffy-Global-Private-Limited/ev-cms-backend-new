package cpo

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func parseChargerOperationHistoryQuery(ctx *gin.Context) (ChargerOperationHistoryQuery, bool) {
	query := ChargerOperationHistoryQuery{}
	if text := strings.TrimSpace(ctx.Query("limit")); text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil {
			writeError(ctx, invalid("limit", "Limit must be an integer."))
			return ChargerOperationHistoryQuery{}, false
		}
		query.Limit = limit
	}
	var ok bool
	if query.Before, ok = parseHistoryTimestamp(ctx, "before"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.CreatedAfter, ok = parseHistoryTimestamp(ctx, "created_after"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.CreatedBefore, ok = parseHistoryTimestamp(ctx, "created_before"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.BeforeID, ok = parseHistoryUUID(ctx, "before_id"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.ChargerID, ok = parseHistoryUUID(ctx, "charger_id"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.ConnectorID, ok = parseHistoryUUID(ctx, "connector_id"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	if query.ActorUserID, ok = parseHistoryUUID(ctx, "actor_user_id"); !ok {
		return ChargerOperationHistoryQuery{}, false
	}
	query.Kind = historyQueryText(ctx, "kind")
	query.State = historyQueryText(ctx, "state")
	query.OCPPResult = historyQueryText(ctx, "ocpp_result")
	query.FailureCategory = historyQueryText(ctx, "failure_category")
	query.ResetType = historyQueryText(ctx, "reset_type")
	query.AvailabilityType = historyQueryText(ctx, "availability_type")
	query.RequestedMessage = historyQueryText(ctx, "requested_message")
	query.ConfigurationKey = historyQueryText(ctx, "configuration_key")
	return query, true
}

func parseHistoryTimestamp(ctx *gin.Context, field string) (*time.Time, bool) {
	text := strings.TrimSpace(ctx.Query(field))
	if text == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		writeError(ctx, invalid(field, field+" must be an RFC3339 timestamp."))
		return nil, false
	}
	return &parsed, true
}

func parseHistoryUUID(ctx *gin.Context, field string) (*uuid.UUID, bool) {
	raw := ctx.Query(field)
	text := strings.TrimSpace(raw)
	if text == "" {
		if raw != "" {
			writeError(ctx, invalid(field, field+" must be a canonical non-zero UUID."))
			return nil, false
		}
		return nil, true
	}
	parsed, err := uuid.Parse(text)
	if raw != text || err != nil || parsed == uuid.Nil || parsed.String() != text {
		writeError(ctx, invalid(field, field+" must be a canonical non-zero UUID."))
		return nil, false
	}
	return &parsed, true
}

func historyQueryText(ctx *gin.Context, field string) *string {
	value := strings.TrimSpace(ctx.Query(field))
	if value == "" {
		return nil
	}
	return &value
}

func (handler *Handler) listChargerOperationHistory(ctx *gin.Context) {
	query, ok := parseChargerOperationHistoryQuery(ctx)
	if !ok {
		return
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.ListChargerOperationHistory(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
