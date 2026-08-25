package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	planCodePattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
)

var currentStatuses = []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}

// Service manages manual commercial grants. It deliberately has no payment
// provider, invoice service, webhook receiver, or lifecycle scheduler.
type Service struct {
	database *gorm.DB
	events   *platformops.Service
	now      func() time.Time
}

func NewService(database *gorm.DB, events *platformops.Service) *Service {
	return &Service{database: database, events: events, now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) CreatePlan(ctx context.Context, principal auth.Principal, request CreatePlanRequest) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Terms = normalizeTerms(request.Terms)
	if err := validatePlan(request.Code, request.Name, request.Description, request.Terms); err != nil {
		return PlanView{}, err
	}
	now := service.now()
	plan := models.SubscriptionPlan{ID: uuid.New(), Code: request.Code, Name: request.Name, Description: request.Description, Status: "DRAFT", CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now}
	version := versionFromTerms(plan.ID, 1, request.Terms, now)
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan).Error; err != nil {
			return mapWriteError(err, "plan_conflict")
		}
		if err := tx.Create(&version).Error; err != nil {
			return mapWriteError(err, "plan_conflict")
		}
		if err := writeAudit(tx, principal.UserID, "SUBSCRIPTION_PLAN_CREATED", "SUBSCRIPTION_PLAN", plan.ID, models.JSONB{"code": plan.Code}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.plan_created", "SUBSCRIPTION_PLAN", plan.ID.String(), models.JSONB{"code": plan.Code})
	})
	if err != nil {
		return PlanView{}, err
	}
	return service.GetPlan(ctx, principal, plan.ID)
}

func (service *Service) ListPlans(ctx context.Context, principal auth.Principal) ([]PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return nil, err
	}
	var plans []models.SubscriptionPlan
	if err := service.database.WithContext(ctx).Order("created_at DESC").Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("list subscription plans: %w", err)
	}
	result := make([]PlanView, 0, len(plans))
	for _, plan := range plans {
		view, err := service.loadPlan(ctx, plan)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (service *Service) GetPlan(ctx context.Context, principal auth.Principal, planID uuid.UUID) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	var plan models.SubscriptionPlan
	if err := service.database.WithContext(ctx).First(&plan, "id = ?", planID).Error; err != nil {
		return PlanView{}, notFound("subscription_plan_not_found", "Subscription plan was not found.", err)
	}
	return service.loadPlan(ctx, plan)
}

func (service *Service) loadPlan(ctx context.Context, plan models.SubscriptionPlan) (PlanView, error) {
	var versions []models.SubscriptionPlanVersion
	if err := service.database.WithContext(ctx).Where("plan_id = ?", plan.ID).Order("version DESC").Find(&versions).Error; err != nil {
		return PlanView{}, fmt.Errorf("load subscription plan versions: %w", err)
	}
	result := PlanView{Plan: plan, Published: []models.SubscriptionPlanVersion{}}
	for _, version := range versions {
		if version.Status == "DRAFT" {
			draft := version
			result.Draft = &draft
		} else {
			result.Published = append(result.Published, version)
		}
	}
	return result, nil
}

