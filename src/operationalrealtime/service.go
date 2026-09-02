// Package operationalrealtime publishes durable, scoped notifications after
// CMS operational projections commit. It carries invalidation-sized payloads;
// REST snapshots remain authoritative recovery.
package operationalrealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/workerobs"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	database          *gorm.DB
	retention         time.Duration
	traceRetention    time.Duration
	poll              time.Duration
	heartbeat         time.Duration
	batchSize         int
	now               func() time.Time
	observer          workerobs.Observer
	workerName        string
	workerInstanceKey string
}

func (service *Service) WithWorkerObserver(observer workerobs.Observer, workerName, instanceKey string) *Service {
	service.observer = observer
	service.workerName = workerName
	service.workerInstanceKey = instanceKey
	return service
}

func New(database *gorm.DB, cfg config.Platform) *Service {
	traceRetention := cfg.TraceRetention
	if traceRetention <= 0 {
		// Keep package-level callers constructed by older tests/extensions
		// safe while Config.Validate enforces the configured production value.
		traceRetention = 30 * 24 * time.Hour
	}
	return &Service{database: database, retention: cfg.EventRetention, traceRetention: traceRetention, poll: cfg.RealtimePoll, heartbeat: cfg.RealtimeHeartbeat, batchSize: cfg.RealtimeBatchSize, now: func() time.Time { return time.Now().UTC() }}
}

// StreamTiming returns bounded polling settings shared by every scoped
// operational stream. The database event log, not a connection backlog, is
// the authoritative replay source.
func (service *Service) StreamTiming() (time.Duration, time.Duration, int) {
	poll, heartbeat, batchSize := service.poll, service.heartbeat, service.batchSize
	if poll <= 0 {
		poll = time.Second
	}
	if heartbeat <= poll {
		heartbeat = 15 * time.Second
	}
	if batchSize < 1 || batchSize > 500 {
		batchSize = 100
	}
	return poll, heartbeat, batchSize
}

type Input struct {
	CPOID                          uuid.UUID
	CustomerID                     *uuid.UUID
	Type, ResourceType, ResourceID string
	Data                           models.JSONB
}
type Page struct {
	Events     []models.OperationalEvent `json:"events"`
	NextCursor int64                     `json:"next_cursor"`
	HasMore    bool                      `json:"has_more"`
}

const (
	chargingSessionResourceType = "CHARGING_SESSION"
	chargerResourceType         = "CHARGER"
	connectorResourceType       = "CONNECTOR"
)

var chargingSessionEventTypes = []string{
	"charging.session_changed",
	"charging.meter_changed",
	"charging.telemetry_changed",
}

var (
	chargerLiveStateEventTypes   = []string{"charger.live_state_changed"}
	connectorLiveStateEventTypes = []string{"connector.live_state_changed"}
)

func (service *Service) Emit(tx *gorm.DB, input Input) (models.OperationalEvent, error) {
	if tx == nil || input.CPOID == uuid.Nil || input.Type == "" || input.ResourceType == "" || input.ResourceID == "" {
		return models.OperationalEvent{}, fmt.Errorf("operational event requires transaction, scope, type and resource")
	}
	now := service.now()
	data := input.Data
	if data == nil {
		data = models.JSONB{}
	}
	record := models.OperationalEvent{CPOID: input.CPOID, CustomerID: input.CustomerID, EventType: input.Type, ResourceType: input.ResourceType, ResourceID: input.ResourceID, Data: data, OccurredAt: now, ExpiresAt: now.Add(service.retention)}
	if err := tx.Create(&record).Error; err != nil {
		return models.OperationalEvent{}, fmt.Errorf("store operational event: %w", err)
	}
	return record, nil
}

func (service *Service) ListCPO(ctx context.Context, cpoID uuid.UUID, after int64, limit int) (Page, error) {
	return service.list(ctx, cpoID, nil, after, limit, "", nil)
}

// ListCPOChargingSessionEvents is the recoverable event feed for the live
// session projection. The event log remains the cursor authority; the live
// session REST snapshot remains the state authority.
func (service *Service) ListCPOChargingSessionEvents(ctx context.Context, cpoID uuid.UUID, after int64, limit int) (Page, error) {
	return service.list(ctx, cpoID, nil, after, limit, chargingSessionResourceType, chargingSessionEventTypes)
}

