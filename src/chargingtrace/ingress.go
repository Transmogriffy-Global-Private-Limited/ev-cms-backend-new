// Package chargingtrace owns the isolated HAL diagnostic ingestion boundary.
// It deliberately does not project, repair, or decide charging business state.
package chargingtrace

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Envelope struct {
	SchemaVersion          int          `json:"schema_version"`
	TraceID                uuid.UUID    `json:"trace_id"`
	EventID                uuid.UUID    `json:"event_id"`
	CPOID                  uuid.UUID    `json:"cpo_id"`
	CMSStartIntentID       *uuid.UUID   `json:"cms_start_intent_id"`
	CMSChargingSessionID   *uuid.UUID   `json:"cms_charging_session_id"`
	CMSCommandID           *uuid.UUID   `json:"cms_command_id"`
	CMSChargerOperationID  *uuid.UUID   `json:"cms_charger_operation_id"`
	HALChargerOperationID  *uuid.UUID   `json:"hal_charger_operation_id"`
	HALTransactionID       *uuid.UUID   `json:"hal_transaction_id"`
	OCPPTransactionID      *int64       `json:"ocpp_transaction_id"`
	ChargerOCPPIdentity    string       `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int          `json:"ocpp_connector_number"`
	Source                 string       `json:"source"`
	Target                 string       `json:"target"`
	Category               string       `json:"category"`
	Protocol               string       `json:"protocol"`
	Phase                  string       `json:"phase"`
	Summary                string       `json:"summary"`
	OccurredAt             time.Time    `json:"occurred_at"`
	StateBefore            string       `json:"state_before"`
	StateAfter             string       `json:"state_after"`
	CorrelationID          string       `json:"correlation_id"`
	Data                   models.JSONB `json:"data"`
	ImmutableContentSHA256 string       `json:"immutable_content_sha256"`
}

type Error struct {
	Status        int
	Code, Message string
}

func (err *Error) Error() string { return err.Code }

type Ingestor struct {
	database *gorm.DB
	bearer   string
	now      func() time.Time
}

func NewIngestor(database *gorm.DB, bearer string) *Ingestor {
	return &Ingestor{database: database, bearer: strings.TrimSpace(bearer), now: func() time.Time { return time.Now().UTC() }}
}

func (ingestor *Ingestor) Accept(ctx context.Context, bearer string, envelope Envelope) error {
	if ingestor == nil || ingestor.bearer == "" || len(bearer) != len(ingestor.bearer) || subtle.ConstantTimeCompare([]byte(bearer), []byte(ingestor.bearer)) != 1 {
		return &Error{Status: 401, Code: "hal_trace_authentication_required", Message: "Trace service authentication is required."}
	}
	if err := validateEnvelope(envelope); err != nil {
		return err
	}
	if ingestor.database == nil {
		return &Error{Status: 500, Code: "hal_trace_storage_unavailable", Message: "The trace event could not be processed."}
	}
	digest, err := Digest(envelope)
	if err != nil {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(envelope.ImmutableContentSHA256)) != 1 {
		return &Error{Status: 409, Code: "hal_trace_integrity_conflict", Message: "The trace event immutable content conflicts."}
	}
	return ingestor.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.ChargingTraceEvent
		err := tx.First(&existing, "id = ?", envelope.EventID).Error
		if err == nil {
			if subtle.ConstantTimeCompare([]byte(existing.ImmutableContentSHA256), []byte(digest)) == 1 {
				return nil
			}
			return &Error{Status: 409, Code: "hal_trace_event_conflict", Message: "The trace event identity conflicts."}
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		root, err := ingestor.adoptRoot(tx, envelope)
		if err != nil {
			return err
		}
		if err := tx.Create(&models.ChargingTraceEvent{ID: envelope.EventID, TraceID: root.TraceID, CPOID: root.CPOID, SessionID: root.CMSChargingSessionID, Source: envelope.Source, Target: envelope.Target, Category: envelope.Category, Protocol: envelope.Protocol, Phase: envelope.Phase, Summary: envelope.Summary, OccurredAt: envelope.OccurredAt.UTC(), RecordedAt: ingestor.now(), StateBefore: envelope.StateBefore, StateAfter: envelope.StateAfter, CorrelationID: envelope.CorrelationID, Data: sanitize(envelope.Data), ImmutableContentSHA256: digest}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (ingestor *Ingestor) adoptRoot(tx *gorm.DB, envelope Envelope) (models.ChargingTrace, error) {
	var root models.ChargingTrace
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&root, "trace_id = ?", envelope.TraceID).Error
	if err == gorm.ErrRecordNotFound {
		root = models.ChargingTrace{TraceID: envelope.TraceID, CPOID: envelope.CPOID, CMSStartIntentID: envelope.CMSStartIntentID, CMSChargingSessionID: envelope.CMSChargingSessionID, CMSCommandID: envelope.CMSCommandID, CMSChargerOperationID: envelope.CMSChargerOperationID, HALChargerOperationID: envelope.HALChargerOperationID, HALTransactionID: envelope.HALTransactionID, OCPPTransactionID: envelope.OCPPTransactionID, ChargerOCPPIdentity: strings.TrimSpace(envelope.ChargerOCPPIdentity), OCPPConnectorNumber: envelope.OCPPConnectorNumber, CreatedAt: ingestor.now(), UpdatedAt: ingestor.now()}
		if err := tx.Create(&root).Error; err != nil {
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) && pgError.Code == "23505" {
				return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
			}
			return models.ChargingTrace{}, err
		}
		return root, nil
	}
	if err != nil {
		return models.ChargingTrace{}, err
	}
	if root.CPOID != envelope.CPOID {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	updates := map[string]any{"updated_at": ingestor.now()}
	if err := monotonicUUID(root.CMSStartIntentID, envelope.CMSStartIntentID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.CMSChargingSessionID, envelope.CMSChargingSessionID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.CMSCommandID, envelope.CMSCommandID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.CMSChargerOperationID, envelope.CMSChargerOperationID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.HALChargerOperationID, envelope.HALChargerOperationID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.HALTransactionID, envelope.HALTransactionID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicInt64(root.OCPPTransactionID, envelope.OCPPTransactionID); err != nil {
		return models.ChargingTrace{}, err
	}
	for _, field := range []struct {
		current, incoming any
		column            string
	}{{root.CMSStartIntentID, envelope.CMSStartIntentID, "cms_start_intent_id"}, {root.CMSChargingSessionID, envelope.CMSChargingSessionID, "cms_charging_session_id"}, {root.CMSCommandID, envelope.CMSCommandID, "cms_command_id"}, {root.CMSChargerOperationID, envelope.CMSChargerOperationID, "cms_charger_operation_id"}, {root.HALChargerOperationID, envelope.HALChargerOperationID, "hal_charger_operation_id"}, {root.HALTransactionID, envelope.HALTransactionID, "hal_transaction_id"}, {root.OCPPTransactionID, envelope.OCPPTransactionID, "ocpp_transaction_id"}} {
		if field.current == nil && field.incoming != nil {
			updates[field.column] = field.incoming
		}
	}
	if root.ChargerOCPPIdentity == "" {
		updates["charger_ocpp_identity"] = strings.TrimSpace(envelope.ChargerOCPPIdentity)
	} else if root.ChargerOCPPIdentity != envelope.ChargerOCPPIdentity {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	if root.OCPPConnectorNumber == 0 {
		updates["ocpp_connector_number"] = envelope.OCPPConnectorNumber
	} else if root.OCPPConnectorNumber != envelope.OCPPConnectorNumber {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	if err := tx.Model(&root).Updates(updates).Error; err != nil {
		return models.ChargingTrace{}, err
	}
	if err := tx.First(&root, "trace_id = ?", envelope.TraceID).Error; err != nil {
		return models.ChargingTrace{}, err
	}
	return root, nil
}

func monotonicUUID(current, incoming *uuid.UUID) error {
	if current == nil || incoming == nil {
		return nil
	}
	if *current != *incoming {
		return &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	return nil
}
func monotonicInt64(current, incoming *int64) error {
	if current == nil || incoming == nil {
		return nil
	}
	if *current != *incoming {
		return &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	return nil
}
func validateEnvelope(e Envelope) error {
	if e.SchemaVersion != 1 || e.TraceID == uuid.Nil || e.EventID == uuid.Nil || e.CPOID == uuid.Nil || e.OCPPConnectorNumber < 0 || e.OccurredAt.IsZero() || !validText(e.ChargerOCPPIdentity, 255) || !validText(e.Source, 32) || !validText(e.Target, 32) || !validText(e.Category, 48) || !validText(e.Protocol, 24) || !validText(e.Phase, 24) || !validText(e.Summary, 200) || len(e.StateBefore) > 64 || len(e.StateAfter) > 64 || len(e.CorrelationID) > 128 || len(e.ImmutableContentSHA256) != 64 || e.Data == nil {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if e.OCPPTransactionID != nil && *e.OCPPTransactionID <= 0 {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if !validActor(e.Source) || !validActor(e.Target) || !validPhase(e.Phase) {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if e.Category == "CHARGER_OPERATION_OCPP" && (e.CMSChargerOperationID == nil || e.HALChargerOperationID == nil || !validOperationEvidence(e.Data)) {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if e.Category == "CHARGER_OPERATION_FOLLOW_ON" && (e.CMSChargerOperationID == nil || e.HALChargerOperationID == nil || !validTriggerMessageFollowOnEvidence(e.Data)) {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if len(e.Data) > 16 {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	return nil
}
func validText(v string, n int) bool { return strings.TrimSpace(v) != "" && len(v) <= n }
func validActor(v string) bool       { return v == "APP" || v == "CMS" || v == "HAL" || v == "CHARGER" }
func validPhase(v string) bool {
	return v == "STARTING" || v == "CHARGING" || v == "STOPPING" || v == "POST_STOP"
}
func sanitize(input models.JSONB) models.JSONB {
	if input["expected_message"] != nil {
		return sanitizeTriggerMessageFollowOnEvidence(input)
	}
	if input["message_type"] != nil {
		return sanitizeOperationEvidence(input)
	}
	output := models.JSONB{}
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}

func validTriggerMessageFollowOnEvidence(input models.JSONB) bool {
	_, ok := sanitizeTriggerMessageFollowOnEvidence(input)["expected_message"]
	return ok
}

func sanitizeTriggerMessageFollowOnEvidence(input models.JSONB) models.JSONB {
	expected, expectedOK := input["expected_message"].(string)
	observed, observedOK := input["observed_action"].(string)
	identity, identityOK := input["charger_ocpp_identity"].(string)
	if !expectedOK || !observedOK || !identityOK || expected != observed || !triggerMessageFollowOnAction(expected) || !validText(identity, 255) {
		return models.JSONB{}
	}
	safe := models.JSONB{"expected_message": expected, "observed_action": observed, "charger_ocpp_identity": identity}
	if triggerMessageFollowOnConnectorScoped(expected) {
		connector, connectorOK := safeConnectorNumber(input["connector_number"])
		if !connectorOK || connector < 1 || len(input) != 4 {
			return models.JSONB{}
		}
		safe["connector_number"] = connector
		return safe
	}
	if len(input) != 3 {
		return models.JSONB{}
	}
	return safe
}

func triggerMessageFollowOnAction(action string) bool {
	switch action {
	case "BootNotification", "DiagnosticsStatusNotification", "FirmwareStatusNotification", "Heartbeat", "MeterValues", "StatusNotification":
		return true
	default:
		return false
	}
}

func triggerMessageFollowOnConnectorScoped(action string) bool {
	return action == "MeterValues" || action == "StatusNotification"
}

func safeConnectorNumber(value any) (int, bool) {
	switch connector := value.(type) {
	case float64:
		return int(connector), connector == float64(int(connector)) && connector >= 0 && connector <= 999
	case int:
		return connector, connector >= 0 && connector <= 999
	default:
		return 0, false
	}
}

func validOperationEvidence(input models.JSONB) bool {
	messageType, _ := input["message_type"].(string)
	action, _ := input["action"].(string)
	uniqueID, _ := input["unique_id"].(string)
	payload, ok := input["payload"].(map[string]any)
	if (messageType != "CALL" && messageType != "CALLRESULT" && messageType != "CALLERROR") || !validText(uniqueID, 128) || !supportedOperationAction(action) || !ok || len(payload) > 8 {
		return false
	}
	_, valid := safeOperationPayload(action, messageType, payload)
	return valid
}
func supportedOperationAction(action string) bool {
	for _, value := range []string{"Reset", "UnlockConnector", "ChangeAvailability", "ClearCache", "ChangeConfiguration", "TriggerMessage", "GetConfiguration"} {
		if action == value {
			return true
		}
	}
	return false
}
func sanitizeOperationEvidence(input models.JSONB) models.JSONB {
	uniqueID, _ := input["unique_id"].(string)
	action, _ := input["action"].(string)
	messageType, _ := input["message_type"].(string)
	payload, _ := input["payload"].(map[string]any)
	safe, ok := safeOperationPayload(action, messageType, payload)
	if !ok {
		return models.JSONB{}
	}
	return models.JSONB{"unique_id": uniqueID, "action": action, "message_type": messageType, "payload": safe}
}

func safeOperationPayload(action, messageType string, payload map[string]any) (map[string]any, bool) {
	text := func(key string, maximum int) (string, bool) {
		value, ok := payload[key].(string)
		return value, ok && validText(value, maximum)
	}
	status := func() (map[string]any, bool) {
		value, ok := text("status", 64)
		if !ok {
			return nil, false
		}
		return map[string]any{"status": value}, true
	}
	if messageType == "CALLERROR" {
		code, ok := text("error_code", 64)
		return map[string]any{"error_code": code}, ok && len(payload) == 1
	}
	if messageType == "CALLRESULT" {
		if action != "GetConfiguration" {
			return status()
		}
		return safeConfigurationResult(payload)
	}
	switch action {
	case "Reset":
		value, ok := text("type", 16)
		return map[string]any{"type": value}, ok && (value == "Soft" || value == "Hard") && len(payload) == 1
	case "UnlockConnector", "ChangeAvailability":
		connector, ok := payload["connectorId"].(float64)
		if !ok || connector < 0 || connector > 999 || connector != float64(int(connector)) {
			return nil, false
		}
		if action == "UnlockConnector" {
			return map[string]any{"connectorId": int(connector)}, len(payload) == 1
		}
		value, valid := text("type", 16)
		return map[string]any{"connectorId": int(connector), "type": value}, valid && (value == "Operative" || value == "Inoperative") && len(payload) == 2
	case "ClearCache":
		return map[string]any{}, len(payload) == 0
	case "ChangeConfiguration":
		key, ok := text("key", 100)
		redacted, redactedOK := payload["redacted"].(bool)
		return map[string]any{"key": key, "redacted": true}, ok && redactedOK && redacted && len(payload) == 2
	case "TriggerMessage":
		value, ok := text("requestedMessage", 64)
		safe := map[string]any{"requestedMessage": value}
		if connector, exists := payload["connectorId"]; exists {
			number, valid := connector.(float64)
			if !valid || number < 0 || number > 999 || number != float64(int(number)) {
				return nil, false
			}
			safe["connectorId"] = int(number)
		}
		return safe, ok && (len(payload) == 1 || len(payload) == 2)
	case "GetConfiguration":
		keys, ok := safeStringList(payload["configuration_keys"], 64, 100)
		return map[string]any{"configuration_keys": keys}, ok && len(payload) == 1
	}
	return nil, false
}

func safeConfigurationResult(payload map[string]any) (map[string]any, bool) {
	keys, ok := safeConfigurationKeys(payload["configuration_keys"])
	if !ok {
		return nil, false
	}
	unknown, ok := safeStringList(payload["unknown_keys"], 64, 100)
	if !ok || len(payload) != 2 {
		return nil, false
	}
	return map[string]any{"configuration_keys": keys, "unknown_keys": unknown}, true
}

func safeConfigurationKeys(value any) ([]map[string]any, bool) {
	items, ok := value.([]any)
	if !ok || len(items) > 64 {
		return nil, false
	}
	output := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok || len(entry) < 2 || len(entry) > 3 {
			return nil, false
		}
		key, ok := entry["key"].(string)
		redacted, redactedOK := entry["redacted"].(bool)
		if !ok || !validText(key, 100) || !redactedOK {
			return nil, false
		}
		safe := map[string]any{"key": key, "redacted": redacted}
		if value, exists := entry["value"]; exists && !redacted {
			stringValue, valid := value.(string)
			if !valid || len(stringValue) > 500 {
				return nil, false
			}
			safe["value"] = stringValue
		} else if exists {
			return nil, false
		}
		output = append(output, safe)
	}
	return output, true
}

func safeStringList(value any, maximum, itemMaximum int) ([]string, bool) {
	items, ok := value.([]any)
	if !ok || len(items) > maximum {
		return nil, false
	}
	output := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || !validText(text, itemMaximum) {
			return nil, false
		}
		output = append(output, text)
	}
	return output, true
}
func Digest(envelope Envelope) (string, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	var immutable map[string]any
	if err := json.Unmarshal(raw, &immutable); err != nil {
		return "", err
	}
	delete(immutable, "immutable_content_sha256")
	raw, err = json.Marshal(immutable)
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
