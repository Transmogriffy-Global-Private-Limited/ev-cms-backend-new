// Package halops owns CMS-side operational integration mechanics above the
// frozen HAL wire adapter. Business services authorize requests before calling
// this package; this package never grants business authority.
package halops

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	database                  *gorm.DB
	client                    *halclient.Client
	now                       func() time.Time
	startCommandAbsentHandler StartCommandAbsentHandler
	startMaterializer         StartMaterializer
	startReconcileAfter       time.Duration
}

// StartCommandAbsentHandler lets the charging domain own the financial
// consequence of an exact HAL command absence without making halops depend on
// customerauth. The handler must be transactional and idempotent.
type StartCommandAbsentHandler func(context.Context, uuid.UUID) error

// StartEvidence is HAL's durable, authoritative OCPP start truth normalized
// for the CMS business projection. It is transport-neutral so the same
// materializer serves immutable fact ingestion and exact HAL lookup recovery.
type StartEvidence struct {
	HALTransactionID    uuid.UUID
	HALCommandID        uuid.UUID
	CMSCommandID        uuid.UUID
	CMSStartIntentID    uuid.UUID
	CPOID               uuid.UUID
	CMSChargerID        uuid.UUID
	CMSConnectorID      uuid.UUID
	ChargerOCPPIdentity string
	OCPPConnectorNumber int
	OCPPTransactionID   int64
	MeterStartWh        int64
	ActualStartedAt     time.Time
}

// StartMaterializer belongs to the charging domain because it owns sessions,
// wallet holds, and customer-visible projection state. halops only discovers
// durable HAL truth and invokes this explicit socket.
type StartMaterializer func(context.Context, StartEvidence) error

// ErrCommandNotFound is returned only when HAL's exact CMS command lookup
// responds with HTTP 404. It is authoritative absence, not a transport error.
var ErrCommandNotFound = errors.New("HAL command not found")

func New(database *gorm.DB, cfg config.HAL) *Service {
	return &Service{database: database, client: halclient.New(cfg), now: func() time.Time { return time.Now().UTC() }, startReconcileAfter: cfg.StartReconcileAfter}
}

func (service *Service) Available() bool {
	return service != nil && service.client != nil && service.client.Available()
}

// WithStartCommandAbsentHandler connects exact command lookup to the business
// owner of start-intent and wallet-hold state.
func (service *Service) WithStartCommandAbsentHandler(handler StartCommandAbsentHandler) *Service {
	service.startCommandAbsentHandler = handler
	return service
}

func (service *Service) WithStartMaterializer(materializer StartMaterializer) *Service {
	service.startMaterializer = materializer
	return service
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
		if isHALHTTPStatus(err, 404) {
			return Command{}, fmt.Errorf("%w: %s", ErrCommandNotFound, commandID)
		}
		return Command{}, err
	}
	result := fromWireCommand(command)
	if result.CMSCommandID != commandID {
		return Command{}, errors.New("HAL exact command lookup returned a different CMS command identity")
	}
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
		updates["last_sync_error"] = "HAL mapping prerequisite synchronization failed"
	}
	_ = service.database.WithContext(ctx).Model(&models.HALChargerMapping{}).Where("cms_charger_id = ?", chargerID).Updates(updates).Error
}

func isHALHTTPStatus(cause error, status int) bool {
	var httpError *halclient.HTTPError
	return errors.As(cause, &httpError) && httpError.Status == status
}

func commandErrorCategory(cause error) string {
	if errors.Is(cause, halclient.ErrUnavailable) {
		return "hal_unavailable"
	}
	var httpError *halclient.HTTPError
	if errors.As(cause, &httpError) {
		return "provider_http"
	}
	return "transport"
}

