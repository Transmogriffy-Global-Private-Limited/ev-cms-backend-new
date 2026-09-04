package cpo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

var chargerOperationKinds = map[string]struct{}{
	"RESET": {}, "UNLOCK_CONNECTOR": {}, "CHANGE_AVAILABILITY": {},
	"CLEAR_CACHE": {}, "CHANGE_CONFIGURATION": {}, "TRIGGER_MESSAGE": {},
}

var chargerOperationStates = map[string]struct{}{
	"PERSISTED": {}, "HAL_ACCEPTED": {}, "OCPP_CONFIRMED": {},
	"RECONCILIATION_REQUIRED": {}, "CONFIRMED_ABSENT": {},
}

var triggerMessageKinds = map[string]struct{}{
	"BootNotification": {}, "DiagnosticsStatusNotification": {},
	"FirmwareStatusNotification": {}, "Heartbeat": {}, "MeterValues": {},
	"StatusNotification": {},
}

type ChargerOperationHistoryQuery struct {
	Limit            int
	Before           *time.Time
	BeforeID         *uuid.UUID
	ChargerID        *uuid.UUID
	ConnectorID      *uuid.UUID
	ActorUserID      *uuid.UUID
	Kind             *string
	State            *string
	OCPPResult       *string
	FailureCategory  *string
	CreatedAfter     *time.Time
	CreatedBefore    *time.Time
	ResetType        *string
	AvailabilityType *string
	RequestedMessage *string
	ConfigurationKey *string
}

type ChargerOperationHistoryChargerView struct {
	ID        uuid.UUID `json:"id"`
	ChargerID string    `json:"charger_id"`
	Name      string    `json:"name"`
}

type ChargerOperationHistoryConnectorView struct {
	ID     uuid.UUID `json:"id"`
	Number int       `json:"number"`
	Type   string    `json:"type"`
}

type ChargerOperationHistoryActorView struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
}

// ChargerOperationHistoryParameters intentionally contains only typed,
// non-secret request semantics. In particular, ChangeConfiguration never
// returns its persisted value.
type ChargerOperationHistoryParameters struct {
	Type             string `json:"type,omitempty"`
	Reason           string `json:"reason,omitempty"`
	RequestedMessage string `json:"requested_message,omitempty"`
	Key              string `json:"key,omitempty"`
}

type ChargerOperationHistoryItem struct {
	ID              uuid.UUID                             `json:"id"`
	Kind            string                                `json:"kind"`
	State           string                                `json:"state"`
	OCPPResult      string                                `json:"ocpp_result,omitempty"`
	FailureCategory string                                `json:"failure_category,omitempty"`
	HALOperationID  *uuid.UUID                            `json:"hal_operation_id,omitempty"`
	Charger         ChargerOperationHistoryChargerView    `json:"charger"`
	Connector       *ChargerOperationHistoryConnectorView `json:"connector,omitempty"`
	Actor           ChargerOperationHistoryActorView      `json:"actor"`
	Parameters      *ChargerOperationHistoryParameters    `json:"parameters,omitempty"`
	CreatedAt       time.Time                             `json:"created_at"`
	UpdatedAt       time.Time                             `json:"updated_at"`
	CompletedAt     *time.Time                            `json:"completed_at,omitempty"`
}

