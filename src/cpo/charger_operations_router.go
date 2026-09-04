package cpo

import (
	"net/http"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type resetChargerRequest struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}
type connectorOperationRequest struct {
	ConnectorID string `json:"connector_id"`
}
type availabilityOperationRequest struct {
	ConnectorID string `json:"connector_id,omitempty"`
	Type        string `json:"type"`
}
type configurationOperationRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type triggerMessageOperationRequest struct {
	ConnectorID      string `json:"connector_id,omitempty"`
	RequestedMessage string `json:"requested_message"`
}

func (handler *Handler) requestChargerOperation(ctx *gin.Context, kind string, connectorID *uuid.UUID, parameters map[string]string) {
	chargerID, err := uuid.Parse(ctx.Param("charger_id"))
	if err != nil || chargerID == uuid.Nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_charger_id", Message: "A canonical charger UUID is required."})
		return
	}
	key := strings.TrimSpace(ctx.GetHeader("Idempotency-Key"))
	correlation, ok := cmsmiddleware.RequestID(ctx)
	if !ok {
		writeError(ctx, &auth.APIError{Status: http.StatusInternalServerError, Code: "missing_request_correlation", Message: "The operation could not be correlated."})
		return
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.RequestChargerOperation(ctx.Request.Context(), principal, chargerID, ChargerOperationInput{Kind: kind, ConnectorID: connectorID, Parameters: parameters, IdempotencyKey: key, CorrelationID: correlation})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func parseOperationConnector(ctx *gin.Context, raw string, required bool) (*uuid.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" && !required {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_connector_id", Message: "A canonical connector UUID is required."})
		return nil, false
	}
	return &id, true
}
func (handler *Handler) resetCharger(ctx *gin.Context) {
	var request resetChargerRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return
	}
	kind := strings.ToUpper(strings.TrimSpace(request.Type))
	if (kind != "SOFT" && kind != "HARD") || len(strings.TrimSpace(request.Reason)) < 3 || len(request.Reason) > 500 {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_parameters", Message: "Reset type and reason are required."})
		return
	}
	handler.requestChargerOperation(ctx, "RESET", nil, map[string]string{"type": kind, "reason": strings.TrimSpace(request.Reason)})
}
func (handler *Handler) unlockConnector(ctx *gin.Context) {
	var request connectorOperationRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return
	}
	connector, ok := parseOperationConnector(ctx, request.ConnectorID, true)
	if ok {
		handler.requestChargerOperation(ctx, "UNLOCK_CONNECTOR", connector, map[string]string{})
	}
}
func (handler *Handler) changeAvailability(ctx *gin.Context) {
	var request availabilityOperationRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return
	}
	connector, ok := parseOperationConnector(ctx, request.ConnectorID, false)
	if !ok {
		return
	}
	availability := strings.ToUpper(strings.TrimSpace(request.Type))
	if availability != "OPERATIVE" && availability != "INOPERATIVE" {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_parameters", Message: "Availability type is invalid."})
		return
	}
	handler.requestChargerOperation(ctx, "CHANGE_AVAILABILITY", connector, map[string]string{"type": availability})
}
func (handler *Handler) clearCache(ctx *gin.Context) {
	handler.requestChargerOperation(ctx, "CLEAR_CACHE", nil, map[string]string{})
}
func (handler *Handler) changeConfiguration(ctx *gin.Context) {
	var request configurationOperationRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return
	}
	key := strings.TrimSpace(request.Key)
	if len(key) < 1 || len(key) > 100 || len(request.Value) > 500 {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_parameters", Message: "Configuration key or value is invalid."})
		return
	}
	lowerKey := strings.ToLower(key)
	if lowerKey == "heartbeatinterval" || lowerKey == "metervalueinterval" || lowerKey == "metervaluesampleinterval" || lowerKey == "authorizeremotetxrequests" || lowerKey == "localauthorizeoffline" || lowerKey == "localpreauthorize" || lowerKey == "authorizationcacheenabled" || lowerKey == "allowofflinetxforunknownid" || lowerKey == "stoptransactiononinvalidid" || lowerKey == "chargepointauthenable" || lowerKey == "freevendenabled" {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "reserved_configuration_key", Message: "The configuration key is HAL-owned and cannot be changed here."})
		return
	}
	if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "privatekey") || strings.Contains(lowerKey, "certificate") || lowerKey == "authorizationkey" {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "sensitive_configuration_key", Message: "The configuration key requires a separate secure workflow."})
		return
	}
	handler.requestChargerOperation(ctx, "CHANGE_CONFIGURATION", nil, map[string]string{"key": key, "value": request.Value})
}
func (handler *Handler) triggerMessage(ctx *gin.Context) {
	var request triggerMessageOperationRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return
	}
	connector, ok := parseOperationConnector(ctx, request.ConnectorID, false)
	if !ok {
		return
	}
	allowed := map[string]bool{"BootNotification": true, "DiagnosticsStatusNotification": true, "FirmwareStatusNotification": true, "Heartbeat": true, "MeterValues": true, "StatusNotification": true}
	if !allowed[request.RequestedMessage] {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "unsupported_operation", Message: "The requested OCPP message is not supported."})
		return
	}
	handler.requestChargerOperation(ctx, "TRIGGER_MESSAGE", connector, map[string]string{"requested_message": request.RequestedMessage})
}
func (handler *Handler) getChargerOperation(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("operation_id"))
	if err != nil || id == uuid.Nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_id", Message: "A canonical operation UUID is required."})
		return
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.GetChargerOperation(ctx.Request.Context(), principal, id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getChargerConfiguration(ctx *gin.Context) {
	chargerID, err := uuid.Parse(ctx.Param("charger_id"))
	if err != nil || chargerID == uuid.Nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_charger_id", Message: "A canonical charger UUID is required."})
		return
	}
	keys := ctx.QueryArray("key")
	if len(keys) > 64 {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_parameters", Message: "Too many configuration keys were requested."})
		return
	}
	for _, key := range keys {
		if len(strings.TrimSpace(key)) < 1 || len(key) > 100 {
			writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_operation_parameters", Message: "A configuration key is invalid."})
			return
		}
	}
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.GetChargerConfiguration(ctx.Request.Context(), principal, chargerID, keys)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
