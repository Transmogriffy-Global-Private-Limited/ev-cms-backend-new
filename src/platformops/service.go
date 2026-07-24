package platformops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
	maxEventType    = 150
	maxResourceType = 100
	maxResourceID   = 255
	maxWorkerName   = 100
	maxInstanceKey  = 255
)

type Service struct {
	database *gorm.DB
	config   config.Platform
	now      func() time.Time
}

func NewService(database *gorm.DB, cfg config.Platform) *Service {
	return &Service{
		database: database,
		config:   cfg,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (service *Service) Emit(tx *gorm.DB, input EventInput) (models.PlatformEvent, error) {
	if tx == nil {
		return models.PlatformEvent{}, errors.New("platform event transaction is required")
	}
	input.Type = strings.TrimSpace(input.Type)
	input.ResourceType = strings.TrimSpace(input.ResourceType)
	if input.Type == "" || input.ResourceType == "" {
		return models.PlatformEvent{}, errors.New("platform event type and resource type are required")
	}
	if len(input.Type) > maxEventType ||
		len(input.ResourceType) > maxResourceType ||
		(input.ResourceID != nil && len(*input.ResourceID) > maxResourceID) {
		return models.PlatformEvent{}, errors.New("platform event field exceeds storage limit")
	}
	now := service.now()
	record := models.PlatformEvent{
		EventType:    input.Type,
		ActorUserID:  input.ActorUserID,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		Data:         input.Data,
		OccurredAt:   now,
		ExpiresAt:    now.Add(service.config.EventRetention),
	}
	if record.Data == nil {
		record.Data = models.JSONB{}
	}
	if err := tx.Create(&record).Error; err != nil {
		return models.PlatformEvent{}, fmt.Errorf("store platform event: %w", err)
	}
	return record, nil
}

func (service *Service) ListEvents(
	ctx context.Context,
	principal auth.Principal,
	query EventQuery,
) (EventPage, error) {
	if err := requirePlatform(principal); err != nil {
		return EventPage{}, err
	}
	query.Limit = boundedLimit(query.Limit)
	query.Type = strings.TrimSpace(query.Type)
	if len(query.Type) > maxEventType {
		return EventPage{}, invalid("type", "type must not exceed 150 characters.")
	}
	now := service.now()
	if err := service.validateEventCursor(ctx, query.AfterID, now); err != nil {
		return EventPage{}, err
	}

	requested := query.Limit + 1
	records := make([]models.PlatformEvent, 0, requested)
	database := service.database.WithContext(ctx).
		Where("id > ? AND expires_at > ?", query.AfterID, now).
		Order("id ASC").
		Limit(requested)
	if query.Type != "" {
		database = database.Where("event_type = ?", query.Type)
	}
	if err := database.Find(&records).Error; err != nil {
		return EventPage{}, fmt.Errorf("list platform events: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	next := query.AfterID
	if len(records) > 0 {
		next = records[len(records)-1].ID
	}
	return EventPage{Events: records, NextCursor: next, HasMore: hasMore}, nil
}

func (service *Service) validateEventCursor(
	ctx context.Context,
	afterID int64,
	now time.Time,
) error {
	if afterID < 0 {
		return invalid("after_id", "after_id must be zero or a positive event ID.")
	}
	if afterID == 0 {
		return nil
	}
	var minimum sql.NullInt64
	if err := service.database.WithContext(ctx).
		Model(&models.PlatformEvent{}).
		Where("expires_at > ?", now).
		Select("MIN(id)").
		Scan(&minimum).Error; err != nil {
		return fmt.Errorf("inspect platform event retention: %w", err)
	}
	if minimum.Valid && afterID < minimum.Int64-1 {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "realtime_cursor_expired",
			Message: "The event cursor is no longer available. Refresh platform state.",
		}
	}
	return nil
}

func (service *Service) ListAudit(
	ctx context.Context,
	principal auth.Principal,
	query AuditQuery,
) (AuditPage, error) {
	if err := requirePlatform(principal); err != nil {
		return AuditPage{}, err
	}
	query.Limit = boundedLimit(query.Limit)
	requested := query.Limit + 1
	records := make([]models.AuditLog, 0, requested)
	database := service.database.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(requested)
	if query.Before != nil {
		if query.BeforeID == nil {
			database = database.Where("created_at < ?", query.Before.UTC())
		} else {
			database = database.Where(
				"(created_at < ?) OR (created_at = ? AND id < ?)",
				query.Before.UTC(),
				query.Before.UTC(),
				*query.BeforeID,
			)
		}
	} else if query.BeforeID != nil {
		return AuditPage{}, invalid(
			"before_id",
			"before_id may be supplied only with before.",
		)
	}
	if action := strings.TrimSpace(query.Action); action != "" {
		if len(action) > maxResourceType {
			return AuditPage{}, invalid(
				"action",
				"action must not exceed 100 characters.",
			)
		}
		database = database.Where("action = ?", action)
	}
	if entity := strings.TrimSpace(query.Entity); entity != "" {
		if len(entity) > maxResourceType {
			return AuditPage{}, invalid(
				"entity",
				"entity must not exceed 100 characters.",
			)
		}
		database = database.Where("entity = ?", entity)
	}
	if query.ActorUserID != nil {
		database = database.Where("user_id = ?", *query.ActorUserID)
	}
	if query.CPOID != nil {
		database = database.Where("cpo_id = ?", *query.CPOID)
	}
	if err := database.Find(&records).Error; err != nil {
		return AuditPage{}, fmt.Errorf("list platform audit records: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(records) > 0 {
		value := records[len(records)-1].CreatedAt
		id := records[len(records)-1].ID
		next = &value
		nextID = &id
	}
	return AuditPage{
		Records:      records,
		NextBefore:   next,
		NextBeforeID: nextID,
		HasMore:      hasMore,
	}, nil
}

func (service *Service) Heartbeat(
	ctx context.Context,
	workerName string,
	instanceKey string,
) error {
	workerName = strings.TrimSpace(workerName)
	instanceKey = strings.TrimSpace(instanceKey)
	if workerName == "" || instanceKey == "" {
		return errors.New("worker name and instance key are required")
	}
	if len(workerName) > maxWorkerName || len(instanceKey) > maxInstanceKey {
		return errors.New("worker identity exceeds storage limit")
	}
	now := service.now()
	record := models.WorkerInstance{
		ID:              uuid.New(),
		WorkerName:      workerName,
		InstanceKey:     instanceKey,
		Required:        true,
		ReportedStatus:  "HEALTHY",
		StartedAt:       now,
		LastHeartbeatAt: now,
		Metadata:        models.JSONB{},
		UpdatedAt:       now,
	}
	return service.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "worker_name"}, {Name: "instance_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reported_status":   "HEALTHY",
			"last_heartbeat_at": now,
			"updated_at":        now,
		}),
	}).Create(&record).Error
}

