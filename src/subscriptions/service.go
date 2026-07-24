package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	planCodePattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
	featurePattern  = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

type Service struct {
	database    *gorm.DB
	outbox      *cmsmail.Outbox
	mailEnabled bool
	events      *platformops.Service
	now         func() time.Time
}

func NewService(
	database *gorm.DB,
	outbox *cmsmail.Outbox,
	mailEnabled bool,
	events *platformops.Service,
) *Service {
	return &Service{
		database: database, outbox: outbox, mailEnabled: mailEnabled, events: events,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (service *Service) CreatePlan(
	ctx context.Context,
	principal auth.Principal,
	request CreatePlanRequest,
) (PlanView, error) {
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
	plan := models.SubscriptionPlan{
		ID: uuid.New(), Code: request.Code, Name: request.Name,
		Description: request.Description, Status: "DRAFT",
		CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now,
	}
	version := versionFromTerms(plan.ID, 1, request.Terms, now)
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan).Error; err != nil {
			return mapWriteError(err, "plan_conflict")
		}
		if err := tx.Create(&version).Error; err != nil {
			return mapWriteError(err, "plan_conflict")
		}
		if err := replaceEntitlements(tx, version.ID, request.Terms.Entitlements, now); err != nil {
			return err
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
	result := PlanView{Plan: plan, Published: []models.SubscriptionPlanVersion{}, Entitlements: map[string][]models.SubscriptionPlanEntitlement{}}
	for _, version := range versions {
		var values []models.SubscriptionPlanEntitlement
		if err := service.database.WithContext(ctx).Where("plan_version_id = ?", version.ID).Order("feature_key").Find(&values).Error; err != nil {
			return PlanView{}, fmt.Errorf("load plan entitlements: %w", err)
		}
		result.Entitlements[version.ID.String()] = values
		if version.Status == "DRAFT" {
			copy := version
			result.Draft = &copy
		} else {
			result.Published = append(result.Published, version)
		}
	}
	return result, nil
}

func (service *Service) UpdateDraft(
	ctx context.Context,
	principal auth.Principal,
	planID uuid.UUID,
	request UpdateDraftRequest,
) (PlanView, error) {
	if err := requirePlatform(principal); err != nil {
		return PlanView{}, err
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	request.Terms = normalizeTerms(request.Terms)
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
			var maxVersion int
			if err := tx.Model(&models.SubscriptionPlanVersion{}).Where("plan_id = ?", planID).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
				return err
			}
			draft = versionFromTerms(planID, maxVersion+1, request.Terms, now)
			if err := tx.Create(&draft).Error; err != nil {
				return mapWriteError(err, "plan_conflict")
			}
		} else if result.Error != nil {
			return result.Error
		} else {
			updates := versionFromTerms(planID, draft.Version, request.Terms, now)
			if err := tx.Model(&draft).Updates(map[string]any{
				"currency": updates.Currency, "price_minor": updates.PriceMinor,
				"billing_interval": updates.BillingInterval, "interval_count": updates.IntervalCount,
				"trial_days": updates.TrialDays, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Where("plan_version_id = ?", draft.ID).Delete(&models.SubscriptionPlanEntitlement{}).Error; err != nil {
				return err
			}
		}
		if err := replaceEntitlements(tx, draft.ID, request.Terms.Entitlements, now); err != nil {
			return err
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

func normalizeTerms(input PlanTermsInput) PlanTermsInput {
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.BillingInterval = strings.ToUpper(strings.TrimSpace(input.BillingInterval))
	if input.IntervalCount == 0 {
		input.IntervalCount = 1
	}
	for index := range input.Entitlements {
		input.Entitlements[index].FeatureKey = strings.ToLower(strings.TrimSpace(input.Entitlements[index].FeatureKey))
		if input.Entitlements[index].Configuration == nil {
			input.Entitlements[index].Configuration = models.JSONB{}
		}
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
	seen := map[string]struct{}{}
	for _, entitlement := range terms.Entitlements {
		if !featurePattern.MatchString(entitlement.FeatureKey) || len(entitlement.FeatureKey) > 120 {
			return invalid("entitlements", "Each feature_key must be a valid lowercase feature identifier.")
		}
		if entitlement.LimitValue != nil && *entitlement.LimitValue < 0 {
			return invalid("entitlements", "Entitlement limits must not be negative.")
		}
		if _, exists := seen[entitlement.FeatureKey]; exists {
			return invalid("entitlements", "Entitlement feature keys must be unique.")
		}
		seen[entitlement.FeatureKey] = struct{}{}
	}
	return nil
}

func versionFromTerms(planID uuid.UUID, version int, terms PlanTermsInput, now time.Time) models.SubscriptionPlanVersion {
	return models.SubscriptionPlanVersion{
		ID: uuid.New(), PlanID: planID, Version: version, Status: "DRAFT",
		Currency: terms.Currency, PriceMinor: terms.PriceMinor,
		BillingInterval: terms.BillingInterval, IntervalCount: terms.IntervalCount,
		TrialDays: terms.TrialDays, CreatedAt: now, UpdatedAt: now,
	}
}

func replaceEntitlements(tx *gorm.DB, versionID uuid.UUID, inputs []EntitlementInput, now time.Time) error {
	for _, input := range inputs {
		record := models.SubscriptionPlanEntitlement{
			ID: uuid.New(), PlanVersionID: versionID, FeatureKey: input.FeatureKey,
			Enabled: input.Enabled, LimitValue: input.LimitValue,
			Configuration: input.Configuration, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return mapWriteError(err, "plan_conflict")
		}
	}
	return nil
}

func addPeriod(start time.Time, version models.SubscriptionPlanVersion) time.Time {
	if version.BillingInterval == "YEARLY" {
		return start.AddDate(version.IntervalCount, 0, 0)
	}
	return start.AddDate(0, version.IntervalCount, 0)
}

func (service *Service) Assign(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request AssignRequest) (SubscriptionView, error) {
	if err := requirePlatform(principal); err != nil {
		return SubscriptionView{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.PlanVersionID == uuid.Nil || request.Reason == "" || len(request.Reason) > 500 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 120 {
		return SubscriptionView{}, invalid("request", "plan_version_id, reason, and idempotency_key are required and must be valid.")
	}
	now := service.now()
	start := now
	if request.StartsAt != nil {
		start = request.StartsAt.UTC()
	}
	if start.After(now) {
		return SubscriptionView{}, invalid(
			"starts_at",
			"starts_at cannot be in the future.",
		)
	}
	var subscription models.CPOSubscription
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, ok, err := service.idempotentSubscription(
			tx,
			principal.UserID,
			request.IdempotencyKey,
			"ASSIGN",
			cpoID,
		); err != nil {
			return err
		} else if ok {
			subscription = existing
			return nil
		}
		var cpo models.CPO
		if err := tx.First(&cpo, "id = ?", cpoID).Error; err != nil {
			return notFound("cpo_not_found", "CPO was not found.", err)
		}
		version, err := publishedVersion(tx, request.PlanVersionID)
		if err != nil {
			return err
		}
		status := "ACTIVE"
		var trialEnd *time.Time
		if version.TrialDays > 0 {
			status = "TRIAL"
			value := start.AddDate(0, 0, version.TrialDays)
			trialEnd = &value
		}
		subscription = models.CPOSubscription{
			ID: uuid.New(), CPOID: cpoID, PlanVersionID: version.ID, Status: status,
			StartsAt: start, TrialEndsAt: trialEnd,
			CurrentPeriodStartsAt: start, CurrentPeriodEndsAt: addPeriod(start, version),
			CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&subscription).Error; err != nil {
			return mapWriteError(err, "subscription_conflict")
		}
		if err := service.recordTransition(tx, principal.UserID, subscription, nil, nil, request.Reason, request.IdempotencyKey, now, "ASSIGN"); err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "CPO_SUBSCRIPTION_ASSIGNED", "CPO_SUBSCRIPTION", subscription.ID, models.JSONB{"cpo_id": cpoID, "status": status, "plan_version_id": version.ID}, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, "platform.subscription.assigned", "CPO_SUBSCRIPTION", subscription.ID.String(), models.JSONB{"cpo_id": cpoID, "status": status, "plan_version_id": version.ID}); err != nil {
			return err
		}
		return service.notifyCPOAdmins(tx, cpo, subscription, version, "assigned")
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
	var record models.CPOSubscription
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND status IN ?", cpoID, []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}).First(&record).Error; err != nil {
		return SubscriptionView{}, notFound("subscription_not_found", "CPO has no current subscription.", err)
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

func (service *Service) ChangePlan(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request ChangePlanRequest) (SubscriptionView, error) {
	request.Effective = strings.ToUpper(strings.TrimSpace(request.Effective))
	if request.Effective != "IMMEDIATE" && request.Effective != "PERIOD_END" {
		return SubscriptionView{}, invalid("effective", "effective must be IMMEDIATE or PERIOD_END.")
	}
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "CHANGE_PLAN", func(record *models.CPOSubscription, tx *gorm.DB, now time.Time) (string, error) {
		version, err := publishedVersion(tx, request.PlanVersionID)
		if err != nil {
			return "", err
		}
		if request.Effective == "PERIOD_END" {
			record.PendingPlanVersionID = &version.ID
			at := record.CurrentPeriodEndsAt
			record.PendingChangeAt = &at
			record.PendingChangeBy = &principal.UserID
			return "platform.subscription.plan_change_scheduled", nil
		}
		record.PlanVersionID = version.ID
		record.PendingPlanVersionID = nil
		record.PendingChangeAt = nil
		record.PendingChangeBy = nil
		record.CurrentPeriodStartsAt = now
		record.CurrentPeriodEndsAt = addPeriod(now, version)
		return "platform.subscription.plan_changed", nil
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

func (service *Service) MarkPastDue(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request TransitionRequest,
) (SubscriptionView, error) {
	return service.mutate(
		ctx,
		principal,
		cpoID,
		request.Reason,
		request.IdempotencyKey,
		"MARK_PAST_DUE",
		func(
			record *models.CPOSubscription,
			_ *gorm.DB,
			_ time.Time,
		) (string, error) {
			if record.Status == "PAST_DUE" {
				return "platform.subscription.past_due", nil
			}
			if record.Status != "ACTIVE" &&
				record.Status != "TRIAL" &&
				record.Status != "PAUSED" {
				return "", conflict(
					"invalid_subscription_transition",
					"This subscription cannot be marked past due.",
				)
			}
			record.Status = "PAST_DUE"
			return "platform.subscription.past_due", nil
		},
	)
}

func (service *Service) Expire(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request TransitionRequest,
) (SubscriptionView, error) {
	return service.mutate(
		ctx,
		principal,
		cpoID,
		request.Reason,
		request.IdempotencyKey,
		"EXPIRE",
		func(
			record *models.CPOSubscription,
			_ *gorm.DB,
			now time.Time,
		) (string, error) {
			record.Status = "EXPIRED"
			record.EndedAt = &now
			record.CancelAtPeriodEnd = false
			record.CancellationScheduledBy = nil
			record.PendingPlanVersionID = nil
			record.PendingChangeAt = nil
			record.PendingChangeBy = nil
			return "platform.subscription.expired", nil
		},
	)
}

func (service *Service) Cancel(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, request TransitionRequest) (SubscriptionView, error) {
	return service.mutate(ctx, principal, cpoID, request.Reason, request.IdempotencyKey, "CANCEL", func(record *models.CPOSubscription, _ *gorm.DB, now time.Time) (string, error) {
		if request.AtPeriodEnd {
			record.CancelAtPeriodEnd = true
			record.CancellationScheduledBy = &principal.UserID
			return "platform.subscription.cancellation_scheduled", nil
		}
		record.Status = "CANCELLED"
		record.CancelAtPeriodEnd = false
		record.CancellationScheduledBy = nil
		record.CancelledAt = &now
		record.EndedAt = &now
		record.PendingPlanVersionID = nil
		record.PendingChangeAt = nil
		record.PendingChangeBy = nil
		return "platform.subscription.cancelled", nil
	})
}

type mutation func(*models.CPOSubscription, *gorm.DB, time.Time) (string, error)

func (service *Service) mutate(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, reason, key, operation string, change mutation) (SubscriptionView, error) {
	if err := requirePlatform(principal); err != nil {
		return SubscriptionView{}, err
	}
	reason, key = strings.TrimSpace(reason), strings.TrimSpace(key)
	if reason == "" || len(reason) > 500 || key == "" || len(key) > 120 {
		return SubscriptionView{}, invalid("request", "reason and idempotency_key are required and must be within limits.")
	}
	now := service.now()
	var record models.CPOSubscription
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, ok, err := service.idempotentSubscription(
			tx,
			principal.UserID,
			key,
			operation,
			cpoID,
		); err != nil {
			return err
		} else if ok {
			record = existing
			return nil
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("cpo_id = ? AND status IN ?", cpoID, []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}).First(&record).Error; err != nil {
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
		if err := service.emit(tx, principal.UserID, eventType, "CPO_SUBSCRIPTION", record.ID.String(), models.JSONB{"cpo_id": cpoID, "status": record.Status, "plan_version_id": record.PlanVersionID}); err != nil {
			return err
		}
		var cpo models.CPO
		if err := tx.First(&cpo, "id = ?", cpoID).Error; err != nil {
			return err
		}
		var version models.SubscriptionPlanVersion
		if err := tx.First(&version, "id = ?", record.PlanVersionID).Error; err != nil {
			return err
		}
		return service.notifyCPOAdmins(
			tx,
			cpo,
			record,
			version,
			strings.ToLower(strings.ReplaceAll(operation, "_", " ")),
		)
	})
	if err != nil {
		return SubscriptionView{}, err
	}
	return service.subscriptionView(ctx, record)
}

func (service *Service) EffectiveEntitlements(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) (EffectiveEntitlementsResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return EffectiveEntitlementsResponse{}, err
	}
	response := EffectiveEntitlementsResponse{Entitlements: []EffectiveEntitlement{}}
	current, err := service.GetCurrent(ctx, principal, cpoID)
	if err == nil {
		response.Subscription = &current
		var base []models.SubscriptionPlanEntitlement
		if err := service.database.WithContext(ctx).Where("plan_version_id = ?", current.Version.ID).Find(&base).Error; err != nil {
			return response, err
		}
		for _, value := range base {
			response.Entitlements = append(response.Entitlements, EffectiveEntitlement{FeatureKey: value.FeatureKey, Enabled: value.Enabled, LimitValue: value.LimitValue, Configuration: value.Configuration, Source: "PLAN"})
		}
	} else {
		var apiError *auth.APIError
		if !errors.As(err, &apiError) || apiError.Code != "subscription_not_found" {
			return response, err
		}
	}
	var overrides []models.CPOEntitlementOverride
	now := service.now()
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND (expires_at IS NULL OR expires_at > ?)", cpoID, now).Find(&overrides).Error; err != nil {
		return response, err
	}
	index := map[string]int{}
	for i, value := range response.Entitlements {
		index[value.FeatureKey] = i
	}
	for _, value := range overrides {
		resolved := EffectiveEntitlement{FeatureKey: value.FeatureKey, Enabled: value.Enabled, LimitValue: value.LimitValue, Configuration: value.Configuration, Source: "OVERRIDE", ExpiresAt: value.ExpiresAt}
		if position, exists := index[value.FeatureKey]; exists {
			response.Entitlements[position] = resolved
		} else {
			response.Entitlements = append(response.Entitlements, resolved)
		}
	}
	sort.Slice(response.Entitlements, func(i, j int) bool { return response.Entitlements[i].FeatureKey < response.Entitlements[j].FeatureKey })
	return response, nil
}

func (service *Service) SetOverride(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, featureKey string, request OverrideRequest) (models.CPOEntitlementOverride, error) {
	if err := requirePlatform(principal); err != nil {
		return models.CPOEntitlementOverride{}, err
	}
	featureKey, request.Reason = strings.ToLower(strings.TrimSpace(featureKey)), strings.TrimSpace(request.Reason)
	if !featurePattern.MatchString(featureKey) || len(featureKey) > 120 || request.Reason == "" || len(request.Reason) > 500 || (request.LimitValue != nil && *request.LimitValue < 0) || (request.ExpiresAt != nil && !request.ExpiresAt.After(service.now())) {
		return models.CPOEntitlementOverride{}, invalid("override", "feature key, reason, limit, or expiry is invalid.")
	}
	if request.Configuration == nil {
		request.Configuration = models.JSONB{}
	}
	now := service.now()
	record := models.CPOEntitlementOverride{ID: uuid.New(), CPOID: cpoID, FeatureKey: featureKey, Enabled: request.Enabled, LimitValue: request.LimitValue, Configuration: request.Configuration, Reason: request.Reason, ExpiresAt: request.ExpiresAt, CreatedBy: principal.UserID, CreatedAt: now, UpdatedAt: now}
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.CPO{}).Where("id = ?", cpoID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return notFound("cpo_not_found", "CPO was not found.", gorm.ErrRecordNotFound)
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cpo_id"}, {Name: "feature_key"}}, DoUpdates: clause.Assignments(map[string]any{"enabled": record.Enabled, "limit_value": record.LimitValue, "configuration": record.Configuration, "reason": record.Reason, "expires_at": record.ExpiresAt, "created_by": record.CreatedBy, "updated_at": now})}).Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Where("cpo_id = ? AND feature_key = ?", cpoID, featureKey).First(&record).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, principal.UserID, "CPO_ENTITLEMENT_OVERRIDE_SET", "CPO", cpoID, models.JSONB{"feature_key": featureKey, "enabled": record.Enabled, "expires_at": record.ExpiresAt}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.entitlement_override_set", "CPO", cpoID.String(), models.JSONB{"feature_key": featureKey})
	})
	return record, err
}

func (service *Service) DeleteOverride(ctx context.Context, principal auth.Principal, cpoID uuid.UUID, featureKey, reason string) error {
	if err := requirePlatform(principal); err != nil {
		return err
	}
	featureKey, reason = strings.ToLower(strings.TrimSpace(featureKey)), strings.TrimSpace(reason)
	if !featurePattern.MatchString(featureKey) || reason == "" || len(reason) > 500 {
		return invalid("request", "feature key and reason are required and must be valid.")
	}
	now := service.now()
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("cpo_id = ? AND feature_key = ?", cpoID, featureKey).Delete(&models.CPOEntitlementOverride{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return notFound("entitlement_override_not_found", "Entitlement override was not found.", gorm.ErrRecordNotFound)
		}
		if err := writeAudit(tx, principal.UserID, "CPO_ENTITLEMENT_OVERRIDE_REMOVED", "CPO", cpoID, models.JSONB{"feature_key": featureKey, "reason": reason}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.subscription.entitlement_override_removed", "CPO", cpoID.String(), models.JSONB{"feature_key": featureKey})
	})
}

func (service *Service) RunLifecycle(
	ctx context.Context,
	instanceKey string,
	every time.Duration,
) {
	const workerName = "subscription-lifecycle"
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if service.events != nil {
			if err := service.events.Heartbeat(ctx, workerName, instanceKey); err != nil &&
				ctx.Err() == nil {
				log.Printf("record subscription lifecycle heartbeat: %v", err)
			}
		}
		if err := service.ReconcileDue(ctx); err != nil && ctx.Err() == nil {
			log.Printf("reconcile due subscriptions: %v", err)
		} else if service.events != nil {
			if err := service.events.JobCompleted(ctx, workerName, instanceKey); err != nil &&
				ctx.Err() == nil {
				log.Printf("record subscription lifecycle completion: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) ReconcileDue(ctx context.Context) error {
	for {
		processed := false
		err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			now := service.now()
			var record models.CPOSubscription
			result := tx.Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			}).Where(
				"status IN ? AND ("+
					"(status = 'TRIAL' AND trial_ends_at IS NOT NULL AND trial_ends_at <= ?) OR "+
					"(cancel_at_period_end = TRUE AND current_period_ends_at <= ?) OR "+
					"(pending_change_at IS NOT NULL AND pending_change_at <= ?))",
				[]string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"},
				now,
				now,
				now,
			).Order("current_period_ends_at, id").First(&record)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return nil
			}
			if result.Error != nil {
				return result.Error
			}
			processed = true
			previousStatus, previousVersion := record.Status, record.PlanVersionID
			eventType, operation := "", ""
			var actor uuid.UUID
			switch {
			case record.CancelAtPeriodEnd &&
				!record.CurrentPeriodEndsAt.After(now):
				if record.CancellationScheduledBy == nil {
					return errors.New("scheduled cancellation has no actor")
				}
				actor = *record.CancellationScheduledBy
				record.Status = "CANCELLED"
				record.CancelAtPeriodEnd = false
				record.CancellationScheduledBy = nil
				record.CancelledAt = &now
				record.EndedAt = &now
				record.PendingPlanVersionID = nil
				record.PendingChangeAt = nil
				record.PendingChangeBy = nil
				eventType = "platform.subscription.cancelled"
				operation = "APPLY_SCHEDULED_CANCELLATION"
			case record.PendingChangeAt != nil &&
				!record.PendingChangeAt.After(now):
				if record.PendingPlanVersionID == nil ||
					record.PendingChangeBy == nil {
					return errors.New("scheduled plan change is incomplete")
				}
				actor = *record.PendingChangeBy
				version, err := publishedVersionIncludingArchived(
					tx,
					*record.PendingPlanVersionID,
				)
				if err != nil {
					return err
				}
				record.PlanVersionID = version.ID
				record.PendingPlanVersionID = nil
				record.PendingChangeAt = nil
				record.PendingChangeBy = nil
				record.CurrentPeriodStartsAt = now
				record.CurrentPeriodEndsAt = addPeriod(now, version)
				eventType = "platform.subscription.plan_changed"
				operation = "APPLY_SCHEDULED_PLAN_CHANGE"
			case record.Status == "TRIAL" &&
				record.TrialEndsAt != nil &&
				!record.TrialEndsAt.After(now):
				actor = record.CreatedBy
				record.Status = "ACTIVE"
				eventType = "platform.subscription.trial_completed"
				operation = "COMPLETE_TRIAL"
			default:
				return nil
			}
			record.UpdatedAt = now
			if err := tx.Save(&record).Error; err != nil {
				return err
			}
			key := fmt.Sprintf(
				"system:%s:%s:%d",
				record.ID,
				operation,
				now.Unix(),
			)
			if err := service.recordTransition(
				tx,
				actor,
				record,
				&previousStatus,
				&previousVersion,
				"Applied automatically at the recorded lifecycle boundary.",
				key,
				now,
				operation,
			); err != nil {
				return err
			}
			if err := writeAudit(
				tx,
				actor,
				"CPO_SUBSCRIPTION_"+operation,
				"CPO_SUBSCRIPTION",
				record.ID,
				models.JSONB{
					"cpo_id":          record.CPOID,
					"previous_status": previousStatus,
					"status":          record.Status,
					"executed_by":     "subscription-lifecycle",
				},
				now,
			); err != nil {
				return err
			}
			if err := service.emit(
				tx,
				actor,
				eventType,
				"CPO_SUBSCRIPTION",
				record.ID.String(),
				models.JSONB{
					"cpo_id": record.CPOID,
					"status": record.Status,
				},
			); err != nil {
				return err
			}
			var cpo models.CPO
			if err := tx.First(&cpo, "id = ?", record.CPOID).Error; err != nil {
				return err
			}
			var version models.SubscriptionPlanVersion
			if err := tx.First(&version, "id = ?", record.PlanVersionID).Error; err != nil {
				return err
			}
			return service.notifyCPOAdmins(
				tx,
				cpo,
				record,
				version,
				strings.ToLower(strings.ReplaceAll(operation, "_", " ")),
			)
		})
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func publishedVersion(tx *gorm.DB, id uuid.UUID) (models.SubscriptionPlanVersion, error) {
	var version models.SubscriptionPlanVersion
	if err := tx.
		Joins(
			"JOIN subscription_plans ON subscription_plans.id = "+
				"subscription_plan_versions.plan_id",
		).
		Where(
			"subscription_plan_versions.id = ? "+
				"AND subscription_plan_versions.status = 'PUBLISHED' "+
				"AND subscription_plans.status <> 'ARCHIVED'",
			id,
		).
		First(&version).Error; err != nil {
		return version, notFound("published_plan_version_not_found", "Published plan version was not found.", err)
	}
	return version, nil
}

func publishedVersionIncludingArchived(
	tx *gorm.DB,
	id uuid.UUID,
) (models.SubscriptionPlanVersion, error) {
	var version models.SubscriptionPlanVersion
	if err := tx.Where(
		"id = ? AND status = 'PUBLISHED'",
		id,
	).First(&version).Error; err != nil {
		return version, notFound(
			"published_plan_version_not_found",
			"Published plan version was not found.",
			err,
		)
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

func (service *Service) idempotentSubscription(
	tx *gorm.DB,
	actor uuid.UUID,
	key string,
	operation string,
	cpoID uuid.UUID,
) (models.CPOSubscription, bool, error) {
	var history models.CPOSubscriptionHistory
	result := tx.Where("actor_user_id = ? AND idempotency_key = ?", actor, key).First(&history)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return models.CPOSubscription{}, false, nil
	}
	if result.Error != nil {
		return models.CPOSubscription{}, false, result.Error
	}
	if history.CPOID != cpoID ||
		fmt.Sprint(history.Metadata["operation"]) != operation {
		return models.CPOSubscription{}, false, conflict(
			"idempotency_conflict",
			"The idempotency key was already used for a different operation.",
		)
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
	returnError := ""
	_, err := service.events.Emit(tx, platformops.EventInput{Type: eventType, ActorUserID: &actor, ResourceType: resourceType, ResourceID: &resourceID, Data: data})
	if err != nil {
		returnError = err.Error()
	}
	if returnError != "" {
		return fmt.Errorf("emit platform event: %s", returnError)
	}
	return nil
}

func (service *Service) notifyCPOAdmins(tx *gorm.DB, cpo models.CPO, subscription models.CPOSubscription, version models.SubscriptionPlanVersion, change string) error {
	if !service.mailEnabled || service.outbox == nil {
		return nil
	}
	var users []models.User
	if err := tx.Table("users").Select("users.*").Joins("JOIN cpo_memberships ON cpo_memberships.user_id = users.id").Where("cpo_memberships.cpo_id = ? AND cpo_memberships.status = 'ACTIVE' AND cpo_memberships.role IN ('OWNER', 'ADMIN')", cpo.ID).Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		if err := service.outbox.EnqueueMessage(tx, user.Email, "CPO_SUBSCRIPTION_CHANGED", cmsmail.MessagePayload{RecipientName: user.FullName, CPOName: cpo.BusinessName, CPOID: cpo.ID.String(), SubscriptionStatus: subscription.Status, SubscriptionChange: change, PlanVersionID: version.ID.String()}); err != nil {
			return err
		}
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