func (service *Service) UpdateDraft(ctx context.Context, principal auth.Principal, planID uuid.UUID, request UpdateDraftRequest) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	request.Name, request.Description, request.Terms = strings.TrimSpace(request.Name), strings.TrimSpace(request.Description), normalizeTerms(request.Terms)
	if err := validatePlan("valid", request.Name, request.Description, request.Terms); err != nil {
		return PlanView{}, err
	}
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan models.SubscriptionPlan
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", planID).Error; err != nil {
			return notFound("subscription_plan_not_found", "Subscription plan was not found.", err)
		}
		if plan.Status == "ARCHIVED" {
			return conflict("subscription_plan_archived", "Archived plans cannot be edited.")
		}
		var draft models.SubscriptionPlanVersion
		result := tx.Where("plan_id = ? AND status = 'DRAFT'", planID).First(&draft)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			var maximum int
			if err := tx.Model(&models.SubscriptionPlanVersion{}).Where("plan_id = ?", planID).Select("COALESCE(MAX(version), 0)").Scan(&maximum).Error; err != nil {
				return err
			}
			draft = versionFromTerms(planID, maximum+1, request.Terms, now)
			if err := tx.Create(&draft).Error; err != nil {
				return mapWriteError(err, "plan_conflict")
			}
		} else if result.Error != nil {
			return result.Error
		} else {
			updated := versionFromTerms(planID, draft.Version, request.Terms, now)
			if err := tx.Model(&draft).Updates(map[string]any{"currency": updated.Currency, "price_minor": updated.PriceMinor, "billing_interval": updated.BillingInterval, "interval_count": updated.IntervalCount, "trial_days": updated.TrialDays, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&plan).Updates(map[string]any{"name": request.Name, "description": request.Description, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "SUBSCRIPTION_PLAN_DRAFT_UPDATED", "SUBSCRIPTION_PLAN", planID, models.JSONB{"version": draft.Version}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.plan_draft_updated", "SUBSCRIPTION_PLAN", planID.String(), models.JSONB{"version": draft.Version})
	})
	if err != nil {
		return PlanView{}, err
	}
	return service.GetPlan(ctx, principal, planID)
}

func (service *Service) PublishPlan(ctx context.Context, principal auth.Principal, planID uuid.UUID) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan models.SubscriptionPlan
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", planID).Error; err != nil {
			return notFound("subscription_plan_not_found", "Subscription plan was not found.", err)
		}
		if plan.Status == "ARCHIVED" {
			return conflict("subscription_plan_archived", "Archived plans cannot be published.")
		}
		var draft models.SubscriptionPlanVersion
		if err := tx.Where("plan_id = ? AND status = 'DRAFT'", planID).First(&draft).Error; err != nil {
			return notFound("subscription_plan_draft_not_found", "No draft version is available to publish.", err)
		}
		if err := tx.Model(&draft).Updates(map[string]any{"status": "PUBLISHED", "published_at": now, "published_by": principal.UserID, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("publish subscription plan: %w", err)
		}
		if err := tx.Model(&plan).Updates(map[string]any{"status": "PUBLISHED", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "SUBSCRIPTION_PLAN_PUBLISHED", "SUBSCRIPTION_PLAN", planID, models.JSONB{"version": draft.Version, "version_id": draft.ID}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.plan_published", "SUBSCRIPTION_PLAN", planID.String(), models.JSONB{"version": draft.Version, "version_id": draft.ID})
	})
	if err != nil {
		return PlanView{}, err
	}
	return service.GetPlan(ctx, principal, planID)
}

func (service *Service) ArchivePlan(ctx context.Context, principal auth.Principal, planID uuid.UUID) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plan models.SubscriptionPlan
		if err := tx.First(&plan, "id = ?", planID).Error; err != nil {
			return notFound("subscription_plan_not_found", "Subscription plan was not found.", err)
		}
		if plan.Status == "ARCHIVED" {
			return nil
		}
		if err := tx.Model(&plan).Updates(map[string]any{"status": "ARCHIVED", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "SUBSCRIPTION_PLAN_ARCHIVED", "SUBSCRIPTION_PLAN", planID, models.JSONB{}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.plan_archived", "SUBSCRIPTION_PLAN", planID.String(), models.JSONB{})
	})
	if err != nil {
		return PlanView{}, err
	}
	return service.GetPlan(ctx, principal, planID)
}