type ChargerOperationHistoryResponse struct {
	Operations   []ChargerOperationHistoryItem `json:"operations"`
	NextBefore   *time.Time                    `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID                    `json:"next_before_id,omitempty"`
	HasMore      bool                          `json:"has_more"`
}

type chargerOperationHistoryRow struct {
	models.ChargerOperation
	ChargerCode     string  `gorm:"column:charger_code"`
	ChargerName     string  `gorm:"column:charger_name"`
	ConnectorNumber *int    `gorm:"column:connector_number"`
	ConnectorType   *string `gorm:"column:connector_type"`
	ActorFullName   string  `gorm:"column:actor_full_name"`
}

func validateChargerOperationHistoryQuery(query ChargerOperationHistoryQuery) (ChargerOperationHistoryQuery, error) {
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return ChargerOperationHistoryQuery{}, invalid("limit", "Limit must be between 1 and 200.")
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return ChargerOperationHistoryQuery{}, invalid("cursor", "before and before_id must be supplied together.")
	}
	if query.CreatedAfter != nil && query.CreatedBefore != nil && query.CreatedAfter.After(*query.CreatedBefore) {
		return ChargerOperationHistoryQuery{}, invalid("created_range", "created_after must not be after created_before.")
	}
	if err := normalizeHistoryEnum(&query.Kind, "kind", chargerOperationKinds, "A supported operation kind is required."); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryEnum(&query.State, "state", chargerOperationStates, "A supported operation state is required."); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryText(&query.OCPPResult, "ocpp_result", 64); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryText(&query.FailureCategory, "failure_category", 64); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryEnum(&query.ResetType, "reset_type", map[string]struct{}{"SOFT": {}, "HARD": {}}, "reset_type must be SOFT or HARD."); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryEnum(&query.AvailabilityType, "availability_type", map[string]struct{}{"OPERATIVE": {}, "INOPERATIVE": {}}, "availability_type must be OPERATIVE or INOPERATIVE."); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryExactEnum(&query.RequestedMessage, "requested_message", triggerMessageKinds, "requested_message is not supported."); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}
	if err := normalizeHistoryText(&query.ConfigurationKey, "configuration_key", 100); err != nil {
		return ChargerOperationHistoryQuery{}, err
	}

	parameterKinds := 0
	if query.ResetType != nil {
		parameterKinds++
	}
	if query.AvailabilityType != nil {
		parameterKinds++
	}
	if query.RequestedMessage != nil {
		parameterKinds++
	}
	if query.ConfigurationKey != nil {
		parameterKinds++
	}
	if parameterKinds > 1 {
		return ChargerOperationHistoryQuery{}, invalid("parameter_filter", "Only one typed operation-parameter filter may be supplied.")
	}
	for expected, supplied := range map[string]bool{
		"RESET":                query.ResetType != nil,
		"CHANGE_AVAILABILITY":  query.AvailabilityType != nil,
		"TRIGGER_MESSAGE":      query.RequestedMessage != nil,
		"CHANGE_CONFIGURATION": query.ConfigurationKey != nil,
	} {
		if !supplied {
			continue
		}
		if query.Kind == nil {
			kind := expected
			query.Kind = &kind
		} else if *query.Kind != expected {
			return ChargerOperationHistoryQuery{}, invalid("parameter_filter", "The typed parameter filter is incompatible with kind.")
		}
	}
	return query, nil
}

func normalizeHistoryEnum(value **string, field string, allowed map[string]struct{}, message string) error {
	if *value == nil {
		return nil
	}
	normalized := strings.ToUpper(strings.TrimSpace(**value))
	if _, ok := allowed[normalized]; !ok {
		return invalid(field, message)
	}
	*value = &normalized
	return nil
}

func normalizeHistoryText(value **string, field string, maximum int) error {
	if *value == nil {
		return nil
	}
	normalized := strings.TrimSpace(**value)
	if normalized == "" || len(normalized) > maximum {
		return invalid(field, field+" is invalid.")
	}
	*value = &normalized
	return nil
}

func normalizeHistoryExactEnum(value **string, field string, allowed map[string]struct{}, message string) error {
	if *value == nil {
		return nil
	}
	normalized := strings.TrimSpace(**value)
	if _, ok := allowed[normalized]; !ok {
		return invalid(field, message)
	}
	*value = &normalized
	return nil
}

func (service *Service) ListChargerOperationHistory(ctx context.Context, principal auth.Principal, query ChargerOperationHistoryQuery) (ChargerOperationHistoryResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerOperationHistoryResponse{}, err
	}
	query, err := validateChargerOperationHistoryQuery(query)
	if err != nil {
		return ChargerOperationHistoryResponse{}, err
	}
	cpoID := *principal.CPOID
	databaseQuery := service.database.WithContext(ctx).Table("charger_operations").
		Joins("JOIN chargers ON chargers.id = charger_operations.charger_id AND chargers.cpo_id = charger_operations.cpo_id").
		Joins("LEFT JOIN connectors ON connectors.id = charger_operations.connector_id AND connectors.cpo_id = charger_operations.cpo_id").
		Joins("JOIN users AS actors ON actors.id = charger_operations.actor_user_id").
		Where("charger_operations.cpo_id = ?", cpoID)
	if query.ChargerID != nil {
		databaseQuery = databaseQuery.Where("charger_operations.charger_id = ?", *query.ChargerID)
	}
	if query.ConnectorID != nil {
		databaseQuery = databaseQuery.Where("charger_operations.connector_id = ?", *query.ConnectorID)
	}
	if query.ActorUserID != nil {
		databaseQuery = databaseQuery.Where("charger_operations.actor_user_id = ?", *query.ActorUserID)
	}
	if query.Kind != nil {
		databaseQuery = databaseQuery.Where("charger_operations.kind = ?", *query.Kind)
	}
	if query.State != nil {
		databaseQuery = databaseQuery.Where("charger_operations.state = ?", *query.State)
	}
	if query.OCPPResult != nil {
		databaseQuery = databaseQuery.Where("charger_operations.ocpp_result = ?", *query.OCPPResult)
	}
	if query.FailureCategory != nil {
		databaseQuery = databaseQuery.Where("charger_operations.failure_category = ?", *query.FailureCategory)
	}
	if query.CreatedAfter != nil {
		databaseQuery = databaseQuery.Where("charger_operations.created_at >= ?", *query.CreatedAfter)
	}
	if query.CreatedBefore != nil {
		databaseQuery = databaseQuery.Where("charger_operations.created_at <= ?", *query.CreatedBefore)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(charger_operations.created_at, charger_operations.id) < (?, ?)", *query.Before, *query.BeforeID)
	}
	if query.ResetType != nil {
		databaseQuery = databaseQuery.Where("charger_operations.parameters ->> 'type' = ?", *query.ResetType)
	}
	if query.AvailabilityType != nil {
		databaseQuery = databaseQuery.Where("charger_operations.parameters ->> 'type' = ?", *query.AvailabilityType)
	}
	if query.RequestedMessage != nil {
		databaseQuery = databaseQuery.Where("charger_operations.parameters ->> 'requested_message' = ?", *query.RequestedMessage)
	}
	if query.ConfigurationKey != nil {
		databaseQuery = databaseQuery.Where("charger_operations.parameters ->> 'key' = ?", *query.ConfigurationKey)
	}

	var rows []chargerOperationHistoryRow
	if err := databaseQuery.Select(
		"charger_operations.*", "chargers.charger_id AS charger_code", "chargers.charger_name", "connectors.connector_number", "connectors.connector_type", "actors.full_name AS actor_full_name",
	).Order("charger_operations.created_at DESC, charger_operations.id DESC").Limit(query.Limit + 1).Scan(&rows).Error; err != nil {
		return ChargerOperationHistoryResponse{}, fmt.Errorf("list CPO charger operations: %w", err)
	}
	hasMore := len(rows) > query.Limit
	if hasMore {
		rows = rows[:query.Limit]
	}
	response := ChargerOperationHistoryResponse{Operations: make([]ChargerOperationHistoryItem, 0, len(rows)), HasMore: hasMore}
	for _, row := range rows {
		response.Operations = append(response.Operations, chargerOperationHistoryView(row))
	}
	if hasMore && len(rows) > 0 {
		next := rows[len(rows)-1]
		response.NextBefore, response.NextBeforeID = &next.CreatedAt, &next.ID
	}
	return response, nil
}

func chargerOperationHistoryView(row chargerOperationHistoryRow) ChargerOperationHistoryItem {
	item := ChargerOperationHistoryItem{
		ID: row.ID, Kind: row.Kind, State: row.State, OCPPResult: row.OCPPResult, FailureCategory: row.FailureCategory, HALOperationID: row.HALOperationID,
		Charger:    ChargerOperationHistoryChargerView{ID: row.ChargerID, ChargerID: row.ChargerCode, Name: row.ChargerName},
		Actor:      ChargerOperationHistoryActorView{ID: row.ActorUserID, FullName: row.ActorFullName},
		Parameters: safeChargerOperationHistoryParameters(row.Kind, row.Parameters), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, CompletedAt: row.CompletedAt,
	}
	if row.ConnectorID != nil {
		connectorType := ""
		if row.ConnectorType != nil {
			connectorType = *row.ConnectorType
		}
		connectorNumber := 0
		if row.ConnectorNumber != nil {
			connectorNumber = *row.ConnectorNumber
		}
		item.Connector = &ChargerOperationHistoryConnectorView{ID: *row.ConnectorID, Number: connectorNumber, Type: connectorType}
	}
	return item
}

func safeChargerOperationHistoryParameters(kind string, parameters models.JSONB) *ChargerOperationHistoryParameters {
	text := func(key string) string { value, _ := parameters[key].(string); return value }
	switch kind {
	case "RESET":
		return &ChargerOperationHistoryParameters{Type: text("type"), Reason: text("reason")}
	case "CHANGE_AVAILABILITY":
		return &ChargerOperationHistoryParameters{Type: text("type")}
	case "TRIGGER_MESSAGE":
		return &ChargerOperationHistoryParameters{RequestedMessage: text("requested_message")}
	case "CHANGE_CONFIGURATION":
		return &ChargerOperationHistoryParameters{Key: text("key")}
	default:
		return nil
	}
}
