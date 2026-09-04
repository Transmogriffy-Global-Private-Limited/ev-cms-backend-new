package halops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

var ErrChargerOperationNotFound = errors.New("HAL charger operation not found")

type ChargerOperationRequest struct {
	CMSOperationID      uuid.UUID
	CPOID               uuid.UUID
	CMSChargerID        uuid.UUID
	CMSConnectorID      *uuid.UUID
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	Kind                string
	Parameters          map[string]string
}

type ChargerOperation struct {
	HALOperationID   uuid.UUID
	CMSOperationID   uuid.UUID
	Kind             string
	State            string
	OCPPResult       string
	ErrorCategory    string
	DeliveryAttempts int
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}

func fromWireChargerOperation(operation halclient.ChargerOperation) ChargerOperation {
	return ChargerOperation{HALOperationID: operation.HALOperationID, CMSOperationID: operation.CMSOperationID, Kind: operation.Kind, State: operation.State, OCPPResult: operation.OCPPResult, ErrorCategory: operation.ErrorCategory, DeliveryAttempts: operation.DeliveryAttempts, UpdatedAt: operation.UpdatedAt, CompletedAt: operation.CompletedAt}
}

func (service *Service) RequestChargerOperation(ctx context.Context, request ChargerOperationRequest, correlationID string) (ChargerOperation, error) {
	if !service.Available() {
		return ChargerOperation{}, halclient.ErrUnavailable
	}
	operation, err := service.client.OperateCharger(ctx, halclient.ChargerOperationRequest{CMSOperationID: request.CMSOperationID, CPOID: request.CPOID, CMSChargerID: request.CMSChargerID, CMSConnectorID: request.CMSConnectorID, ChargerOCPPIdentity: request.ChargerOCPPIdentity, OCPPConnectorNumber: request.OCPPConnectorNumber, Kind: request.Kind, Parameters: request.Parameters}, correlationID)
	return fromWireChargerOperation(operation), err
}

// ReconcileChargerOperation uses the original CMS identity only. An exact HAL
// 404 is confirmed absence; a transport error remains delivery uncertainty.
func (service *Service) ReconcileChargerOperation(ctx context.Context, operationID uuid.UUID) (ChargerOperation, error) {
	if !service.Available() {
		return ChargerOperation{}, halclient.ErrUnavailable
	}
	operation, err := service.client.GetChargerOperation(ctx, operationID)
	if err != nil {
		if isHALHTTPStatus(err, 404) {
			return ChargerOperation{}, fmt.Errorf("%w: %s", ErrChargerOperationNotFound, operationID)
		}
		return ChargerOperation{}, err
	}
	result := fromWireChargerOperation(operation)
	updates := map[string]any{"hal_operation_id": result.HALOperationID, "state": result.State, "ocpp_result": result.OCPPResult, "failure_category": result.ErrorCategory, "updated_at": service.now(), "completed_at": result.CompletedAt}
	if err := service.database.WithContext(ctx).Model(&models.ChargerOperation{}).Where("id = ?", operationID).Updates(updates).Error; err != nil {
		return ChargerOperation{}, fmt.Errorf("store reconciled charger operation: %w", err)
	}
	return result, nil
}

func (service *Service) GetChargerConfiguration(ctx context.Context, cpoID, chargerID uuid.UUID, identity string, keys []string) ([]halclient.ChargerConfigurationKey, []string, error) {
	if !service.Available() {
		return nil, nil, halclient.ErrUnavailable
	}
	return service.client.GetChargerConfiguration(ctx, cpoID, chargerID, identity, keys)
}