func (service *Service) Issue(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request IssueRequest) (SubscriptionView, error) {
	if err := requirePlatform(principal); err != nil {
		return SubscriptionView{}, err
	}
	request.Reason, request.IdempotencyKey = strings.TrimSpace(request.Reason), strings.TrimSpace(request.IdempotencyKey)
	if request.PlanVersionID == uuid.Nil || !validActionFields(request.Reason, request.IdempotencyKey) {
		return SubscriptionView{}, invalid("request", "plan_version_id, reason, and idempotency_key are required and must be valid.")
	}
	now, start := service.now(), service.now()
	if request.StartsAt != nil {
		start = request.StartsAt.UTC()
	}
	if start.After(now) {
		return SubscriptionView{}, invalid("starts_at", "starts_at cannot be in the future.")
	}
	var subscription models.CPOSubscription
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, replayed, err := service.idempotentSubscription(tx, principal.UserID, request.IdempotencyKey, "ISSUE", cpoID); err != nil {
			return err
		} else if replayed {
			subscription = existing
			return nil
		}
		if err := requireCPO(tx, cpoID); err != nil {
			return err
		}
		version, err := publishedVersion(tx, request.PlanVersionID)
		if err != nil {
			return err
		}
		status := "ACTIVE"
		var trialEnd *time.Time
		if version.TrialDays > 0 {
			status = "TRIAL"
			end := start.AddDate(0, 0, version.TrialDays)
			trialEnd = &end
		}
		subscription = models.CPOSubscription{ID: uuid.New(), CPOID: cpoID, PlanVersionID: version.ID, Status: status, StartsAt: start, TrialEndsAt: trialEnd, CurrentPeriodStartsAt: start, CurrentPeriodEndsAt: addPeriod(start, version), CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&subscription).Error; err != nil {
			return mapWriteError(err, "subscription_conflict")
		}
		if err := service.recordTransition(tx, principal.UserID, subscription, nil, nil, request.Reason, request.IdempotencyKey, now, "ISSUE"); err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "CPO_SUBSCRIPTION_ISSUED", "CPO_SUBSCRIPTION", subscription.ID, models.JSONB{"cpo_id": cpoID, "status": status, "plan_version_id": version.ID}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.issued", "CPO_SUBSCRIPTION", subscription.ID.String(), models.JSONB{"cpo_id": cpoID, "status": status, "plan_version_id": version.ID})
	})
	if err != nil {
		return SubscriptionView{}, err
	}
	return service.subscriptionView(ctx, subscription)
}

func (service *Service) GetCurrent(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) (SubscriptionView, error) {
	if err := requirePlatform(principal); err != nil {
		return SubscriptionView{}, err
	}
	var subscription models.CPOSubscription
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND status IN ?", cpoID, currentStatuses).First(&subscription).Error; err != nil {
		return SubscriptionView{}, notFound("subscription_not_found", "CPO has no current subscription.", err)
	}
	return service.subscriptionView(ctx, subscription)
}

func (service *Service) Renew(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request RenewRequest) (SubscriptionView, error) {
	start := service.now()
	if request.StartsAt != nil {
		start = request.StartsAt.UTC()
	}
	if start.After(service.now()) {
		return SubscriptionView{}, invalid("starts_at", "starts_at cannot be in the future.")
	}
	return service.mutateForStatuses(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "RENEW", append(append([]string{}, currentStatuses...), "EXPIRED"), func(record *models.CPOSubscription, tx *gorm.DB, now time.Time) (string, error) {
		var version models.SubscriptionPlanVersion
		if err := tx.First(&version, "id = ?", record.PlanVersionID).Error; err != nil {
			return "", err
		}
		renewSubscriptionPeriod(record, start, now, version)
		return "platform.subscription.renewed", nil
	})
}

func (service *Service) ChangePlan(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request ChangePlanRequest) (SubscriptionView, error) {
	if request.PlanVersionID == uuid.Nil {
		return SubscriptionView{}, invalid("plan_version_id", "plan_version_id is required.")
	}
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "CHANGE_PLAN", func(record *models.CPOSubscription, tx *gorm.DB, now time.Time) (string, error) {
		version, err := publishedVersion(tx, request.PlanVersionID)
		if err != nil {
			return "", err
		}
		record.PlanVersionID = version.ID
		record.CurrentPeriodStartsAt, record.CurrentPeriodEndsAt = now, addPeriod(now, version)
		record.PendingPlanVersionID, record.PendingChangeAt, record.PendingChangeBy = nil, nil, nil
		return "platform.subscription.plan_changed", nil
	})
}

func (service *Service) Activate(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "ACTIVATE", func(record *models.CPOSubscription, _ *gorm.DB, _ time.Time) (string, error) {
		if record.Status != "TRIAL" {
			return "", conflict("invalid_subscription_transition", "Only a trial subscription can be activated.")
		}
		record.Status, record.TrialEndsAt = "ACTIVE", nil
		return "platform.subscription.activated", nil
	})
}

