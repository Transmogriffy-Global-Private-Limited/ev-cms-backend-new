package cpo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChargerOperationInput struct {
	Kind              string
	ConnectorID       *uuid.UUID
	Parameters        map[string]string
	ConfigurationKeys []string
	IdempotencyKey    string
	CorrelationID     string
}

type ChargerOperationResponse struct {
	ID              uuid.UUID                               `json:"id"`
	ChargerID       uuid.UUID                               `json:"charger_id"`
	ConnectorID     *uuid.UUID                              `json:"connector_id,omitempty"`
	Kind            string                                  `json:"kind"`
	State           string                                  `json:"state"`
	OCPPResult      string                                  `json:"ocpp_result,omitempty"`
	FailureCategory string                                  `json:"failure_category,omitempty"`
	HALOperationID  *uuid.UUID                              `json:"hal_operation_id,omitempty"`
	TraceID         uuid.UUID                               `json:"trace_id"`
	CreatedAt       time.Time                               `json:"created_at"`
	UpdatedAt       time.Time                               `json:"updated_at"`
	CompletedAt     *time.Time                              `json:"completed_at,omitempty"`
	Configuration   *halclient.ChargerConfigurationResponse `json:"-"`
}

func chargerOperationView(operation models.ChargerOperation) ChargerOperationResponse {
	return ChargerOperationResponse{ID: operation.ID, ChargerID: operation.ChargerID, ConnectorID: operation.ConnectorID, Kind: operation.Kind, State: operation.State, OCPPResult: operation.OCPPResult, FailureCategory: operation.FailureCategory, HALOperationID: operation.HALOperationID, TraceID: operation.TraceID, CreatedAt: operation.CreatedAt, UpdatedAt: operation.UpdatedAt, CompletedAt: operation.CompletedAt}
}

func chargerOperationDigest(input ChargerOperationInput) (string, error) {
	raw, err := json.Marshal(struct {
		Kind              string            `json:"kind"`
		ConnectorID       *uuid.UUID        `json:"connector_id,omitempty"`
		Parameters        map[string]string `json:"parameters"`
		ConfigurationKeys []string          `json:"configuration_keys,omitempty"`
	}{input.Kind, input.ConnectorID, input.Parameters, input.ConfigurationKeys})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (service *Service) RequestChargerOperation(ctx context.Context, principal auth.Principal, chargerID uuid.UUID, input ChargerOperationInput) (ChargerOperationResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerOperationResponse{}, err
	}
	if service.halOperations == nil || !service.halOperations.Available() {
		return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusServiceUnavailable, Code: "hal_unavailable", Message: "Charger operations are temporarily unavailable."}
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 128 {
		return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_idempotency_key", Message: "A bounded Idempotency-Key is required."}
	}
	digest, err := chargerOperationDigest(input)
	if err != nil {
		return ChargerOperationResponse{}, fmt.Errorf("digest charger operation: %w", err)
	}
	var existing models.ChargerOperation
	err = service.database.WithContext(ctx).Where("cpo_id = ? AND idempotency_key = ?", *principal.CPOID, input.IdempotencyKey).First(&existing).Error
	if err == nil {
		if existing.RequestDigest != digest || existing.ChargerID != chargerID {
			return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusConflict, Code: "idempotency_conflict", Message: "The Idempotency-Key was already used for a different operation."}
		}
		return service.dispatchChargerOperation(ctx, existing.ID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ChargerOperationResponse{}, fmt.Errorf("load idempotent charger operation: %w", err)
	}
	var charger models.Charger
	if err := service.database.WithContext(ctx).First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
		return ChargerOperationResponse{}, mapChargerNotFound(err)
	}
	connectorNumber := 0
	if input.ConnectorID != nil {
		var connector models.Connector
		if err := service.database.WithContext(ctx).First(&connector, "id = ? AND cpo_id = ? AND charger_id = ?", *input.ConnectorID, *principal.CPOID, chargerID).Error; err != nil {
			return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusNotFound, Code: "connector_not_found", Message: "The connector was not found."}
		}
		connectorNumber = connector.ConnectorNumber
	}
	var mapping models.HALChargerMapping
	if err := service.database.WithContext(ctx).First(&mapping, "cms_charger_id = ? AND cpo_id = ? AND sync_state = ?", chargerID, *principal.CPOID, "SYNCHRONIZED").Error; err != nil {
		return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusServiceUnavailable, Code: "mapping_unavailable", Message: "The charger mapping is not ready for operations."}
	}
	operation := models.ChargerOperation{ID: uuid.New(), TraceID: uuid.New(), CPOID: *principal.CPOID, ChargerID: chargerID, ConnectorID: input.ConnectorID, ActorUserID: principal.UserID, IdempotencyKey: input.IdempotencyKey, RequestDigest: digest, CorrelationID: input.CorrelationID, Kind: input.Kind, Parameters: models.JSONB{}, State: "PERSISTED", DispatchChargerIdentity: &mapping.ChargerOCPPIdentity, DispatchConnectorNumber: &connectorNumber, CreatedAt: service.now(), UpdatedAt: service.now()}
	for key, value := range input.Parameters {
		if key != "_connector_number" {
			operation.Parameters[key] = value
		}
	}
	if input.Kind == "GET_CONFIGURATION" {
		operation.Parameters["configuration_keys"] = append([]string(nil), input.ConfigurationKeys...)
	}
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		insert := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cpo_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&operation)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			operation = models.ChargerOperation{} // Do not constrain the winner lookup by our losing UUID.
			if err := tx.Where("cpo_id = ? AND idempotency_key = ?", *principal.CPOID, input.IdempotencyKey).First(&operation).Error; err != nil {
				return err
			}
			if operation.RequestDigest != digest || operation.ChargerID != chargerID {
				return &auth.APIError{Status: http.StatusConflict, Code: "idempotency_conflict", Message: "The Idempotency-Key was already used for a different operation."}
			}
			return nil
		}
		return service.emitChargerOperationEvent(tx, operation)
	}); err != nil {
		return ChargerOperationResponse{}, fmt.Errorf("persist charger operation: %w", err)
	}
	return service.dispatchChargerOperation(ctx, operation.ID)
}