// LatestCPOChargingSessionEventID establishes the committed-event watermark
// before a caller reads a live-session snapshot. Events after the watermark
// wake a replacement projection; event data itself is never state.
func (service *Service) LatestCPOChargingSessionEventID(ctx context.Context, cpoID uuid.UUID) (int64, error) {
	if cpoID == uuid.Nil {
		return 0, fmt.Errorf("CPO ID is required")
	}
	var eventID int64
	err := service.database.WithContext(ctx).
		Model(&models.OperationalEvent{}).
		Where("cpo_id = ? AND expires_at > ? AND resource_type = ? AND event_type IN ?", cpoID, service.now(), chargingSessionResourceType, chargingSessionEventTypes).
		Select("COALESCE(MAX(id), 0)").
		Scan(&eventID).Error
	if err != nil {
		return 0, fmt.Errorf("load latest live charging-session event: %w", err)
	}
	return eventID, nil
}

// LatestCustomerLiveProjectionEventID establishes a pre-snapshot watermark for
// one customer's live-session projection. Customer session events are scoped
// directly to that customer; charger and connector observations are tenant
// shared because their committed live state can affect any owned live session.
func (service *Service) LatestCustomerLiveProjectionEventID(ctx context.Context, cpoID, customerID uuid.UUID) (int64, error) {
	if cpoID == uuid.Nil || customerID == uuid.Nil {
		return 0, fmt.Errorf("CPO and customer IDs are required")
	}
	var eventID int64
	err := service.database.WithContext(ctx).Model(&models.OperationalEvent{}).
		Where("cpo_id = ? AND expires_at > ?", cpoID, service.now()).
		Where(`((customer_id = ? AND resource_type = ? AND event_type IN ?) OR
			(customer_id IS NULL AND ((resource_type = ? AND event_type IN ?) OR (resource_type = ? AND event_type IN ?))))`,
			customerID, chargingSessionResourceType, chargingSessionEventTypes,
			chargerResourceType, chargerLiveStateEventTypes,
			connectorResourceType, connectorLiveStateEventTypes).
		Select("COALESCE(MAX(id), 0)").Scan(&eventID).Error
	if err != nil {
		return 0, fmt.Errorf("load latest customer live-projection event: %w", err)
	}
	return eventID, nil
}

// LatestCPOChargerAvailabilityEventID establishes a pre-snapshot watermark for
// one selected charger. It deliberately includes only committed availability
// observations; the customer-safe charger projection remains the state source.
func (service *Service) LatestCPOChargerAvailabilityEventID(ctx context.Context, cpoID uuid.UUID) (int64, error) {
	if cpoID == uuid.Nil {
		return 0, fmt.Errorf("CPO ID is required")
	}
	var eventID int64
	err := service.database.WithContext(ctx).Model(&models.OperationalEvent{}).
		Where("cpo_id = ? AND customer_id IS NULL AND expires_at > ?", cpoID, service.now()).
		Where("(resource_type = ? AND event_type IN ?) OR (resource_type = ? AND event_type IN ?)",
			chargerResourceType, chargerLiveStateEventTypes,
			connectorResourceType, connectorLiveStateEventTypes).
		Select("COALESCE(MAX(id), 0)").Scan(&eventID).Error
	if err != nil {
		return 0, fmt.Errorf("load latest charger-availability event: %w", err)
	}
	return eventID, nil
}

// ListCustomerLiveProjectionEvents returns only committed events that can
// affect one current live-session collection. A newly materialized session is
// always customer-scoped, so it remains visible even when its charger was not
// in the prior collection snapshot.
func (service *Service) ListCustomerLiveProjectionEvents(ctx context.Context, cpoID, customerID uuid.UUID, after int64, limit int, chargerIDs, connectorIDs []uuid.UUID) (Page, error) {
	if cpoID == uuid.Nil || customerID == uuid.Nil {
		return Page{}, fmt.Errorf("CPO and customer IDs are required")
	}
	return service.listCustomerProjectionEvents(ctx, cpoID, customerID, after, limit, chargerIDs, connectorIDs)
}