func (service *Service) Pause(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "PAUSE", func(record *models.CPOSubscription, _ *gorm.DB, _ time.Time) (string, error) {
		if record.Status == "PAUSED" {
			return "platform.subscription.paused", nil
		}
		if record.Status != "TRIAL" && record.Status != "ACTIVE" && record.Status != "PAST_DUE" {
			return "", conflict("invalid_subscription_transition", "This subscription cannot be paused from its current state.")
		}
		record.Status = "PAUSED"
		return "platform.subscription.paused", nil
	})
}

func (service *Service) Resume(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "RESUME", func(record *models.CPOSubscription, _ *gorm.DB, _ time.Time) (string, error) {
		if record.Status == "ACTIVE" {
			return "platform.subscription.resumed", nil
		}
		if record.Status != "PAUSED" && record.Status != "PAST_DUE" {
			return "", conflict("invalid_subscription_transition", "Only paused or past-due subscriptions can be resumed.")
		}
		record.Status = "ACTIVE"
		return "platform.subscription.resumed", nil
	})
}

func (service *Service) MarkPastDue(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "MARK_PAST_DUE", func(record *models.CPOSubscription, _ *gorm.DB, _ time.Time) (string, error) {
		if record.Status == "PAST_DUE" {
			return "platform.subscription.past_due", nil
		}
		if record.Status != "ACTIVE" && record.Status != "TRIAL" && record.Status != "PAUSED" {
			return "", conflict("invalid_subscription_transition", "This subscription cannot be marked past due.")
		}
		record.Status = "PAST_DUE"
		return "platform.subscription.past_due", nil
	})
}

func (service *Service) Expire(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "EXPIRE", func(record *models.CPOSubscription, _ *gorm.DB, now time.Time) (string, error) {
		record.Status, record.EndedAt = "EXPIRED", &now
		record.CancelAtPeriodEnd, record.CancellationScheduledBy = false, nil
		record.PendingPlanVersionID, record.PendingChangeAt, record.PendingChangeBy = nil, nil, nil
		return "platform.subscription.expired", nil
	})
}

func (service *Service) Cancel(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "CANCEL", func(record *models.CPOSubscription, _ *gorm.DB, now time.Time) (string, error) {
		record.Status, record.CancelledAt, record.EndedAt = "CANCELLED", &now, &now
		record.CancelAtPeriodEnd, record.CancellationScheduledBy = false, nil
		record.PendingPlanVersionID, record.PendingChangeAt, record.PendingChangeBy = nil, nil, nil
		return "platform.subscription.cancelled", nil
	})
}

type mutation func(*models.CPOSubscription, *gorm.DB, time.Time) (string, error)

func (service *Service) mutate(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, reason, key, operation string, change mutation) (SubscriptionView, error) {
	return service.mutateForStatuses(ctx, principal, cpoID, reason, key, operation, currentStatuses, change)
}

func (service *Service) mutateForStatuses(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, reason, key, operation string, statuses []string, change mutation) (SubscriptionView, error) {
	if err := requirePlatform(principal); err != nil {
		return SubscriptionView{}, err
	}
	reason, key = strings.TrimSpace(reason), strings.TrimSpace(key)
	if !validActionFields(reason, key) {
		return SubscriptionView{}, invalid("request", "reason and idempotency_key are required and must be within limits.")
	}
	now := service.now()
	var record models.CPOSubscription
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, replayed, err := service.idempotentSubscription(tx, principal.UserID, key, operation, cpoID); err != nil {
			return err
		} else if replayed {
			record = existing
			return nil
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("cpo_id = ? AND status IN ?", cpoID, statuses).Order("current_period_ends_at DESC, updated_at DESC").First(&record).Error; err != nil {
			return notFound("subscription_not_found", "CPO has no current subscription.", err)
		}
		previousStatus, previousVersion := record.Status, record.PlanVersionID
		eventType, err := change(&record, tx, now)
		if err != nil {
			return err
		}
		record.UpdatedAt = now
		if err := tx.Save(&record).Error; err != nil {
			return mapWriteError(err, "subscription_conflict")
		}
		if err := service.recordTransition(tx, principal.UserID, record, &previousStatus, &previousVersion, reason, key, now, operation); err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "CPO_SUBSCRIPTION_"+operation, "CPO_SUBSCRIPTION", record.ID, models.JSONB{"cpo_id": cpoID, "previous_status": previousStatus, "status": record.Status, "plan_version_id": record.PlanVersionID}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, eventType, "CPO_SUBSCRIPTION", record.ID.String(), models.JSONB{"cpo_id": cpoID, "status": record.Status, "plan_version_id": record.PlanVersionID})
	})
	if err != nil {
		return SubscriptionView{}, err
	}
	return service.subscriptionView(ctx, record)
}

