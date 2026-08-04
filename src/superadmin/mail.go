package superadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func mailJobView(job models.MailOutbox) MailJobView {
	return MailJobView{
		ID: job.ID, ToEmail: job.ToEmail, CPOID: job.CPOID, UserID: job.UserID,
		Template: job.Template, Status: job.Status, Attempts: job.Attempts,
		MaxAttempts: job.MaxAttempts, AvailableAt: job.AvailableAt, LockedAt: job.LockedAt,
		ErrorPresent: job.LastError != nil && strings.TrimSpace(*job.LastError) != "",
		SentAt:       job.SentAt, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
}

func (service *Service) ListMailJobs(ctx context.Context, principal auth.Principal, query MailQuery) (MailPage, error) {
	if err := requirePlatform(principal); err != nil {
		return MailPage{}, err
	}
	page, err := pageQuery(query.PageQuery)
	if err != nil {
		return MailPage{}, err
	}
	database := service.database.WithContext(ctx).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("created_at < ?", page.Before.UTC())
		} else {
			database = database.Where("(created_at < ?) OR (created_at = ? AND id < ?)", page.Before.UTC(), page.Before.UTC(), *page.BeforeID)
		}
	}
	if query.Status != "" {
		database = database.Where("status = ?", query.Status)
	}
	if query.Template != "" {
		if len(query.Template) > 50 {
			return MailPage{}, invalid("template", "Template is too long.")
		}
		database = database.Where("template = ?", strings.TrimSpace(query.Template))
	}
	if query.CPOID != nil {
		database = database.Where("cpo_id = ?", *query.CPOID)
	}
	if query.UserID != nil {
		database = database.Where("user_id = ?", *query.UserID)
	}
	var jobs []models.MailOutbox
	if err := database.Find(&jobs).Error; err != nil {
		return MailPage{}, fmt.Errorf("list mail jobs: %w", err)
	}
	hasMore := len(jobs) > page.Limit
	if hasMore {
		jobs = jobs[:page.Limit]
	}
	views := make([]MailJobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, mailJobView(job))
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(views) > 0 {
		next = &views[len(views)-1].CreatedAt
		nextID = &views[len(views)-1].ID
	}
	return MailPage{Jobs: views, NextBefore: next, NextBeforeID: nextID, HasMore: hasMore}, nil
}

func (service *Service) GetMailJob(ctx context.Context, principal auth.Principal, jobID uuid.UUID) (MailJobView, error) {
	if err := requirePlatform(principal); err != nil {
		return MailJobView{}, err
	}
	var job models.MailOutbox
	if err := service.database.WithContext(ctx).First(&job, "id = ?", jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MailJobView{}, notFound("mail_job_not_found", "The mail job was not found.")
		}
		return MailJobView{}, err
	}
	return mailJobView(job), nil
}

func (service *Service) RetryMailJob(ctx context.Context, principal auth.Principal, jobID uuid.UUID) (MailJobView, error) {
	if err := requirePlatform(principal); err != nil {
		return MailJobView{}, err
	}
	var view MailJobView
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job models.MailOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ?", jobID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("mail_job_not_found", "The mail job was not found.")
			}
			return err
		}
		if job.Status != constants.MailOutboxFailed && job.Status != constants.MailOutboxCanceled {
			return &auth.APIError{Status: 409, Code: "mail_job_not_retryable", Message: "Only failed or canceled mail jobs can be retried."}
		}
		now := service.now()
		if err := tx.Model(&models.MailOutbox{}).Where("id = ?", jobID).Updates(map[string]any{
			"status": constants.MailOutboxPending, "attempts": 0, "available_at": now,
			"locked_at": nil, "last_error": nil, "sent_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := service.audit(tx, principal.UserID, "MAIL_JOB_RETRIED", "MAIL_OUTBOX", &jobID, models.JSONB{}, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, "platform.mail.job_retried", "MAIL_OUTBOX", jobID, models.JSONB{}); err != nil {
			return err
		}
		job.Status = constants.MailOutboxPending
		job.Attempts = 0
		job.AvailableAt = now
		job.LockedAt = nil
		job.LastError = nil
		job.SentAt = nil
		job.UpdatedAt = now
		view = mailJobView(job)
		return nil
	})
	return view, err
}