func (service *Service) JobCompleted(
	ctx context.Context,
	workerName string,
	instanceKey string,
) error {
	now := service.now()
	return service.database.WithContext(ctx).
		Model(&models.WorkerInstance{}).
		Where("worker_name = ? AND instance_key = ?", workerName, instanceKey).
		Updates(map[string]any{
			"last_job_completed_at": now,
			"last_heartbeat_at":     now,
			"reported_status":       "HEALTHY",
			"updated_at":            now,
		}).Error
}

func (service *Service) ListWorkers(
	ctx context.Context,
	principal auth.Principal,
) (WorkerListResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return WorkerListResponse{}, err
	}
	var records []models.WorkerInstance
	if err := service.database.WithContext(ctx).
		Order("worker_name, instance_key").
		Find(&records).Error; err != nil {
		return WorkerListResponse{}, fmt.Errorf("list worker instances: %w", err)
	}
	now := service.now()
	views := make([]WorkerView, 0, len(records))
	for _, record := range records {
		status := record.ReportedStatus
		if status != "DISABLED" &&
			now.Sub(record.LastHeartbeatAt) > service.config.WorkerStaleAfter {
			status = "STALE"
		}
		views = append(views, WorkerView{
			ID:                 record.ID,
			Name:               record.WorkerName,
			InstanceKey:        record.InstanceKey,
			Status:             status,
			Required:           record.Required,
			StartedAt:          record.StartedAt,
			LastHeartbeatAt:    record.LastHeartbeatAt,
			LastJobCompletedAt: record.LastJobCompletedAt,
			Metadata:           record.Metadata,
		})
	}
	return WorkerListResponse{Workers: views}, nil
}

func (service *Service) RequiredWorkersReady(ctx context.Context) (bool, error) {
	var unhealthy int64
	staleBefore := service.now().Add(-service.config.WorkerStaleAfter)
	if err := service.database.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		  FROM (
		        SELECT worker_name
		          FROM worker_instances
		         WHERE required = TRUE
		         GROUP BY worker_name
		        HAVING NOT BOOL_OR(
		            reported_status = 'HEALTHY'
		            AND last_heartbeat_at >= ?
		        )
		       ) AS unhealthy_workers
	`, staleBefore).Scan(&unhealthy).Error; err != nil {
		return false, fmt.Errorf("inspect required worker health: %w", err)
	}
	return unhealthy == 0, nil
}

func (service *Service) DeleteExpiredEvents(ctx context.Context) error {
	return service.database.WithContext(ctx).
		Where("expires_at <= ?", service.now()).
		Delete(&models.PlatformEvent{}).Error
}

func (service *Service) RunMaintenance(ctx context.Context, instanceKey string) {
	const workerName = "platform-maintenance"
	ticker := time.NewTicker(service.config.MaintenanceEvery)
	defer ticker.Stop()

	for {
		if err := service.Heartbeat(ctx, workerName, instanceKey); err != nil &&
			ctx.Err() == nil {
			log.Printf("record platform maintenance heartbeat: %v", err)
		} else if err := service.DeleteExpiredEvents(ctx); err != nil {
			if ctx.Err() == nil {
				log.Printf("delete expired platform events: %v", err)
			}
		} else {
			if err := service.JobCompleted(ctx, workerName, instanceKey); err != nil &&
				ctx.Err() == nil {
				log.Printf("record platform maintenance completion: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func boundedLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultPageSize
	case limit > maxPageSize:
		return maxPageSize
	default:
		return limit
	}
}

func requirePlatform(principal auth.Principal) error {
	if principal.Scope != "PLATFORM" {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "permission_denied",
			Message: "Platform superadmin access is required.",
		}
	}
	return nil
}

func invalid(field, message string) error {
	return &auth.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_" + field,
		Message: message,
	}
}
