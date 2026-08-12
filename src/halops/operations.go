// Package halops owns CMS-side operational integration mechanics above the
// frozen HAL wire adapter. Business services authorize requests before calling
// this package; this package never grants business authority.
package halops

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halclient"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Service is the CMS capability for HAL mapping, command delivery, and exact
// identity reconciliation. It deliberately exposes CMS-shaped request types,
// leaving halclient wire DTOs private to this integration boundary.
type Service struct {
	database *gorm.DB
	client   *halclient.Client
	now      func() time.Time
}

func New(database *gorm.DB, cfg config.HAL) *Service {
	return &Service{database: database, client: halclient.New(cfg), now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) Available() bool {
	return service != nil && service.client != nil && service.client.Available()
}

type ConnectorMapping struct {
	CMSConnectorID      uuid.UUID
	OCPPConnectorNumber int
}

type ChargerMapping struct {
	CPOID               uuid.UUID
	CMSChargerID        uuid.UUID
	ChargerOCPPIdentity string
	Enabled             bool
	Connectors          []ConnectorMapping
}

type StartRequest struct {
	CMSCommandID        uuid.UUID
	CMSStartIntentID    uuid.UUID
	CPOID               uuid.UUID
	CustomerID          uuid.UUID
	CMSChargerID        uuid.UUID
	CMSConnectorID      uuid.UUID
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	Credential          string
	CredentialExpiresAt time.Time
	CommandExpiresAt    time.Time
	EnergyLimitWh       int64
	MaxDurationSeconds  int64
}

type StopRequest struct {
	CMSCommandID           uuid.UUID
	CMSChargingSessionID   uuid.UUID
	CPOID                  uuid.UUID
	CustomerID             uuid.UUID
	CMSChargerID           uuid.UUID
	CMSConnectorID         uuid.UUID
	ChargerOCPPIdentity    string
	OCPPConnectorNumber    int
	HALTransactionID       uuid.UUID
	OCPPTransactionID      int64
	RequestedStopInitiator string
	RequestedStopReason    string
	CommandExpiresAt       time.Time
}

type Command struct {
	HALCommandID      uuid.UUID
	CMSCommandID      uuid.UUID
	Kind              string
	State             string
	HALTransactionID  *uuid.UUID
	OCPPTransactionID *int64
	UpdatedAt         time.Time
}

func fromWireCommand(command halclient.Command) Command {
	return Command{HALCommandID: command.HALCommandID, CMSCommandID: command.CMSCommandID, Kind: command.Kind, State: command.State, HALTransactionID: command.HALTransactionID, OCPPTransactionID: command.OCPPTransactionID, UpdatedAt: command.UpdatedAt}
}

func (service *Service) SyncMapping(ctx context.Context, mapping ChargerMapping, correlationID string) error {
	if !service.Available() {
		return halclient.ErrUnavailable
	}
	connectors := make([]halclient.ConnectorMapping, 0, len(mapping.Connectors))
	for _, connector := range mapping.Connectors {
		connectors = append(connectors, halclient.ConnectorMapping{CMSConnectorID: connector.CMSConnectorID, OCPPConnectorNumber: connector.OCPPConnectorNumber})
	}
	err := service.client.SyncMapping(ctx, halclient.ChargerMapping{CPOID: mapping.CPOID, CMSChargerID: mapping.CMSChargerID, ChargerOCPPIdentity: mapping.ChargerOCPPIdentity, Enabled: mapping.Enabled, Connectors: connectors}, correlationID)
	service.recordMappingOutcome(ctx, mapping.CMSChargerID, err)
	return err
}

// EnsureChargerMapping builds the mapping from committed CMS inventory. A
// failed provider attempt leaves the durable mapping pending for reconciliation.
func (service *Service) EnsureChargerMapping(ctx context.Context, chargerID uuid.UUID, correlationID string) error {
	var charger models.Charger
	if err := service.database.WithContext(ctx).Preload("Connectors").First(&charger, "id = ?", chargerID).Error; err != nil {
		return fmt.Errorf("load charger mapping: %w", err)
	}
	mapping := ChargerMapping{CPOID: charger.CPOID, CMSChargerID: charger.ID, ChargerOCPPIdentity: charger.OCPPIdentity, Enabled: charger.Status == "ACTIVE", Connectors: make([]ConnectorMapping, 0, len(charger.Connectors))}
	for _, connector := range charger.Connectors {
		mapping.Connectors = append(mapping.Connectors, ConnectorMapping{CMSConnectorID: connector.ID, OCPPConnectorNumber: connector.ConnectorNumber})
	}
	if len(mapping.Connectors) == 0 {
		return errors.New("charger mapping requires at least one connector")
	}
	return service.SyncMapping(ctx, mapping, correlationID)
}

func (service *Service) RequestStart(ctx context.Context, request StartRequest, correlationID string) (Command, error) {
	if !service.Available() {
		return Command{}, halclient.ErrUnavailable
	}
	command, err := service.client.Start(ctx, halclient.StartCommand{CMSCommandID: request.CMSCommandID, CMSStartIntentID: request.CMSStartIntentID, CPOID: request.CPOID, CustomerID: request.CustomerID, CMSChargerID: request.CMSChargerID, CMSConnectorID: request.CMSConnectorID, ChargerOCPPIdentity: request.ChargerOCPPIdentity, OCPPConnectorNumber: request.OCPPConnectorNumber, IDTag: request.Credential, CredentialExpiresAt: request.CredentialExpiresAt, CommandExpiresAt: request.CommandExpiresAt, EnergyLimitWh: request.EnergyLimitWh, MaxDurationSeconds: request.MaxDurationSeconds}, correlationID)
	return fromWireCommand(command), err
}

func (service *Service) RequestStop(ctx context.Context, request StopRequest, correlationID string) (Command, error) {
	if !service.Available() {
		return Command{}, halclient.ErrUnavailable
	}
	command, err := service.client.Stop(ctx, halclient.StopCommand{CMSCommandID: request.CMSCommandID, CMSChargingSessionID: request.CMSChargingSessionID, CPOID: request.CPOID, CustomerID: request.CustomerID, CMSChargerID: request.CMSChargerID, CMSConnectorID: request.CMSConnectorID, ChargerOCPPIdentity: request.ChargerOCPPIdentity, OCPPConnectorNumber: request.OCPPConnectorNumber, HALTransactionID: request.HALTransactionID, OCPPTransactionID: request.OCPPTransactionID, RequestedStopInitiator: request.RequestedStopInitiator, RequestedStopReason: request.RequestedStopReason, CommandExpiresAt: request.CommandExpiresAt}, correlationID)
	return fromWireCommand(command), err
}

// ReconcileCommand queries only the durable CMS command identity. It never
// guesses a transaction from a charger or creates a replacement command.
func (service *Service) ReconcileCommand(ctx context.Context, commandID uuid.UUID) (Command, error) {
	if !service.Available() {
		return Command{}, halclient.ErrUnavailable
	}
	command, err := service.client.GetCommand(ctx, commandID)
	if err != nil {
		service.recordCommandFailure(ctx, commandID, err)
		return Command{}, err
	}
	result := fromWireCommand(command)
	if err := service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{"hal_command_id": result.HALCommandID, "state": result.State, "last_error_category": "", "last_error_detail": "", "updated_at": service.now()}).Error; err != nil {
		return Command{}, fmt.Errorf("store reconciled HAL command: %w", err)
	}
	return result, nil
}