func (service *Service) CancelMailJob(ctx context.Context, principal auth.Principal, jobID uuid.UUID, request ReasonRequest) (MailJobView, error) {
	if err := requirePlatform(principal); err != nil {
		return MailJobView{}, err
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return MailJobView{}, err
	}
	var view MailJobView
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job models.MailOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ?", jobID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("mail_job_not_found", "The mail job was not found.")
			}
			return err
		}
		if job.Status != constants.MailOutboxPending && job.Status != constants.MailOutboxFailed {
			return &auth.APIError{Status: 409, Code: "mail_job_not_cancelable", Message: "Only unsent mail jobs can be canceled."}
		}
		now := service.now()
		if err := tx.Model(&models.MailOutbox{}).Where("id = ?", jobID).Updates(map[string]any{"status": constants.MailOutboxCanceled, "locked_at": nil, "sent_at": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := service.audit(tx, principal.UserID, "MAIL_JOB_CANCELED", "MAIL_OUTBOX", &jobID, models.JSONB{"reason": reason}, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, "platform.mail.job_canceled", "MAIL_OUTBOX", jobID, models.JSONB{"reason": reason}); err != nil {
			return err
		}
		job.Status = constants.MailOutboxCanceled
		job.UpdatedAt = now
		job.LockedAt = nil
		job.SentAt = nil
		view = mailJobView(job)
		return nil
	})
	return view, err
}

func (service *Service) MailMetrics(ctx context.Context, principal auth.Principal) (MailMetricsResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return MailMetricsResponse{}, err
	}
	var rows []struct {
		Template string
		Status   constants.MailOutboxStatus
		Count    int64
	}
	if err := service.database.WithContext(ctx).Model(&models.MailOutbox{}).
		Select("template, status, count(*) AS count").Group("template, status").Order("template, status").Scan(&rows).Error; err != nil {
		return MailMetricsResponse{}, fmt.Errorf("aggregate mail metrics: %w", err)
	}
	metrics := make([]MailMetric, 0, len(rows))
	for _, row := range rows {
		metrics = append(metrics, MailMetric{Template: row.Template, Status: row.Status, Count: row.Count})
	}
	return MailMetricsResponse{Metrics: metrics}, nil
}

func (service *Service) ReconcileMailJobs(ctx context.Context, principal auth.Principal, request ReasonRequest) (MailReconcileResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return MailReconcileResponse{}, err
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return MailReconcileResponse{}, err
	}
	var response MailReconcileResponse
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		staleBefore := now.Add(-5 * time.Minute)
		result := tx.Model(&models.MailOutbox{}).
			Where("status = ? AND locked_at IS NOT NULL AND locked_at < ?", constants.MailOutboxProcessing, staleBefore).
			Updates(map[string]any{
				"status": constants.MailOutboxPending, "locked_at": nil,
				"available_at": now, "updated_at": now,
			})
		if result.Error != nil {
			return fmt.Errorf("reconcile stale mail jobs: %w", result.Error)
		}
		response.Requeued = result.RowsAffected
		data := models.JSONB{"reason": reason, "requeued": response.Requeued}
		if err := service.audit(tx, principal.UserID, "MAIL_JOBS_RECONCILED", "MAIL_OUTBOX", nil, data, now); err != nil {
			return err
		}
		return service.emitCollection(tx, principal.UserID, "platform.mail.jobs_reconciled", "MAIL_OUTBOX", data)
	})
	return response, err
}

func (service *Service) RetainMailJobs(ctx context.Context, principal auth.Principal, request MailRetentionRequest) (MailRetentionResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return MailRetentionResponse{}, err
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return MailRetentionResponse{}, err
	}
	if request.Before.IsZero() || request.Before.After(service.now().Add(-30*24*time.Hour)) {
		return MailRetentionResponse{}, invalid("before", "Retention cutoff must be at least 30 days old.")
	}
	var response MailRetentionResponse
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("status IN ? AND updated_at < ?", []constants.MailOutboxStatus{constants.MailOutboxSent, constants.MailOutboxCanceled}, request.Before.UTC()).Delete(&models.MailOutbox{})
		if result.Error != nil {
			return fmt.Errorf("retain mail jobs: %w", result.Error)
		}
		response.Deleted = result.RowsAffected
		now := service.now()
		data := models.JSONB{"reason": reason, "before": request.Before.UTC().Format(time.RFC3339), "deleted": response.Deleted}
		if err := service.audit(tx, principal.UserID, "MAIL_JOBS_RETAINED", "MAIL_OUTBOX", nil, data, now); err != nil {
			return err
		}
		return service.emitCollection(tx, principal.UserID, "platform.mail.jobs_retained", "MAIL_OUTBOX", data)
	})
	return response, err
}