func commandLookupFailureDetail(cause error) string {
	var httpError *halclient.HTTPError
	if errors.As(cause, &httpError) {
		return fmt.Sprintf("HAL exact command lookup returned HTTP %d; reconciliation will retry", httpError.Status)
	}
	if errors.Is(cause, halclient.ErrUnavailable) {
		return "HAL exact command lookup is unavailable; reconciliation will retry"
	}
	return "HAL exact command lookup transport outcome is unknown; reconciliation will retry"
}

func (service *Service) recordCommandLookupFailure(ctx context.Context, commandID uuid.UUID, cause error) {
	_ = service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ?", commandID).Updates(map[string]any{
		"state":               "RECONCILIATION_REQUIRED",
		"last_error_category": commandErrorCategory(cause),
		"last_error_detail":   commandLookupFailureDetail(cause),
		"updated_at":          service.now(),
	}).Error
}

func (service *Service) recordStopCommandAbsent(ctx context.Context, commandID uuid.UUID) {
	_ = service.database.WithContext(ctx).Model(&models.HALCommandRecord{}).Where("cms_command_id = ? AND kind = ?", commandID, "STOP").Updates(map[string]any{
		"state":               "RECONCILIATION_REQUIRED",
		"last_error_category": "confirmed_absent",
		"last_error_detail":   "HAL exact stop-command lookup found no durable command; stop reconciliation remains pending",
		"updated_at":          service.now(),
	}).Error
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
		_, err := service.ReconcileCommand(ctx, command.CMSCommandID)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrCommandNotFound) {
			if command.Kind != "START" {
				// A missing STOP command cannot establish that an already-started
				// session stopped. Keep its conservative reconciliation state.
				service.recordStopCommandAbsent(ctx, command.CMSCommandID)
				continue
			}
			if service.startCommandAbsentHandler == nil {
				service.recordCommandLookupFailure(ctx, command.CMSCommandID, errors.New("start command absence handler is unavailable"))
				continue
			}
			if err := service.startCommandAbsentHandler(ctx, command.CMSCommandID); err != nil {
				return fmt.Errorf("terminalize confirmed-absent start command %s: %w", command.CMSCommandID, err)
			}
			continue
		}
		service.recordCommandLookupFailure(ctx, command.CMSCommandID, err)
	}
	return service.reconcileStrandedStarts(ctx, limit)
}

// reconcileStrandedStarts closes the gap in which HAL accepted/delivered a
// start and later materialized charger truth, but the immutable fact never
// became a CMS projection. It queries only the exact CMS start-intent ID; a
// 404 is absence of HAL transaction truth, never permission to fabricate one.
func (service *Service) reconcileStrandedStarts(ctx context.Context, limit int) error {
	if service.startMaterializer == nil || !service.Available() {
		return nil
	}
	after := service.startReconcileAfter
	if after <= 0 {
		after = 2 * time.Minute
	}
	var intents []models.ChargingStartIntent
	if err := service.database.WithContext(ctx).
		Where("status IN ? AND materialized_session_id IS NULL AND updated_at <= ?", []string{"ACCEPTED_FOR_DELIVERY", "PROTOCOL_ACKNOWLEDGED", "RECONCILIATION_REQUIRED"}, service.now().Add(-after)).
		Order("updated_at ASC").Limit(limit).Find(&intents).Error; err != nil {
		return fmt.Errorf("list stranded HAL start intents: %w", err)
	}
	for _, intent := range intents {
		transaction, err := service.client.GetTransactionByStartIntent(ctx, intent.ID)
		if err == nil {
			command, commandErr := service.ReconcileCommand(ctx, transaction.CMSCommandID)
			if commandErr != nil {
				service.markStartReconciliation(ctx, intent.ID, commandLookupFailureDetail(commandErr))
				continue
			}
			evidence, conversionErr := startEvidenceFromTransaction(transaction, command.HALCommandID)
			if conversionErr != nil {
				service.markStartReconciliation(ctx, intent.ID, "HAL transaction lookup returned invalid authoritative start evidence")
				continue
			}
			if evidence.CMSStartIntentID != intent.ID {
				service.markStartReconciliation(ctx, intent.ID, "HAL transaction lookup returned a different start intent")
				continue
			}
			if err := service.startMaterializer(ctx, evidence); err != nil {
				return fmt.Errorf("materialize reconciled HAL start %s: %w", intent.ID, err)
			}
			continue
		}
		if isHALHTTPStatus(err, 404) {
			// Reconcile exact command state before making the unresolved outcome
			// customer-visible. A command response may still become late evidence.
			service.reconcileStartCommandAfterMissingTransaction(ctx, intent.ID)
			continue
		}
		service.markStartReconciliation(ctx, intent.ID, commandLookupFailureDetail(err))
	}
	return nil
}