func (service *Service) subscriptionView(ctx context.Context, record models.CPOSubscription) (SubscriptionView, error) {
	var version models.SubscriptionPlanVersion
	if err := service.database.WithContext(ctx).First(&version, "id = ?", record.PlanVersionID).Error; err != nil {
		return SubscriptionView{}, err
	}
	var plan models.SubscriptionPlan
	if err := service.database.WithContext(ctx).First(&plan, "id = ?", version.PlanID).Error; err != nil {
		return SubscriptionView{}, err
	}
	return SubscriptionView{Subscription: record, Plan: plan, Version: version}, nil
}

func (service *Service) History(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) ([]models.CPOSubscriptionHistory, error) {
	if err := requirePlatform(principal); err != nil {
		return nil, err
	}
	var records []models.CPOSubscriptionHistory
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).Order("created_at DESC, id DESC").Limit(500).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func normalizeTerms(input PlanTermsInput) PlanTermsInput {
	input.Currency, input.BillingInterval = strings.ToUpper(strings.TrimSpace(input.Currency)), strings.ToUpper(strings.TrimSpace(input.BillingInterval))
	if input.IntervalCount == 0 {
		input.IntervalCount = 1
	}
	return input
}

func validatePlan(code, name, description string, terms PlanTermsInput) error {
	if !planCodePattern.MatchString(code) || len(code) > 80 {
		return invalid("code", "code must be a lowercase underscore-separated identifier of at most 80 characters.")
	}
	if name == "" || len(name) > 150 {
		return invalid("name", "name is required and must not exceed 150 characters.")
	}
	if len(description) > 2000 {
		return invalid("description", "description must not exceed 2000 characters.")
	}
	if len(terms.Currency) != 3 {
		return invalid("currency", "currency must be a three-letter uppercase code.")
	}
	if terms.PriceMinor < 0 {
		return invalid("price_minor", "price_minor must not be negative.")
	}
	if terms.BillingInterval != "MONTHLY" && terms.BillingInterval != "YEARLY" {
		return invalid("billing_interval", "billing_interval must be MONTHLY or YEARLY.")
	}
	if terms.IntervalCount < 1 || terms.IntervalCount > 120 || terms.TrialDays < 0 || terms.TrialDays > 365 {
		return invalid("terms", "interval_count or trial_days is outside its supported range.")
	}
	return nil
}

func versionFromTerms(planID uuid.UUID, version int, terms PlanTermsInput, now time.Time) models.SubscriptionPlanVersion {
	return models.SubscriptionPlanVersion{ID: uuid.New(), PlanID: planID, Version: version, Status: "DRAFT", Currency: terms.Currency, PriceMinor: terms.PriceMinor, BillingInterval: terms.BillingInterval, IntervalCount: terms.IntervalCount, TrialDays: terms.TrialDays, CreatedAt: now, UpdatedAt: now}
}
func addPeriod(start time.Time, version models.SubscriptionPlanVersion) time.Time {
	if version.BillingInterval == "YEARLY" {
		return start.AddDate(version.IntervalCount, 0, 0)
	}
	return start.AddDate(0, version.IntervalCount, 0)
}

func renewSubscriptionPeriod(record *models.CPOSubscription, requestedStart, now time.Time, version models.SubscriptionPlanVersion) {
	effectiveStart := requestedStart
	if record.Status == "EXPIRED" {
		// An expired period cannot be backdated into another elapsed period.
		// Renewal is the audited manual reactivation after repayment.
		if effectiveStart.Before(now) {
			effectiveStart = now
		}
		record.Status, record.EndedAt, record.TrialEndsAt = "ACTIVE", nil, nil
	}
	record.CurrentPeriodStartsAt, record.CurrentPeriodEndsAt = effectiveStart, addPeriod(effectiveStart, version)
	if record.Status == "TRIAL" {
		record.Status, record.TrialEndsAt = "ACTIVE", nil
	}
}
func validActionFields(reason, key string) bool {
	return reason != "" && len(reason) <= 500 && key != "" && len(key) <= 120
}