func (service *Service) recordMappingOutcome(ctx context.Context, chargerID uuid.UUID, outcome error) {
	now := service.now()
	updates := map[string]any{"updated_at": now}
	if outcome == nil {
		updates["sync_state"] = "SYNCHRONIZED"
		updates["last_sync_error"] = ""
		updates["last_synchronized_at"] = now
	} else {
		updates["sync_state"] = "RECONCILIATION_REQUIRED"
		updates["last_sync_error"] = "HAL mapping delivery requires reconciliation"
	}
	_ = service.database.WithContext(ctx).Model(&models.HALChargerMapping{}).Where("cms_charger_id = ?", chargerID).Updates(updates).Error
}

func (service *Service) recordCommandFailure(ctx context.Context, commandID uuid.UUID, cause error) {
	category := "transport"
	if httpError := new(halclient.HTTPError); errors.As(cause, &httpError) {
		category = "provider_http"
	}
	_ = service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{"state": "RECONCILIATION_REQUIRED", "last_error_category": category, "last_error_detail": "HAL command requires exact-identity reconciliation", "updated_at": service.now()}).Error
}

// ReconcilePending performs one bounded recovery pass. Missing provider state
// remains observable; it is never converted into a fabricated session result.
func (service *Service) ReconcilePending(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 50
	}
	var mappings []models.HALChargerMapping
	if err := service.database.WithContext(ctx).Where("sync_state IN ?", []string{"PENDING", "RECONCILIATION_REQUIRED"}).Order("updated_at ASC").Limit(limit).Find(&mappings).Error; err != nil {
		return fmt.Errorf("list pending HAL mappings: %w", err)
	}
	for _, mapping := range mappings {
		_ = service.EnsureChargerMapping(ctx, mapping.CMSChargerID, "hal-mapping-reconciliation")
	}
	var commands []models.HALCommandRecord
	if err := service.database.WithContext(ctx).Where("state = ?", "RECONCILIATION_REQUIRED").Order("updated_at ASC").Limit(limit).Find(&commands).Error; err != nil {
		return fmt.Errorf("list pending HAL commands: %w", err)
	}
	for _, command := range commands {
		_, _ = service.ReconcileCommand(ctx, command.CMSCommandID)
	}
	return nil
}

func (service *Service) RunReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := service.ReconcilePending(ctx, 50); err != nil && ctx.Err() == nil {
			log.Printf("HAL reconciliation pass failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func trimError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