func operationParameters(parameters models.JSONB) map[string]string {
	result := map[string]string{}
	for key, value := range parameters {
		if text, ok := value.(string); ok {
			result[key] = text
		}
	}
	return result
}
func chargerOperationFailure(err error) string {
	if errors.Is(err, halclient.ErrUnavailable) {
		return "hal_unavailable"
	}
	var provider *halclient.HTTPError
	if errors.As(err, &provider) {
		return "provider_http"
	}
	return "delivery_uncertain"
}

func (service *Service) emitChargerOperationEvent(tx *gorm.DB, operation models.ChargerOperation) error {
	if service.operationalEvents == nil {
		return nil
	}
	_, err := service.operationalEvents.Emit(tx, operationalrealtime.Input{CPOID: operation.CPOID, Type: "charger.operation_changed", ResourceType: "CHARGER_OPERATION", ResourceID: operation.ID.String()})
	return err
}

func (service *Service) GetChargerOperation(ctx context.Context, principal auth.Principal, id uuid.UUID) (ChargerOperationResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerOperationResponse{}, err
	}
	var operation models.ChargerOperation
	if err := service.database.WithContext(ctx).First(&operation, "id = ? AND cpo_id = ?", id, *principal.CPOID).Error; err != nil {
		return ChargerOperationResponse{}, &auth.APIError{Status: http.StatusNotFound, Code: "charger_operation_not_found", Message: "The charger operation was not found."}
	}
	if operationNeedsReconciliation(operation.State) && service.halOperations != nil && service.halOperations.Available() {
		if err := service.reconcileChargerOperation(ctx, operation.ID); err != nil {
			return ChargerOperationResponse{}, err
		}
		if err := service.database.WithContext(ctx).First(&operation, "id = ?", operation.ID).Error; err != nil {
			return ChargerOperationResponse{}, err
		}
	}
	return chargerOperationView(operation), nil
}

type ChargerConfigurationResponse struct {
	ConfigurationKeys []halclient.ChargerConfigurationKey `json:"configuration_keys"`
	UnknownKeys       []string                            `json:"unknown_keys"`
}

func (service *Service) GetChargerConfiguration(ctx context.Context, principal auth.Principal, chargerID uuid.UUID, keys []string) (ChargerConfigurationResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargerConfigurationResponse{}, err
	}
	if service.halOperations == nil || !service.halOperations.Available() {
		return ChargerConfigurationResponse{}, &auth.APIError{Status: http.StatusServiceUnavailable, Code: "hal_unavailable", Message: "Charger operations are temporarily unavailable."}
	}
	var charger models.Charger
	if err := service.database.WithContext(ctx).First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
		return ChargerConfigurationResponse{}, mapChargerNotFound(err)
	}
	var mapping models.HALChargerMapping
	if err := service.database.WithContext(ctx).First(&mapping, "cms_charger_id = ? AND cpo_id = ? AND sync_state = ?", chargerID, *principal.CPOID, "SYNCHRONIZED").Error; err != nil {
		return ChargerConfigurationResponse{}, &auth.APIError{Status: http.StatusServiceUnavailable, Code: "mapping_unavailable", Message: "The charger mapping is not ready for operations."}
	}
	items, unknown, err := service.halOperations.GetChargerConfiguration(ctx, *principal.CPOID, charger.ID, mapping.ChargerOCPPIdentity, keys)
	if err != nil {
		return ChargerConfigurationResponse{}, &auth.APIError{Status: http.StatusServiceUnavailable, Code: "charger_not_connected", Message: "The charger configuration is not currently available."}
	}
	return ChargerConfigurationResponse{ConfigurationKeys: items, UnknownKeys: unknown}, nil
}