func requireCPO(tx *gorm.DB, cpoID uuid.UUID) error {
	var count int64
	if err := tx.Model(&models.CPO{}).Where("id = ?", cpoID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return notFound("cpo_not_found", "CPO was not found.", gorm.ErrRecordNotFound)
	}
	return nil
}
func publishedVersion(tx *gorm.DB, id uuid.UUID) (models.SubscriptionPlanVersion, error) {
	var version models.SubscriptionPlanVersion
	if err := tx.Joins("JOIN subscription_plans ON subscription_plans.id = subscription_plan_versions.plan_id").Where("subscription_plan_versions.id = ? AND subscription_plan_versions.status = 'PUBLISHED' AND subscription_plans.status <> 'ARCHIVED'", id).First(&version).Error; err != nil {
		return version, notFound("published_plan_version_not_found", "Published plan version was not found.", err)
	}
	return version, nil
}

func (service *Service) recordTransition(tx *gorm.DB, actor uuid.UUID, record models.CPOSubscription, previousStatus *string, previousVersion *uuid.UUID, reason, key string, now time.Time, operation string) error {
	history := models.CPOSubscriptionHistory{ID: uuid.New(), SubscriptionID: record.ID, CPOID: record.CPOID, PreviousStatus: previousStatus, NextStatus: record.Status, PreviousPlanVersionID: previousVersion, NextPlanVersionID: record.PlanVersionID, ActorUserID: actor, Reason: reason, IdempotencyKey: key, EffectiveAt: now, Metadata: models.JSONB{"operation": operation}, CreatedAt: now}
	if err := tx.Create(&history).Error; err != nil {
		return mapWriteError(err, "idempotency_conflict")
	}
	return nil
}

func (service *Service) idempotentSubscription(tx *gorm.DB, actor uuid.UUID, key, operation string, cpoID uuid.UUID) (models.CPOSubscription, bool, error) {
	var history models.CPOSubscriptionHistory
	result := tx.Where("actor_user_id = ? AND idempotency_key = ?", actor, key).First(&history)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return models.CPOSubscription{}, false, nil
	}
	if result.Error != nil {
		return models.CPOSubscription{}, false, result.Error
	}
	if history.CPOID != cpoID || fmt.Sprint(history.Metadata["operation"]) != operation {
		return models.CPOSubscription{}, false, conflict("idempotency_conflict", "The idempotency key was already used for a different operation.")
	}
	var record models.CPOSubscription
	if err := tx.First(&record, "id = ?", history.SubscriptionID).Error; err != nil {
		return record, false, err
	}
	return record, true, nil
}

func (service *Service) emit(tx *gorm.DB, actor uuid.UUID, eventType, resourceType, resourceID string, data models.JSONB) error {
	if service.events == nil {
		return nil
	}
	_, err := service.events.Emit(tx, platformops.EventInput{Type: eventType, ActorUserID: &actor, ResourceType: resourceType, ResourceID: &resourceID, Data: data})
	if err != nil {
		return fmt.Errorf("emit platform event: %w", err)
	}
	return nil
}
func writeAudit(tx *gorm.DB, actor uuid.UUID, action, entity string, entityID uuid.UUID, details models.JSONB, now time.Time) error {
	return tx.Create(&models.AuditLog{ID: uuid.New(), UserID: &actor, Action: action, Entity: entity, EntityID: &entityID, Details: details, CreatedAt: now}).Error
}
func requirePlatform(principal auth.Principal) error {
	if principal.Scope != "PLATFORM" {
		return &auth.APIError{Status: http.StatusForbidden, Code: "permission_denied", Message: "Platform superadmin access is required."}
	}
	return nil
}
func invalid(field, message string) error {
	return &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_" + field, Message: message}
}
func conflict(code, message string) error {
	return &auth.APIError{Status: http.StatusConflict, Code: code, Message: message}
}
func notFound(code, message string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{Status: http.StatusNotFound, Code: code, Message: message}
	}
	return err
}
func mapWriteError(err error, code string) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && (pgError.Code == "23505" || pgError.Code == "23514" || pgError.Code == "23503") {
		return conflict(code, "The requested subscription operation conflicts with current state.")
	}
	return err
}
