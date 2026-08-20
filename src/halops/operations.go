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
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/workerobs"
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
	stopCommandAbsentHandler  StopCommandAbsentHandler
	stopCommandReconciler     StopCommandReconciler
	settlementReconciler      SettlementReconciler
	startReconcileAfter       time.Duration
	observer                  workerobs.Observer
	workerName                string
	workerInstanceKey         string
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

// Stop callbacks keep session policy in the charging domain while halops owns
// exact provider lookup and durable command evidence.
type StopCommandAbsentHandler func(context.Context, uuid.UUID) error
type StopCommandReconciler func(context.Context, uuid.UUID, Command) error
type SettlementReconciler func(context.Context, int) error

// ErrCommandNotFound is returned only when HAL's exact CMS command lookup
// responds with HTTP 404. It is authoritative absence, not a transport error.
var ErrCommandNotFound = errors.New("HAL command not found")

func New(database *gorm.DB, cfg config.HAL) *Service {
	return &Service{database: database, client: halclient.New(cfg), now: func() time.Time { return time.Now().UTC() }, startReconcileAfter: cfg.StartReconcileAfter}
}

func (service *Service) Available() bool {
	return service != nil && service.client != nil && service.client.Available()
}

// RequeuePlatformFact adapts HAL's receiver-recovery result to the platform
// operation port without letting platform code reach into the wire client.
func (service *Service) RequeuePlatformFact(ctx context.Context, factID uuid.UUID, correlationID string) error {
	if !service.Available() {
		return &platformops.HALFactRequeueError{Status: 503, Code: "hal_unavailable"}
	}
	err := service.client.RequeueFact(ctx, factID, correlationID)
	if err == nil {
		return nil
	}
	if isHALHTTPStatus(err, 404) {
		return &platformops.HALFactRequeueError{Status: 404, Code: "hal_fact_not_found"}
	}
	if isHALHTTPStatus(err, 409) {
		return &platformops.HALFactRequeueError{Status: 409, Code: "hal_fact_not_reconciliation_required"}
	}
	return &platformops.HALFactRequeueError{Status: 502, Code: "hal_fact_requeue_failed"}
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

func (service *Service) WithStopCommandAbsentHandler(handler StopCommandAbsentHandler) *Service {
	service.stopCommandAbsentHandler = handler
	return service
}
func (service *Service) WithStopCommandReconciler(reconciler StopCommandReconciler) *Service {
	service.stopCommandReconciler = reconciler
	return service
}
func (service *Service) WithSettlementReconciler(reconciler SettlementReconciler) *Service {
	service.settlementReconciler = reconciler
	return service
}

func (service *Service) WithWorkerObserver(observer workerobs.Observer, workerName, instanceKey string) *Service {
	service.observer = observer
	service.workerName = workerName
	service.workerInstanceKey = instanceKey
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
	if result.HALCommandID == uuid.Nil {
		return Command{}, halclient.ErrInvalidCommandResponse
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
		result, err := service.ReconcileCommand(ctx, command.CMSCommandID)
		if err == nil {
			if command.Kind == "STOP" && service.stopCommandReconciler != nil {
				if err := service.stopCommandReconciler(ctx, command.CMSCommandID, result); err != nil {
					return fmt.Errorf("reconcile stop command %s: %w", command.CMSCommandID, err)
				}
			}
			continue
		}
		if errors.Is(err, ErrCommandNotFound) {
			if command.Kind == "STOP" {
				if service.stopCommandAbsentHandler == nil {
					service.recordStopCommandAbsent(ctx, command.CMSCommandID)
					continue
				}
				if err := service.stopCommandAbsentHandler(ctx, command.CMSCommandID); err != nil {
					return fmt.Errorf("reconcile confirmed-absent stop command %s: %w", command.CMSCommandID, err)
				}
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
	if err := service.reconcileStrandedStarts(ctx, limit); err != nil {
		return err
	}
	if service.settlementReconciler != nil {
		return service.settlementReconciler(ctx, limit)
	}
	return nil
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
		service.recordWorkerHeartbeat(ctx)
		if err := service.ReconcilePending(ctx, 50); err != nil && ctx.Err() == nil {
			service.markWorkerUnhealthy(ctx)
			log.Printf("HAL reconciliation pass failed: %v", err)
		} else if ctx.Err() == nil {
			service.recordWorkerCompletion(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) recordWorkerHeartbeat(ctx context.Context) {
	if service.observer == nil {
		return
	}
	if err := service.observer.Heartbeat(ctx, service.workerName, service.workerInstanceKey); err != nil && ctx.Err() == nil {
		log.Printf("record HAL reconciler heartbeat: %v", err)
	}
}

func (service *Service) recordWorkerCompletion(ctx context.Context) {
	if service.observer == nil {
		return
	}
	if err := service.observer.JobCompleted(ctx, service.workerName, service.workerInstanceKey); err != nil && ctx.Err() == nil {
		log.Printf("record HAL reconciler completion: %v", err)
	}
}

func (service *Service) markWorkerUnhealthy(ctx context.Context) {
	if service.observer == nil {
		return
	}
	if err := service.observer.MarkUnhealthy(ctx, service.workerName, service.workerInstanceKey); err != nil && ctx.Err() == nil {
		log.Printf("mark HAL reconciler unhealthy: %v", err)
	}
}