func startEvidenceFromTransaction(transaction halclient.Transaction, halCommandID uuid.UUID) (StartEvidence, error) {
	if transaction.HALTransactionID == uuid.Nil || transaction.CMSStartIntentID == uuid.Nil || transaction.CMSCommandID == uuid.Nil || transaction.CPOID == uuid.Nil || transaction.CMSChargerID == uuid.Nil || transaction.CMSConnectorID == uuid.Nil || transaction.OCPPTransactionID < 1 || transaction.MeterStartWh < 0 || transaction.ActualStartedAt.IsZero() || transaction.OCPPConnectorNumber < 1 || transaction.ChargerOCPPIdentity == "" {
		return StartEvidence{}, errors.New("HAL transaction lookup omitted required start evidence")
	}
	if halCommandID == uuid.Nil {
		return StartEvidence{}, errors.New("HAL command lookup omitted HAL command identity")
	}
	return StartEvidence{HALTransactionID: transaction.HALTransactionID, HALCommandID: halCommandID, CMSCommandID: transaction.CMSCommandID, CMSStartIntentID: transaction.CMSStartIntentID, CPOID: transaction.CPOID, CMSChargerID: transaction.CMSChargerID, CMSConnectorID: transaction.CMSConnectorID, ChargerOCPPIdentity: transaction.ChargerOCPPIdentity, OCPPConnectorNumber: transaction.OCPPConnectorNumber, OCPPTransactionID: transaction.OCPPTransactionID, MeterStartWh: transaction.MeterStartWh, ActualStartedAt: transaction.ActualStartedAt}, nil
}

func (service *Service) reconcileStartCommandAfterMissingTransaction(ctx context.Context, intentID uuid.UUID) {
	var command models.HALCommandRecord
	if err := service.database.WithContext(ctx).First(&command, "start_intent_id = ? AND kind = ?", intentID, "START").Error; err != nil {
		service.markStartReconciliation(ctx, intentID, "CMS start command could not be loaded for exact HAL reconciliation")
		return
	}
	if _, err := service.ReconcileCommand(ctx, command.CMSCommandID); err != nil && !errors.Is(err, ErrCommandNotFound) {
		service.markStartReconciliation(ctx, intentID, commandLookupFailureDetail(err))
		return
	}
	service.markStartReconciliation(ctx, intentID, "HAL has no materialized transaction for the start intent; exact command reconciliation remains required")
}

func (service *Service) markStartReconciliation(ctx context.Context, intentID uuid.UUID, detail string) {
	now := service.now()
	_ = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.HALCommandRecord{}).Where("start_intent_id = ? AND kind = ? AND state <> ?", intentID, "START", "MATERIALIZED").Updates(map[string]any{"state": "RECONCILIATION_REQUIRED", "last_error_category": "start_projection_reconciliation", "last_error_detail": detail, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChargingStartIntent{}).Where("id = ? AND status IN ? AND materialized_session_id IS NULL", intentID, []string{"ACCEPTED_FOR_DELIVERY", "PROTOCOL_ACKNOWLEDGED", "RECONCILIATION_REQUIRED"}).Updates(map[string]any{"status": "RECONCILIATION_REQUIRED", "updated_at": now}).Error
	})
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