// ListCPOChargerAvailabilityEvents returns committed observations for one
// customer-visible charger and its connectors. It does not disclose the event
// payload to the browser; it merely wakes a replacement projection.
func (service *Service) ListCPOChargerAvailabilityEvents(ctx context.Context, cpoID, chargerID uuid.UUID, connectorIDs []uuid.UUID, after int64, limit int) (Page, error) {
	if cpoID == uuid.Nil || chargerID == uuid.Nil {
		return Page{}, fmt.Errorf("CPO and charger IDs are required")
	}
	if after < 0 {
		return Page{}, fmt.Errorf("event cursor must be nonnegative")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	query := service.database.WithContext(ctx).
		Where("cpo_id = ? AND customer_id IS NULL AND id > ? AND expires_at > ?", cpoID, after, service.now()).
		Where("resource_type = ? AND resource_id = ? AND event_type IN ?", chargerResourceType, chargerID.String(), chargerLiveStateEventTypes)
	if connectorIDTexts := uuidStrings(connectorIDs); len(connectorIDTexts) > 0 {
		query = service.database.WithContext(ctx).
			Where("cpo_id = ? AND customer_id IS NULL AND id > ? AND expires_at > ?", cpoID, after, service.now()).
			Where("(resource_type = ? AND resource_id = ? AND event_type IN ?) OR (resource_type = ? AND resource_id IN ? AND event_type IN ?)",
				chargerResourceType, chargerID.String(), chargerLiveStateEventTypes,
				connectorResourceType, connectorIDTexts, connectorLiveStateEventTypes)
	}
	return pageOperationalEvents(query, after, limit)
}

func (service *Service) ListCustomer(ctx context.Context, cpoID, customerID uuid.UUID, after int64, limit int) (Page, error) {
	return service.list(ctx, cpoID, &customerID, after, limit, "", nil)
}
func (service *Service) ListPlatform(ctx context.Context, cpoID *uuid.UUID, after int64, limit int) (Page, error) {
	if after < 0 {
		return Page{}, fmt.Errorf("event cursor must be nonnegative")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	records := make([]models.OperationalEvent, 0, limit+1)
	query := service.database.WithContext(ctx).Where("id > ? AND expires_at > ?", after, service.now())
	if cpoID != nil {
		query = query.Where("cpo_id = ?", *cpoID)
	}
	if err := query.Order("id ASC").Limit(limit + 1).Find(&records).Error; err != nil {
		return Page{}, fmt.Errorf("list platform operational events: %w", err)
	}
	page := Page{Events: records, NextCursor: after}
	if len(records) > limit {
		page.Events, page.HasMore = records[:limit], true
	}
	if len(page.Events) > 0 {
		page.NextCursor = page.Events[len(page.Events)-1].ID
	}
	return page, nil
}
func (service *Service) list(ctx context.Context, cpoID uuid.UUID, customerID *uuid.UUID, after int64, limit int, resourceType string, eventTypes []string) (Page, error) {
	if after < 0 {
		return Page{}, fmt.Errorf("event cursor must be nonnegative")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	now := service.now()
	query := service.database.WithContext(ctx).Where("cpo_id = ? AND id > ? AND expires_at > ?", cpoID, after, now)
	if customerID != nil {
		query = query.Where("customer_id IS NULL OR customer_id = ?", *customerID)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if len(eventTypes) > 0 {
		query = query.Where("event_type IN ?", eventTypes)
	}
	return pageOperationalEvents(query, after, limit)
}

func (service *Service) listCustomerProjectionEvents(ctx context.Context, cpoID, customerID uuid.UUID, after int64, limit int, chargerIDs, connectorIDs []uuid.UUID) (Page, error) {
	if after < 0 {
		return Page{}, fmt.Errorf("event cursor must be nonnegative")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	sharedClauses := ""
	arguments := []any{customerID, chargingSessionResourceType, chargingSessionEventTypes}
	if len(chargerIDs) > 0 {
		sharedClauses += " OR (customer_id IS NULL AND resource_type = ? AND resource_id IN ? AND event_type IN ?)"
		arguments = append(arguments, chargerResourceType, uuidStrings(chargerIDs), chargerLiveStateEventTypes)
	}
	if len(connectorIDs) > 0 {
		sharedClauses += " OR (customer_id IS NULL AND resource_type = ? AND resource_id IN ? AND event_type IN ?)"
		arguments = append(arguments, connectorResourceType, uuidStrings(connectorIDs), connectorLiveStateEventTypes)
	}
	query := service.database.WithContext(ctx).
		Where("cpo_id = ? AND id > ? AND expires_at > ?", cpoID, after, service.now()).
		Where("(customer_id = ? AND resource_type = ? AND event_type IN ?)"+sharedClauses, arguments...)
	return pageOperationalEvents(query, after, limit)
}

func pageOperationalEvents(query *gorm.DB, after int64, limit int) (Page, error) {
	records := make([]models.OperationalEvent, 0, limit+1)
	if err := query.Order("id ASC").Limit(limit + 1).Find(&records).Error; err != nil {
		return Page{}, fmt.Errorf("list operational events: %w", err)
	}
	page := Page{Events: records, NextCursor: after}
	if len(records) > limit {
		page.Events, page.HasMore = records[:limit], true
	}
	if len(page.Events) > 0 {
		page.NextCursor = page.Events[len(page.Events)-1].ID
	}
	return page, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	unique := make(map[uuid.UUID]struct{}, len(ids))
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		values = append(values, id.String())
	}
	return values
}
func (service *Service) DeleteExpired(ctx context.Context) error {
	now := service.now()
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now).Delete(&models.OperationalEvent{}).Error; err != nil {
			return err
		}
		// Trace rows are diagnostic evidence, never charging authority. A
		// bounded deletion keeps retention work proportional without touching
		// sessions, facts, commands, wallets, or connector state.
		if err := tx.Exec(`DELETE FROM charging_trace_events WHERE id IN (SELECT id FROM charging_trace_events WHERE recorded_at < ? ORDER BY recorded_at ASC LIMIT 500)`, now.Add(-service.traceRetention)).Error; err != nil {
			return err
		}
		// A retained-out root must not make an empty trace appear available. This
		// removes only bounded, eventless diagnostic roots and never touches any
		// authoritative charging or commercial relation.
		return tx.Exec(`DELETE FROM charging_traces WHERE trace_id IN (SELECT t.trace_id FROM charging_traces t WHERE t.updated_at < ? AND NOT EXISTS (SELECT 1 FROM charging_trace_events e WHERE e.trace_id = t.trace_id) ORDER BY t.updated_at ASC LIMIT 500)`, now.Add(-service.traceRetention)).Error
	})
}

// RunRetention owns expiration cleanup for the durable replay log. Failed
// cleanup is observable and does not prevent later recovery attempts.
func (service *Service) RunRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		service.recordWorkerHeartbeat(ctx)
		if err := service.DeleteExpired(ctx); err != nil && ctx.Err() == nil {
			service.markWorkerUnhealthy(ctx)
			log.Printf("operational event retention cleanup failed: %v", err)
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
		log.Printf("record operational retention heartbeat: %v", err)
	}
}

func (service *Service) recordWorkerCompletion(ctx context.Context) {
	if service.observer == nil {
		return
	}
	if err := service.observer.JobCompleted(ctx, service.workerName, service.workerInstanceKey); err != nil && ctx.Err() == nil {
		log.Printf("record operational retention completion: %v", err)
	}
}

func (service *Service) markWorkerUnhealthy(ctx context.Context) {
	if service.observer == nil {
		return
	}
	if err := service.observer.MarkUnhealthy(ctx, service.workerName, service.workerInstanceKey); err != nil && ctx.Err() == nil {
		log.Printf("mark operational retention unhealthy: %v", err)
	}
}

// ParseCursor accepts the REST/SSE cursor convention. Last-Event-ID is
// resolved by callers before this function so query handling stays transport
// neutral.
func ParseCursor(afterText, limitText string) (int64, int, error) {
	afterText = strings.TrimSpace(afterText)
	limitText = strings.TrimSpace(limitText)
	after := int64(0)
	if afterText != "" {
		value, err := strconv.ParseInt(afterText, 10, 64)
		if err != nil || value < 0 {
			return 0, 0, fmt.Errorf("after_id must be a nonnegative integer")
		}
		after = value
	}
	limit := 100
	if limitText != "" {
		value, err := strconv.Atoi(limitText)
		if err != nil || value < 1 || value > 500 {
			return 0, 0, fmt.Errorf("limit must be between 1 and 500")
		}
		limit = value
	}
	return after, limit, nil
}

func WriteSSE(writer io.Writer, events []models.OperationalEvent) error {
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, payload); err != nil {
			return err
		}
	}
	return nil
}
