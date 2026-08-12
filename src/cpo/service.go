package cpo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	netmail "net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const dummyAppIDPrefix = "cpo_dummy_"

const (
	defaultListLimit = 50
	maxListLimit     = 200
	maxSearchLength  = 200
	minReasonLength  = 3
	maxReasonLength  = 500
)

var (
	slugPattern                    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	gstinPattern                   = regexp.MustCompile(`^[0-9A-Z]{15}$`)
	appIDPattern                   = regexp.MustCompile(`^[a-z0-9_-]{16,100}$`)
	currentCPOSubscriptionStatuses = []string{"TRIAL", "ACTIVE", "PAUSED", "PAST_DUE"}
)

type Service struct {
	database             *gorm.DB
	outbox               *cmsmail.Outbox
	mailEnabled          bool
	events               *platformops.Service
	now                  func() time.Time
	chargerConnectionURL string
}

type sessionRevocationCounts struct {
	sessions      int64
	refreshTokens int64
}

func (service *Service) WithPlatformEvents(events *platformops.Service) *Service {
	service.events = events
	return service
}

func NewService(
	database *gorm.DB,
	outbox *cmsmail.Outbox,
	mailEnabled bool,
	chargerConnectionURL string,
) *Service {
	return &Service{
		database:             database,
		outbox:               outbox,
		mailEnabled:          mailEnabled,
		now:                  func() time.Time { return time.Now().UTC() },
		chargerConnectionURL: chargerConnectionURL,
	}
}

func (service *Service) Create(
	ctx context.Context,
	principal auth.Principal,
	request CreateRequest,
) (CreateResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return CreateResponse{}, err
	}
	if !service.mailEnabled {
		return CreateResponse{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request = normalizeCreateRequest(request)
	if err := validateCreateRequest(request); err != nil {
		return CreateResponse{}, err
	}
	randomID, err := security.RandomHex(16)
	if err != nil {
		return CreateResponse{}, err
	}
	dummyAppID := dummyAppIDPrefix + randomID
	now := service.now()
	statusActorID := principal.UserID
	cpoRecord := models.CPO{
		ID:                    uuid.New(),
		Slug:                  request.Slug,
		BusinessName:          request.BusinessName,
		CompanyType:           request.CompanyType,
		GSTIN:                 request.GSTIN,
		Address:               request.Address,
		City:                  request.City,
		State:                 request.State,
		Pincode:               request.Pincode,
		Status:                constants.CPOStatusPending,
		StatusReason:          "Initial provisioning",
		StatusChangedAt:       now,
		StatusChangedByUserID: &statusActorID,
		AppID:                 dummyAppID,
		AppIDMode:             constants.CPOAppIDModeDummy,
		AppIDUpdatedAt:        now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	var admin models.User
	identityCreated := false
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-admin:"+request.Admin.Email,
		).Error; err != nil {
			return fmt.Errorf("serialize initial CPO administrator: %w", err)
		}
		if err := tx.Create(&cpoRecord).Error; err != nil {
			return mapWriteError(err, "create CPO")
		}

		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", request.Admin.Email).
			First(&admin)
		switch {
		case result.Error == nil:
			if !admin.IsActive {
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "admin_identity_inactive",
					Message: "The administrator identity exists but is inactive.",
				}
			}
		case errors.Is(result.Error, gorm.ErrRecordNotFound):
			temporaryPassword, err := generateTemporaryPassword()
			if err != nil {
				return err
			}
			passwordHash, err := security.HashPassword(temporaryPassword)
			if err != nil {
				return fmt.Errorf("hash temporary administrator password: %w", err)
			}
			admin = models.User{
				ID:                 uuid.New(),
				Email:              request.Admin.Email,
				PasswordHash:       passwordHash,
				FullName:           request.Admin.FullName,
				IsActive:           true,
				IsVerified:         false,
				MFAEnabled:         false,
				MustChangePassword: true,
				PasswordChangedAt:  now,
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if err := tx.Create(&admin).Error; err != nil {
				return mapWriteError(err, "create initial CPO administrator")
			}
			identityCreated = true
			if err := service.outbox.EnqueueMessageWithContext(
				tx,
				admin.Email,
				"CPO_ADMIN_WELCOME",
				cmsmail.MessagePayload{
					RecipientName:     admin.FullName,
					TemporaryPassword: temporaryPassword,
					CPOName:           cpoRecord.BusinessName,
					CPOID:             cpoRecord.ID.String(),
					CPOAppID:          cpoRecord.AppID,
				},
				cmsmail.MessageContext{
					CPOID:  &cpoRecord.ID,
					UserID: &admin.ID,
				},
			); err != nil {
				return err
			}
		default:
			return fmt.Errorf("find initial CPO administrator: %w", result.Error)
		}

		membership := models.CPOMembership{
			ID:             uuid.New(),
			CPOID:          cpoRecord.ID,
			UserID:         admin.ID,
			Role:           constants.CPORoleAdmin,
			Status:         constants.MembershipStatusActive,
			IsPrimaryAdmin: true,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return mapWriteError(err, "create initial CPO administrator membership")
		}
		if !identityCreated {
			if err := service.outbox.EnqueueMessageWithContext(
				tx,
				admin.Email,
				"CPO_MEMBERSHIP_ASSIGNED",
				cmsmail.MessagePayload{
					RecipientName: admin.FullName,
					CPOName:       cpoRecord.BusinessName,
					CPOID:         cpoRecord.ID.String(),
					CPOAppID:      cpoRecord.AppID,
				},
				cmsmail.MessageContext{
					CPOID:  &cpoRecord.ID,
					UserID: &admin.ID,
				},
			); err != nil {
				return err
			}
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoRecord.ID,
			"CPO_CREATED",
			models.JSONB{
				"app_id_mode":      constants.CPOAppIDModeDummy,
				"admin_user_id":    admin.ID,
				"identity_created": identityCreated,
			},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoRecord.ID,
			"platform.cpo.created",
			models.JSONB{
				"status":      cpoRecord.Status,
				"app_id_mode": cpoRecord.AppIDMode,
			},
		)
	})
	if err != nil {
		return CreateResponse{}, err
	}
	return CreateResponse{
		CPO: view(cpoRecord),
		Admin: InitialAdminView{
			UserID:          admin.ID,
			Email:           admin.Email,
			FullName:        admin.FullName,
			Role:            constants.CPORoleAdmin,
			IdentityCreated: identityCreated,
		},
	}, nil
}

func (service *Service) List(
	ctx context.Context,
	principal auth.Principal,
	query ListQuery,
) (ListResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return ListResponse{}, err
	}
	query.Search = strings.TrimSpace(query.Search)
	if len(query.Search) > maxSearchLength {
		return ListResponse{}, invalid(
			"q",
			"Search text must not exceed 200 characters.",
		)
	}
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return ListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}
	if query.Status != nil && !query.Status.Valid() {
		return ListResponse{}, invalid(
			"status",
			"Status must be PENDING, ACTIVE, or SUSPENDED.",
		)
	}
	if query.AppMode != nil && !query.AppMode.Valid() {
		return ListResponse{}, invalid(
			"app_id_mode",
			"App ID mode must be DUMMY or LIVE.",
		)
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return ListResponse{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}

	databaseQuery := service.database.WithContext(ctx).Model(&models.CPO{})
	if query.Search != "" {
		search := strings.ToLower(query.Search)
		databaseQuery = databaseQuery.Where(`
			strpos(lower(cpos.business_name), ?) > 0
			OR strpos(lower(cpos.slug), ?) > 0
			OR strpos(lower(coalesce(cpos.gstin, '')), ?) > 0
			OR strpos(lower(cpos.app_id), ?) > 0
			OR EXISTS (
				SELECT 1
				FROM cpo_memberships
				JOIN users ON users.id = cpo_memberships.user_id
				WHERE cpo_memberships.cpo_id = cpos.id
				  AND cpo_memberships.is_primary_admin
				  AND (
				      strpos(lower(users.email), ?) > 0
				      OR strpos(lower(users.full_name), ?) > 0
				  )
			)
		`, search, search, search, search, search, search)
	}
	if query.Status != nil {
		databaseQuery = databaseQuery.Where("cpos.status = ?", *query.Status)
	}
	if query.AppMode != nil {
		databaseQuery = databaseQuery.Where("cpos.app_id_mode = ?", *query.AppMode)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(cpos.created_at, cpos.id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.CPO
	if err := databaseQuery.
		Order("cpos.created_at DESC, cpos.id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return ListResponse{}, fmt.Errorf("list CPOs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	result := make([]View, 0, len(records))
	for _, record := range records {
		result = append(result, view(record))
	}
	response := ListResponse{CPOs: result, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) Get(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	record, err := service.find(ctx, cpoID)
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) CheckSlugAvailability(
	ctx context.Context,
	principal auth.Principal,
	candidate string,
) (SlugAvailabilityResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return SlugAvailabilityResponse{}, err
	}

	slug := normalizeSlug(candidate)
	if err := validateSlug(slug); err != nil {
		return SlugAvailabilityResponse{}, err
	}

	var count int64
	if err := service.database.WithContext(ctx).
		Model(&models.CPO{}).
		Where("lower(slug) = ?", slug).
		Count(&count).Error; err != nil {
		return SlugAvailabilityResponse{}, fmt.Errorf("check CPO slug availability: %w", err)
	}

	return SlugAvailabilityResponse{
		Slug:      slug,
		Available: count == 0,
	}, nil
}

func (service *Service) UpdateProfile(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request UpdateProfileRequest,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	request = normalizeProfileRequest(request)
	if err := validateProfileRequest(request); err != nil {
		return View{}, err
	}

	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		updates := map[string]any{
			"business_name": request.BusinessName,
			"company_type":  request.CompanyType,
			"gstin":         request.GSTIN,
			"address":       request.Address,
			"city":          request.City,
			"state":         request.State,
			"pincode":       request.Pincode,
			"updated_at":    now,
		}
		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(updates).Error; err != nil {
			return mapWriteError(err, "update CPO profile")
		}
		record.BusinessName = request.BusinessName
		record.CompanyType = request.CompanyType
		record.GSTIN = request.GSTIN
		record.Address = request.Address
		record.City = request.City
		record.State = request.State
		record.Pincode = request.Pincode
		record.UpdatedAt = now
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PROFILE_UPDATED",
			models.JSONB{},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.profile_updated",
			models.JSONB{},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) Activate(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request LifecycleRequest,
) (View, error) {
	return service.transitionStatus(
		ctx,
		principal,
		cpoID,
		constants.CPOStatusActive,
		request.Reason,
	)
}

func (service *Service) Suspend(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request LifecycleRequest,
) (View, error) {
	return service.transitionStatus(
		ctx,
		principal,
		cpoID,
		constants.CPOStatusSuspended,
		request.Reason,
	)
}

func (service *Service) SetLiveAppID(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request SetAppIDRequest,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	appID := strings.ToLower(strings.TrimSpace(request.AppID))
	if !appIDPattern.MatchString(appID) || strings.HasPrefix(appID, dummyAppIDPrefix) {
		return View{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_cpo_app_id",
			Message: "The live app ID must be 16 to 100 lowercase URL-safe characters and cannot use the dummy prefix.",
		}
	}
	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(map[string]any{
				"app_id":            appID,
				"app_id_mode":       constants.CPOAppIDModeLive,
				"app_id_updated_at": now,
				"updated_at":        now,
			}).Error; err != nil {
			return mapWriteError(err, "set live CPO app ID")
		}
		record.AppID = appID
		record.AppIDMode = constants.CPOAppIDModeLive
		record.AppIDUpdatedAt = now
		record.UpdatedAt = now
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_APP_ID_SET_LIVE",
			models.JSONB{"app_id_mode": constants.CPOAppIDModeLive},
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.app_id_rotated",
			models.JSONB{"app_id_mode": constants.CPOAppIDModeLive},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) GetPrimaryAdmin(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if _, err := service.find(ctx, cpoID); err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) SetPrimaryAdmin(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request PrimaryAdminRequest,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if !service.mailEnabled {
		return PrimaryAdminView{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	request.FullName = strings.TrimSpace(request.FullName)
	request.Reason = strings.TrimSpace(request.Reason)
	if !validEmail(request.Email) {
		return PrimaryAdminView{}, invalid(
			"email",
			"Primary administrator email is invalid.",
		)
	}
	if request.FullName == "" || len(request.FullName) > 255 {
		return PrimaryAdminView{}, invalid(
			"full_name",
			"Full name is required and must not exceed 255 characters.",
		)
	}
	if err := validateReason(request.Reason); err != nil {
		return PrimaryAdminView{}, err
	}

	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-primary-admin:"+cpoID.String(),
		).Error; err != nil {
			return fmt.Errorf("serialize CPO primary administrator: %w", err)
		}
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
			"cpo-admin:"+request.Email,
		).Error; err != nil {
			return fmt.Errorf("serialize replacement administrator identity: %w", err)
		}

		var currentMembership models.CPOMembership
		currentResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND is_primary_admin", cpoID).
			First(&currentMembership)
		currentFound := currentResult.Error == nil
		if currentResult.Error != nil &&
			!errors.Is(currentResult.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load current primary administrator: %w", currentResult.Error)
		}

		var targetUser models.User
		identityCreated := false
		temporaryPassword := ""
		userResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", request.Email).
			First(&targetUser)
		switch {
		case userResult.Error == nil:
			if !targetUser.IsActive {
				return &auth.APIError{
					Status:  http.StatusConflict,
					Code:    "admin_identity_inactive",
					Message: "The administrator identity exists but is inactive.",
				}
			}
		case errors.Is(userResult.Error, gorm.ErrRecordNotFound):
			var err error
			temporaryPassword, err = generateTemporaryPassword()
			if err != nil {
				return err
			}
			passwordHash, err := security.HashPassword(temporaryPassword)
			if err != nil {
				return fmt.Errorf("hash replacement administrator password: %w", err)
			}
			targetUser = models.User{
				ID:                 uuid.New(),
				Email:              request.Email,
				PasswordHash:       passwordHash,
				FullName:           request.FullName,
				IsActive:           true,
				IsVerified:         false,
				MFAEnabled:         false,
				MustChangePassword: true,
				PasswordChangedAt:  now,
				CreatedAt:          now,
				UpdatedAt:          now,
			}
			if err := tx.Create(&targetUser).Error; err != nil {
				return mapWriteError(err, "create replacement administrator")
			}
			identityCreated = true
		default:
			return fmt.Errorf(
				"find replacement administrator identity: %w",
				userResult.Error,
			)
		}

		var targetMembership models.CPOMembership
		targetResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cpo_id = ? AND user_id = ?", cpoID, targetUser.ID).
			First(&targetMembership)
		targetFound := targetResult.Error == nil
		if targetResult.Error != nil &&
			!errors.Is(targetResult.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load replacement administrator membership: %w", targetResult.Error)
		}

		samePrimary := currentFound && currentMembership.UserID == targetUser.ID
		membershipChanged := !samePrimary ||
			currentMembership.Status != constants.MembershipStatusActive ||
			currentMembership.Role != constants.CPORoleAdmin
		if samePrimary && !membershipChanged {
			return nil
		}

		var previousUserID *uuid.UUID
		if currentFound {
			currentUserID := currentMembership.UserID
			previousUserID = &currentUserID
			if !samePrimary {
				if err := tx.Model(&models.CPOMembership{}).
					Where("id = ?", currentMembership.ID).
					Updates(map[string]any{
						"is_primary_admin": false,
						"status":           constants.MembershipStatusRevoked,
						"updated_at":       now,
					}).Error; err != nil {
					return fmt.Errorf("retire previous primary administrator: %w", err)
				}
				scope := constants.AuthScopeCPO
				if _, err := revokeCPOSessions(
					tx,
					cpoID,
					&scope,
					"PRIMARY_ADMIN_REPLACED",
					now,
					&currentMembership.UserID,
				); err != nil {
					return err
				}
			}
		}

		role := constants.CPORoleAdmin
		if targetFound {
			if err := tx.Model(&models.CPOMembership{}).
				Where("id = ?", targetMembership.ID).
				Updates(map[string]any{
					"role":             role,
					"status":           constants.MembershipStatusActive,
					"is_primary_admin": true,
					"updated_at":       now,
				}).Error; err != nil {
				return mapWriteError(err, "restore primary administrator membership")
			}
		} else {
			targetMembership = models.CPOMembership{
				ID:             uuid.New(),
				CPOID:          cpoID,
				UserID:         targetUser.ID,
				Role:           role,
				Status:         constants.MembershipStatusActive,
				IsPrimaryAdmin: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&targetMembership).Error; err != nil {
				return mapWriteError(err, "create replacement administrator membership")
			}
		}

		template := "CPO_MEMBERSHIP_ASSIGNED"
		payload := cmsmail.MessagePayload{
			RecipientName: targetUser.FullName,
			CPOName:       cpoRecord.BusinessName,
			CPOID:         cpoRecord.ID.String(),
			CPOAppID:      cpoRecord.AppID,
		}
		if identityCreated {
			template = "CPO_ADMIN_WELCOME"
			payload.TemporaryPassword = temporaryPassword
		}
		if samePrimary {
			template = "CPO_ONBOARDING_RESENT"
		}
		if err := service.outbox.EnqueueMessageWithContext(
			tx,
			targetUser.Email,
			template,
			payload,
			cmsmail.MessageContext{
				CPOID:  &cpoID,
				UserID: &targetUser.ID,
			},
		); err != nil {
			return err
		}

		action := "CPO_PRIMARY_ADMIN_REPLACED"
		changeType := "REPLACED"
		if samePrimary {
			action = "CPO_PRIMARY_ADMIN_RESTORED"
			changeType = "RESTORED"
		}
		details := models.JSONB{
			"new_user_id":      targetUser.ID,
			"identity_created": identityCreated,
			"change_type":      changeType,
			"reason":           request.Reason,
		}
		if previousUserID != nil {
			details["previous_user_id"] = *previousUserID
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			action,
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.primary_admin_changed",
			details,
		)
	})
	if err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) ResendPrimaryAdminOnboarding(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request ReasonRequest,
) (PrimaryAdminView, error) {
	if err := requirePlatform(principal); err != nil {
		return PrimaryAdminView{}, err
	}
	if !service.mailEnabled {
		return PrimaryAdminView{}, &auth.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "mail_unavailable",
			Message: "CPO administrator onboarding email is unavailable.",
		}
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if err := validateReason(request.Reason); err != nil {
		return PrimaryAdminView{}, err
	}
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		membership, user, err := loadPrimaryAdmin(tx, cpoID, true)
		if err != nil {
			return err
		}
		if !user.IsActive ||
			membership.Status != constants.MembershipStatusActive {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "primary_admin_unavailable",
				Message: "The primary administrator must be active before onboarding can be resent.",
			}
		}
		if err := service.outbox.EnqueueMessageWithContext(
			tx,
			user.Email,
			"CPO_ONBOARDING_RESENT",
			cmsmail.MessagePayload{
				RecipientName: user.FullName,
				CPOName:       cpoRecord.BusinessName,
				CPOID:         cpoRecord.ID.String(),
				CPOAppID:      cpoRecord.AppID,
			},
			cmsmail.MessageContext{
				CPOID:  &cpoID,
				UserID: &user.ID,
			},
		); err != nil {
			return err
		}
		details := models.JSONB{
			"primary_admin_user_id": user.ID,
			"reason":                request.Reason,
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PRIMARY_ADMIN_ONBOARDING_RESENT",
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.primary_admin_onboarding_resent",
			details,
		)
	})
	if err != nil {
		return PrimaryAdminView{}, err
	}
	return service.primaryAdminView(service.database.WithContext(ctx), cpoID)
}

func (service *Service) RevokeAdministrativeSessions(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	request ReasonRequest,
) (SessionRevocationResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return SessionRevocationResponse{}, err
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if err := validateReason(request.Reason); err != nil {
		return SessionRevocationResponse{}, err
	}
	var counts sessionRevocationCounts
	now := service.now()
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpoRecord models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			First(&cpoRecord, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		scope := constants.AuthScopeCPO
		var err error
		counts, err = revokeCPOSessions(
			tx,
			cpoID,
			&scope,
			"PLATFORM_CPO_ADMIN_REVOKED",
			now,
			nil,
		)
		if err != nil {
			return err
		}
		details := models.JSONB{
			"reason":                 request.Reason,
			"revoked_sessions":       counts.sessions,
			"revoked_refresh_tokens": counts.refreshTokens,
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_ADMIN_SESSIONS_REVOKED",
			details,
			now,
		); err != nil {
			return err
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			"platform.cpo.admin_sessions_revoked",
			details,
		)
	})
	if err != nil {
		return SessionRevocationResponse{}, err
	}
	return SessionRevocationResponse{
		RevokedSessions:      counts.sessions,
		RevokedRefreshTokens: counts.refreshTokens,
	}, nil
}

func (service *Service) transitionStatus(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	status constants.CPOStatus,
	reason string,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
		return View{}, err
	}
	reason = strings.TrimSpace(reason)
	if err := validateReason(reason); err != nil {
		return View{}, err
	}
	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}
		now := service.now()
		changed := record.Status != status
		previousStatus := record.Status
		if changed {
			if err := tx.Model(&models.CPO{}).
				Where("id = ?", cpoID).
				Updates(map[string]any{
					"status":                    status,
					"status_reason":             reason,
					"status_changed_at":         now,
					"status_changed_by_user_id": principal.UserID,
					"updated_at":                now,
				}).Error; err != nil {
				return fmt.Errorf("update CPO status: %w", err)
			}
			record.Status = status
			record.StatusReason = reason
			record.StatusChangedAt = now
			statusActorID := principal.UserID
			record.StatusChangedByUserID = &statusActorID
			record.UpdatedAt = now
		}
		if status == constants.CPOStatusSuspended {
			if _, err := revokeCPOSessions(
				tx,
				cpoID,
				nil,
				"CPO_SUSPENDED",
				now,
				nil,
			); err != nil {
				return err
			}
		}
		if !changed {
			return nil
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_STATUS_"+string(status),
			models.JSONB{
				"previous_status": previousStatus,
				"status":          status,
				"reason":          reason,
			},
			now,
		); err != nil {
			return err
		}
		eventType := "platform.cpo.activated"
		if status == constants.CPOStatusSuspended {
			eventType = "platform.cpo.suspended"
		}
		return service.emit(
			tx,
			principal.UserID,
			cpoID,
			eventType,
			models.JSONB{
				"previous_status": previousStatus,
				"status":          status,
				"reason":          reason,
			},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func loadPrimaryAdmin(
	database *gorm.DB,
	cpoID uuid.UUID,
	lock bool,
) (models.CPOMembership, models.User, error) {
	membershipQuery := database.Where(
		"cpo_id = ? AND is_primary_admin",
		cpoID,
	)
	if lock {
		membershipQuery = membershipQuery.Clauses(
			clause.Locking{Strength: "UPDATE"},
		)
	}
	var membership models.CPOMembership
	if err := membershipQuery.First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.CPOMembership{}, models.User{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "primary_admin_not_found",
				Message: "The CPO does not have a primary administrator.",
			}
		}
		return models.CPOMembership{}, models.User{},
			fmt.Errorf("load primary administrator membership: %w", err)
	}

	userQuery := database.Where("id = ?", membership.UserID)
	if lock {
		userQuery = userQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user models.User
	if err := userQuery.First(&user).Error; err != nil {
		return models.CPOMembership{}, models.User{},
			fmt.Errorf("load primary administrator identity: %w", err)
	}
	return membership, user, nil
}

func (service *Service) primaryAdminView(
	database *gorm.DB,
	cpoID uuid.UUID,
) (PrimaryAdminView, error) {
	membership, user, err := loadPrimaryAdmin(database, cpoID, false)
	if err != nil {
		return PrimaryAdminView{}, err
	}
	var latest models.MailOutbox
	mailResult := database.
		Where(
			"cpo_id = ? AND user_id = ? AND template IN ?",
			cpoID,
			user.ID,
			[]string{
				"CPO_ADMIN_WELCOME",
				"CPO_MEMBERSHIP_ASSIGNED",
				"CPO_ONBOARDING_RESENT",
			},
		).
		Order("created_at DESC, id DESC").
		First(&latest)
	var delivery *OnboardingDeliveryView
	switch {
	case mailResult.Error == nil:
		delivery = &OnboardingDeliveryView{
			JobID:     latest.ID,
			Template:  latest.Template,
			Status:    latest.Status,
			Attempts:  latest.Attempts,
			SentAt:    latest.SentAt,
			CreatedAt: latest.CreatedAt,
			UpdatedAt: latest.UpdatedAt,
		}
	case errors.Is(mailResult.Error, gorm.ErrRecordNotFound):
	default:
		return PrimaryAdminView{}, fmt.Errorf(
			"load primary administrator onboarding delivery: %w",
			mailResult.Error,
		)
	}
	return PrimaryAdminView{
		UserID:                   user.ID,
		Email:                    user.Email,
		FullName:                 user.FullName,
		Role:                     membership.Role,
		MembershipStatus:         membership.Status,
		IdentityActive:           user.IsActive,
		IdentityVerified:         user.IsVerified,
		MustChangePassword:       user.MustChangePassword,
		LastLoginAt:              user.LastLoginAt,
		LatestOnboardingDelivery: delivery,
	}, nil
}

func revokeCPOSessions(
	tx *gorm.DB,
	cpoID uuid.UUID,
	scope *constants.AuthScope,
	revokeReason string,
	now time.Time,
	userID *uuid.UUID,
) (sessionRevocationCounts, error) {
	counts := sessionRevocationCounts{}
	revokeCustomers := userID == nil && (scope == nil || *scope == constants.AuthScopeCustomer)
	if revokeCustomers {
		customerSessionIDs := tx.Model(&models.CustomerAuthSession{}).
			Select("id").Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
		customerRefreshResult := tx.Model(&models.CustomerAuthRefreshToken{}).
			Where("used_at IS NULL AND revoked_at IS NULL").
			Where("session_id IN (?)", customerSessionIDs).
			Update("revoked_at", now)
		if customerRefreshResult.Error != nil {
			return counts, fmt.Errorf("revoke CPO customer refresh tokens: %w", customerRefreshResult.Error)
		}
		customerSessionResult := tx.Model(&models.CustomerAuthSession{}).
			Where("cpo_id = ? AND revoked_at IS NULL", cpoID).
			Updates(map[string]any{"revoked_at": now, "revoke_reason": revokeReason})
		if customerSessionResult.Error != nil {
			return counts, fmt.Errorf("revoke CPO customer sessions: %w", customerSessionResult.Error)
		}
		counts.sessions += customerSessionResult.RowsAffected
		counts.refreshTokens += customerRefreshResult.RowsAffected
	}
	if scope != nil && *scope == constants.AuthScopeCustomer {
		return counts, nil
	}
	sessionIDs := tx.Model(&models.AuthSession{}).
		Select("id").
		Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
	if scope != nil {
		sessionIDs = sessionIDs.Where("scope = ?", *scope)
	}
	if userID != nil {
		sessionIDs = sessionIDs.Where("user_id = ?", *userID)
	}
	refreshResult := tx.Model(&models.AuthRefreshToken{}).
		Where("used_at IS NULL AND revoked_at IS NULL").
		Where("session_id IN (?)", sessionIDs).
		Update("revoked_at", now)
	if refreshResult.Error != nil {
		return counts, fmt.Errorf(
			"revoke CPO refresh tokens: %w",
			refreshResult.Error,
		)
	}

	sessionQuery := tx.Model(&models.AuthSession{}).
		Where("cpo_id = ? AND revoked_at IS NULL", cpoID)
	if scope != nil {
		sessionQuery = sessionQuery.Where("scope = ?", *scope)
	}
	if userID != nil {
		sessionQuery = sessionQuery.Where("user_id = ?", *userID)
	}
	sessionResult := sessionQuery.Updates(map[string]any{
		"revoked_at":    now,
		"revoke_reason": revokeReason,
	})
	if sessionResult.Error != nil {
		return counts, fmt.Errorf(
			"revoke CPO sessions: %w",
			sessionResult.Error,
		)
	}
	counts.sessions += sessionResult.RowsAffected
	counts.refreshTokens += refreshResult.RowsAffected
	return counts, nil
}

func (service *Service) emit(
	tx *gorm.DB,
	actorUserID uuid.UUID,
	cpoID uuid.UUID,
	eventType string,
	data models.JSONB,
) error {
	if service.events == nil {
		return nil
	}
	resourceID := cpoID.String()
	_, err := service.events.Emit(tx, platformops.EventInput{
		Type:         eventType,
		ActorUserID:  &actorUserID,
		ResourceType: "CPO",
		ResourceID:   &resourceID,
		Data:         data,
	})
	return err
}

func (service *Service) find(ctx context.Context, cpoID uuid.UUID) (models.CPO, error) {
	var record models.CPO
	if err := service.database.WithContext(ctx).First(&record, "id = ?", cpoID).Error; err != nil {
		return models.CPO{}, mapNotFound(err)
	}
	return record, nil
}

func normalizeCreateRequest(request CreateRequest) CreateRequest {
	request.Slug = normalizeSlug(request.Slug)
	request.BusinessName = strings.TrimSpace(request.BusinessName)
	request.GSTIN = strings.ToUpper(strings.TrimSpace(request.GSTIN))
	request.Address = strings.TrimSpace(request.Address)
	request.City = strings.TrimSpace(request.City)
	request.State = strings.TrimSpace(request.State)
	request.Pincode = strings.TrimSpace(request.Pincode)
	request.Admin.Email = strings.ToLower(strings.TrimSpace(request.Admin.Email))
	request.Admin.FullName = strings.TrimSpace(request.Admin.FullName)
	return request
}

func normalizeProfileRequest(request UpdateProfileRequest) UpdateProfileRequest {
	request.BusinessName = strings.TrimSpace(request.BusinessName)
	request.GSTIN = strings.ToUpper(strings.TrimSpace(request.GSTIN))
	request.Address = strings.TrimSpace(request.Address)
	request.City = strings.TrimSpace(request.City)
	request.State = strings.TrimSpace(request.State)
	request.Pincode = strings.TrimSpace(request.Pincode)
	return request
}

func validateProfileRequest(request UpdateProfileRequest) error {
	if request.BusinessName == "" || len(request.BusinessName) > 255 {
		return invalid(
			"business_name",
			"Business name is required and must not exceed 255 characters.",
		)
	}
	if !request.CompanyType.Valid() {
		return invalid(
			"company_type",
			"Company type must be INDIVIDUAL or COMPANY.",
		)
	}
	if !gstinPattern.MatchString(request.GSTIN) {
		return invalid(
			"gstin",
			"GSTIN is required and must contain 15 uppercase letters or digits.",
		)
	}
	if request.Address == "" || len(request.Address) > 5000 {
		return invalid("address", "Address is required and must not exceed 5000 characters.")
	}
	if request.City == "" || len(request.City) > 100 {
		return invalid("city", "City is required and must not exceed 100 characters.")
	}
	if request.State == "" || len(request.State) > 100 {
		return invalid("state", "State is required and must not exceed 100 characters.")
	}
	if request.Pincode == "" || len(request.Pincode) > 10 {
		return invalid("pincode", "Pincode is required and must not exceed 10 characters.")
	}
	return nil
}

func validateReason(reason string) error {
	length := len(strings.TrimSpace(reason))
	if length < minReasonLength || length > maxReasonLength {
		return invalid(
			"reason",
			"Reason must contain 3 to 500 characters.",
		)
	}
	return nil
}

func validateCreateRequest(request CreateRequest) error {
	if err := validateSlug(request.Slug); err != nil {
		return err
	}
	if request.BusinessName == "" || len(request.BusinessName) > 255 {
		return invalid("business_name", "Business name is required and must not exceed 255 characters.")
	}
	if !request.CompanyType.Valid() {
		return invalid("company_type", "Company type must be INDIVIDUAL or COMPANY.")
	}
	if !gstinPattern.MatchString(request.GSTIN) {
		return invalid("gstin", "GSTIN is required and must contain 15 uppercase letters or digits.")
	}
	if request.Address == "" || len(request.Address) > 5000 {
		return invalid("address", "Address is required and must not exceed 5000 characters.")
	}
	if request.City == "" || len(request.City) > 100 {
		return invalid("city", "City is required and must not exceed 100 characters.")
	}
	if request.State == "" || len(request.State) > 100 {
		return invalid("state", "State is required and must not exceed 100 characters.")
	}
	if request.Pincode == "" || len(request.Pincode) > 10 {
		return invalid("pincode", "Pincode is required and must not exceed 10 characters.")
	}
	if !validEmail(request.Admin.Email) {
		return invalid("admin.email", "Administrator email is invalid.")
	}
	if request.Admin.FullName == "" || len(request.Admin.FullName) > 255 {
		return invalid("admin.full_name", "Administrator full name is required and must not exceed 255 characters.")
	}
	return nil
}

func normalizeSlug(candidate string) string {
	return strings.ToLower(strings.TrimSpace(candidate))
}

func validateSlug(slug string) error {
	if len(slug) > 80 || !slugPattern.MatchString(slug) {
		return invalid("slug", "Slug must contain lowercase words separated by single hyphens.")
	}
	return nil
}

func validEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	address, err := netmail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func generateTemporaryPassword() (string, error) {
	token, err := security.RandomToken(18)
	if err != nil {
		return "", err
	}
	return "Tmp-" + token, nil
}

func requirePlatform(principal auth.Principal) error {
	if principal.Scope != constants.AuthScopePlatform {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "Platform superadmin access is required.",
		}
	}
	return nil
}

func invalid(field, message string) error {
	return &auth.APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_" + strings.ReplaceAll(field, ".", "_"),
		Message: message,
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "cpo_not_found",
			Message: "The CPO was not found.",
		}
	}
	return fmt.Errorf("load CPO: %w", err)
}

func mapWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		switch postgresError.ConstraintName {
		case "uq_cpos_slug_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_slug_conflict",
				Message: "The CPO slug is already in use.",
			}
		case "uq_cpos_gstin_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_gstin_conflict",
				Message: "The GSTIN is already assigned to another CPO.",
			}
		case "uq_cpos_app_id":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_app_id_conflict",
				Message: "The CPO app ID is already in use.",
			}
		case "uq_users_email_normalized":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "admin_identity_conflict",
				Message: "An administrator identity with this email was created concurrently. Retry the request.",
			}
		case "uq_cpo_membership":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_admin_membership_conflict",
				Message: "The administrator already has a membership for this CPO.",
			}
		case "uq_cpo_memberships_primary_admin":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "cpo_primary_admin_conflict",
				Message: "The CPO already has a primary administrator.",
			}
		}
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "cpo_conflict",
			Message: "The CPO slug, GSTIN, app ID, or administrator membership already exists.",
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func writeAudit(
	tx *gorm.DB,
	actorID uuid.UUID,
	cpoID uuid.UUID,
	action string,
	details models.JSONB,
	now time.Time,
) error {
	record := models.AuditLog{
		ID:        uuid.New(),
		CPOID:     &cpoID,
		UserID:    &actorID,
		Action:    action,
		Entity:    "CPO",
		EntityID:  &cpoID,
		Details:   details,
		CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("audit CPO operation: %w", err)
	}
	return nil
}

func view(record models.CPO) View {
	return View{
		ID:                    record.ID,
		Slug:                  record.Slug,
		BusinessName:          record.BusinessName,
		CompanyType:           record.CompanyType,
		GSTIN:                 record.GSTIN,
		Address:               record.Address,
		City:                  record.City,
		State:                 record.State,
		Pincode:               record.Pincode,
		Status:                record.Status,
		StatusReason:          record.StatusReason,
		StatusChangedAt:       record.StatusChangedAt,
		StatusChangedByUserID: record.StatusChangedByUserID,
		AppID:                 record.AppID,
		AppIDMode:             record.AppIDMode,
		AppIDUpdatedAt:        record.AppIDUpdatedAt,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

var chargerIDPattern = regexp.MustCompile(`^[a-z0-9]{6}$`)

func (service *Service) GetAdminProfile(
	ctx context.Context,
	principal auth.Principal,
) (AdminProfileView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return AdminProfileView{}, err
	}
	var user models.User
	if err := service.database.WithContext(ctx).
		First(&user, "id = ? AND is_active = true", principal.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AdminProfileView{}, &auth.APIError{
				Status:  http.StatusUnauthorized,
				Code:    "unauthorized",
				Message: "The authenticated identity is no longer active.",
			}
		}
		return AdminProfileView{}, fmt.Errorf("load CPO administrator profile: %w", err)
	}
	return adminProfileView(user, *principal.CPOID), nil
}

func (service *Service) UpdateAdminProfile(
	ctx context.Context,
	principal auth.Principal,
	request UpdateAdminProfileRequest,
) (AdminProfileView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return AdminProfileView{}, err
	}
	request.FullName = trimOptionalString(request.FullName)
	request.Phone = trimOptionalString(request.Phone)
	if request.FullName == nil && request.Phone == nil {
		return AdminProfileView{}, invalid(
			"admin_profile",
			"At least one administrator profile field must be supplied.",
		)
	}
	if request.FullName != nil && (*request.FullName == "" || len(*request.FullName) > 255) {
		return AdminProfileView{}, invalid(
			"full_name",
			"Full name is required and must not exceed 255 characters.",
		)
	}
	if request.Phone != nil && len(*request.Phone) > 32 {
		return AdminProfileView{}, invalid("phone", "Phone must not exceed 32 characters.")
	}

	var user models.User
	cpoID := *principal.CPOID
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&user, "id = ? AND is_active = true", principal.UserID).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		changedFields := make([]string, 0, 2)
		if request.FullName != nil {
			updates["full_name"] = *request.FullName
			user.FullName = *request.FullName
			changedFields = append(changedFields, "full_name")
		}
		if request.Phone != nil {
			if *request.Phone == "" {
				updates["phone"] = nil
				user.Phone = nil
			} else {
				updates["phone"] = *request.Phone
				user.Phone = request.Phone
			}
			changedFields = append(changedFields, "phone")
		}
		now := service.now()
		updates["updated_at"] = now
		user.UpdatedAt = now
		if err := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update CPO administrator profile: %w", err)
		}
		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_ADMIN_PROFILE_UPDATED",
			models.JSONB{"changed_fields": changedFields},
			now,
		)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AdminProfileView{}, &auth.APIError{
				Status:  http.StatusUnauthorized,
				Code:    "unauthorized",
				Message: "The authenticated identity is no longer active.",
			}
		}
		return AdminProfileView{}, err
	}
	return adminProfileView(user, cpoID), nil
}

func adminProfileView(user models.User, cpoID uuid.UUID) AdminProfileView {
	return AdminProfileView{
		UserID:     user.ID,
		CPOID:      cpoID,
		Email:      user.Email,
		FullName:   user.FullName,
		Phone:      user.Phone,
		Role:       constants.CPORoleAdmin,
		IsVerified: user.IsVerified,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
}

func cpoUserView(
	user models.User,
	cpoID uuid.UUID,
	membership models.CPOMembership,
) CPOUserView {
	view := CPOUserView{
		ID:         user.ID,
		CPOID:      cpoID,
		Email:      user.Email,
		FullName:   user.FullName,
		Phone:      user.Phone,
		IsActive:   user.IsActive,
		IsVerified: user.IsVerified,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
	role := membership.Role
	view.Role = &role
	status := membership.Status
	view.MembershipStatus = &status
	return view
}

func (service *Service) GetUser(
	ctx context.Context,
	principal auth.Principal,
	userID uuid.UUID,
) (CPOUserView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return CPOUserView{}, err
	}

	cpoID := *principal.CPOID
	var membership models.CPOMembership
	membershipErr := service.database.WithContext(ctx).
		Where("cpo_id = ? AND user_id = ?", cpoID, userID).
		First(&membership).Error
	if membershipErr != nil && !errors.Is(membershipErr, gorm.ErrRecordNotFound) {
		return CPOUserView{}, fmt.Errorf("load CPO membership: %w", membershipErr)
	}

	if membershipErr != nil {
		return CPOUserView{}, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "user_not_found",
			Message: "The user was not found for this CPO.",
		}
	}

	var user models.User
	if err := service.database.WithContext(ctx).
		Where("id = ?", userID).
		Where(`EXISTS (
            SELECT 1 FROM cpo_memberships
            WHERE cpo_id = ? AND user_id = users.id
	        )`, cpoID).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOUserView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "user_not_found",
				Message: "The user was not found for this CPO.",
			}
		}
		return CPOUserView{}, fmt.Errorf("load CPO user: %w", err)
	}

	return cpoUserView(user, cpoID, membership), nil
}

func (service *Service) GetOrganization(
	ctx context.Context,
	principal auth.Principal,
) (OrganizationView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return OrganizationView{}, err
	}
	record, err := service.find(ctx, *principal.CPOID)
	if err != nil {
		return OrganizationView{}, err
	}
	return organizationView(record), nil
}

func organizationView(record models.CPO) OrganizationView {
	return OrganizationView{
		ID:              record.ID,
		Slug:            record.Slug,
		BusinessName:    record.BusinessName,
		CompanyType:     record.CompanyType,
		GSTIN:           record.GSTIN,
		Address:         record.Address,
		City:            record.City,
		State:           record.State,
		Pincode:         record.Pincode,
		Status:          record.Status,
		StatusChangedAt: record.StatusChangedAt,
		AppID:           record.AppID,
		AppIDMode:       record.AppIDMode,
		AppIDUpdatedAt:  record.AppIDUpdatedAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

type subscriptionData struct {
	models.CPOSubscription
	PlanName        string
	PlanDescription string
	Currency        string
	PriceMinor      int64
	BillingInterval string
	IntervalCount   int
	TrialDays       int
}

func (service *Service) GetSubscription(
	ctx context.Context,
	principal auth.Principal,
) (CPOSubscriptionView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return CPOSubscriptionView{}, err
	}
	cpoID := *principal.CPOID
	var result subscriptionData
	err := service.database.WithContext(ctx).
		Model(&models.CPOSubscription{}).
		Select("cpo_subscriptions.*, p.name as plan_name, p.description as plan_description, pv.currency, pv.price_minor, pv.billing_interval, pv.interval_count, pv.trial_days").
		Joins("inner join subscription_plan_versions pv on pv.id = cpo_subscriptions.plan_version_id").
		Joins("inner join subscription_plans p on p.id = pv.plan_id").
		Where("cpo_subscriptions.cpo_id = ? AND cpo_subscriptions.status IN ?", cpoID, currentCPOSubscriptionStatuses).
		First(&result).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOSubscriptionView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "subscription_not_found",
				Message: "No subscription found for this CPO.",
			}
		}
		return CPOSubscriptionView{}, fmt.Errorf("error fetching subscription: %w", err)
	}

	view := CPOSubscriptionView{
		ID:                    result.ID,
		Status:                result.Status,
		StartsAt:              result.StartsAt,
		TrialEndsAt:           result.TrialEndsAt,
		CurrentPeriodStartsAt: result.CurrentPeriodStartsAt,
		CurrentPeriodEndsAt:   result.CurrentPeriodEndsAt,
		CancelAtPeriodEnd:     result.CancelAtPeriodEnd,
		CancelledAt:           result.CancelledAt,
		EndedAt:               result.EndedAt,
		Plan: CPOSubscriptionPlanView{
			Name:            result.PlanName,
			Description:     result.PlanDescription,
			Currency:        result.Currency,
			PriceMinor:      result.PriceMinor,
			BillingInterval: result.BillingInterval,
			IntervalCount:   result.IntervalCount,
			TrialDays:       result.TrialDays,
		},
	}

	return view, nil
}
func (service *Service) CreateCharger(
	ctx *gin.Context,
	principal auth.Principal,
) (ChargerResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerResponse{}, err
	}

	cpoID := *principal.CPOID
	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	var request CreateChargerRequest
	if err := json.Unmarshal([]byte(ctx.Request.FormValue("data")), &request); err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	request = normalizeCreateChargerRequest(request)
	if err := validateCreateChargerRequest(request); err != nil {
		return ChargerResponse{}, err
	}

	var vendor *string
	if request.Vendor != "" {
		v := request.Vendor
		vendor = &v
	}

	var model *string
	if request.Model != "" {
		m := request.Model
		model = &m
	}

	var record models.Charger
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if request.HubID != nil {
			var hub models.Hub
			if err := tx.First(&hub, "id = ? AND cpo_id = ?", *request.HubID, cpoID).Error; err != nil {
				return mapHubNotFound(err)
			}
		}

		chargerID, err := generateUniqueChargerIDTx(tx)
		if err != nil {
			return err
		}

		ocppIdentity := chargerID

		file, err := ctx.FormFile("charger_image")
		var chargerImagePath string
		if err == nil {
			filename := uuid.New().String() + filepath.Ext(file.Filename)
			uploads := "uploads"
			if _, err := os.Stat(uploads); os.IsNotExist(err) {
				if err := os.Mkdir(uploads, 0755); err != nil {
					return err
				}
			}

			chargerImagePath = filepath.Join(uploads, filename)
			if err := ctx.SaveUploadedFile(file, chargerImagePath); err != nil {
				return err
			}
		}

		now := service.now()
		record = models.Charger{
			ID:                  uuid.New(),
			CPOID:               cpoID,
			HubID:               request.HubID,
			ChargerID:           chargerID,
			OCPPIdentity:        ocppIdentity,
			Vendor:              vendor,
			Model:               model,
			SerialNumber:        request.SerialNumber,
			MaxPowerKW:          request.MaxPowerKW,
			ChargerName:         request.ChargerName,
			ChargerHostName:     request.ChargerHostName,
			ChargerHostPhoneNo:  request.ChargerHostPhoneNo,
			ChargerType:         request.ChargerType,
			Segment:             request.Segment,
			SubSegment:          request.SubSegment,
			ChargerImage:        chargerImagePath,
			ChargerUseType:      request.ChargerUseType,
			NumberOfConnectors:  request.NumberOfConnectors,
			Parking:             request.Parking,
			Protocol:            request.Protocol,
			TwentyFourSevenOpen: request.TwentyFourSevenOpen,
			Status:              constants.ChargerStatusInactive,
			OCPPVersion:         "1.6J",
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapChargerWriteError(err, "create charger")
		}

		for _, connector := range request.Connectors {
			connectorType := strings.TrimSpace(connector.ConnectorType)
			connectorRecord := models.Connector{
				ID:                     uuid.New(),
				CPOID:                  cpoID,
				ChargerID:              record.ID,
				ConnectorNumber:        connector.ConnectorNumber,
				ConnectorType:          connectorType,
				ConnectorTotalCapacity: connector.ConnectorTotalCapacity,
				Status:                 constants.ChargerStatusActive,
				CreatedAt:              now,
				UpdatedAt:              now,
			}

			if err := tx.Create(&connectorRecord).Error; err != nil {
				return mapConnectorWriteError(err, "create connector")
			}
			record.Connectors = append(record.Connectors, connectorRecord)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_CREATED",
			models.JSONB{
				"charger_id":    record.ChargerID,
				"ocpp_identity": record.OCPPIdentity,
				"hub_id":        record.HubID,
				"connectors":    len(record.Connectors),
			},
			now,
		)
	})
	if err != nil {
		return ChargerResponse{}, err
	}

	if record.HubID != nil {
		var hub models.Hub
		if err := service.database.WithContext(ctx).First(&hub, "id = ?", *record.HubID).Error; err == nil {
			record.Hub = &hub
		}
	}

	return service.chargerView(record, principal), nil
}

func (service *Service) CreateHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, &hubID, request.ChargerID, request.UserGroupID); err != nil {
			return err
		}
		if request.GSTID != nil {
			var gst models.GST
			if err := tx.First(&gst, "id = ? AND cpo_id = ?", *request.GSTID, cpoID).Error; err != nil {
				return mapGSTNotFound(err)
			}
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         hubID,
			ChargerID:     request.ChargerID,
			GSTID:         request.GSTID,
			UserGroupID:   request.UserGroupID,
			PricePerKWh:   request.PricePerKWh,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create hub tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_TARIFF_CREATED",
			models.JSONB{
				"tariff_id": record.ID,
				"hub_id":    record.HubID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil
}

func (service *Service) ListHubTariffs(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND hub_id = ?", *principal.CPOID, hubID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list hub tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND hub_id = ? AND id = ?", *principal.CPOID, hubID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateHubTariff(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND hub_id = ? AND id = ?", cpoID, hubID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if err := service.validateTariffScope(tx, cpoID, &hubID, request.ChargerID, request.UserGroupID); err != nil {
			return err
		}

		if request.GSTID != nil {
			updates["gst_id"] = request.GSTID
			record.GSTID = request.GSTID
			changedFields["gst_id"] = request.GSTID
		}
		if request.UserGroupID != nil {
			updates["user_group_id"] = request.UserGroupID
			record.UserGroupID = request.UserGroupID
			changedFields["user_group_id"] = request.UserGroupID
		}
		if request.ChargerID != nil {
			updates["charger_id"] = request.ChargerID
			record.ChargerID = request.ChargerID
			changedFields["charger_id"] = request.ChargerID
		}
		if request.PricePerKWh != nil {
			updates["price_per_kwh"] = *request.PricePerKWh
			record.PricePerKWh = *request.PricePerKWh
			changedFields["price_per_kwh"] = *request.PricePerKWh
		}
		if request.IdleFeePerMin != nil {
			updates["idle_fee_per_min"] = *request.IdleFeePerMin
			record.IdleFeePerMin = *request.IdleFeePerMin
			changedFields["idle_fee_per_min"] = *request.IdleFeePerMin
		}
		if request.Currency != nil {
			updates["currency"] = *request.Currency
			record.Currency = *request.Currency
			changedFields["currency"] = *request.Currency
		}
		if request.StartDate != nil {
			updates["start_date"] = request.StartDate
			record.StartDate = request.StartDate
			changedFields["start_date"] = request.StartDate
		}
		if request.EndDate != nil {
			updates["end_date"] = request.EndDate
			record.EndDate = request.EndDate
			changedFields["end_date"] = request.EndDate
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}
		if request.TariffType != nil {
			updates["tariff_type"] = *request.TariffType
			record.TariffType = request.TariffType
			changedFields["tariff_type"] = *request.TariffType
		}
		if request.PriceType != nil {
			updates["price_type"] = *request.PriceType
			record.PriceType = request.PriceType
			changedFields["price_type"] = *request.PriceType
		}
		if request.Units != nil {
			updates["units"] = *request.Units
			record.Units = request.Units
			changedFields["units"] = *request.Units
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil
}

func (service *Service) CreateChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	var charger models.Charger
	if err := service.database.WithContext(ctx).First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
		return TariffView{}, mapChargerNotFound(err)
	}
	if charger.HubID == nil {
		return TariffView{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "charger_not_in_hub",
			Message: "A tariff can only be created for a charger that belongs to a hub.",
		}
	}

	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, charger.HubID, &chargerID, request.UserGroupID); err != nil {
			return err
		}
		if request.GSTID != nil {
			var gst models.GST
			if err := tx.First(&gst, "id = ? AND cpo_id = ?", *request.GSTID, cpoID).Error; err != nil {
				return mapGSTNotFound(err)
			}
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         *charger.HubID,
			ChargerID:     &chargerID,
			GSTID:         request.GSTID,
			UserGroupID:   request.UserGroupID,
			PricePerKWh:   request.PricePerKWh,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create charger tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_TARIFF_CREATED",
			models.JSONB{
				"tariff_id":  record.ID,
				"charger_id": record.ChargerID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) ListChargerTariffs(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list charger tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND charger_id = ? AND id = ?", *principal.CPOID, chargerID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateChargerTariff(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND charger_id = ? AND id = ?", cpoID, chargerID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if err := service.validateTariffScope(tx, cpoID, &record.HubID, &chargerID, request.UserGroupID); err != nil {
			return err
		}

		if request.GSTID != nil {
			updates["gst_id"] = request.GSTID
			record.GSTID = request.GSTID
			changedFields["gst_id"] = request.GSTID
		}
		if request.UserGroupID != nil {
			updates["user_group_id"] = request.UserGroupID
			record.UserGroupID = request.UserGroupID
			changedFields["user_group_id"] = request.UserGroupID
		}
		if request.PricePerKWh != nil {
			updates["price_per_kwh"] = *request.PricePerKWh
			record.PricePerKWh = *request.PricePerKWh
			changedFields["price_per_kwh"] = *request.PricePerKWh
		}
		if request.IdleFeePerMin != nil {
			updates["idle_fee_per_min"] = *request.IdleFeePerMin
			record.IdleFeePerMin = *request.IdleFeePerMin
			changedFields["idle_fee_per_min"] = *request.IdleFeePerMin
		}
		if request.Currency != nil {
			updates["currency"] = *request.Currency
			record.Currency = *request.Currency
			changedFields["currency"] = *request.Currency
		}
		if request.StartDate != nil {
			updates["start_date"] = request.StartDate
			record.StartDate = request.StartDate
			changedFields["start_date"] = request.StartDate
		}
		if request.EndDate != nil {
			updates["end_date"] = request.EndDate
			record.EndDate = request.EndDate
			changedFields["end_date"] = request.EndDate
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}
		if request.TariffType != nil {
			updates["tariff_type"] = *request.TariffType
			record.TariffType = request.TariffType
			changedFields["tariff_type"] = *request.TariffType
		}
		if request.PriceType != nil {
			updates["price_type"] = *request.PriceType
			record.PriceType = request.PriceType
			changedFields["price_type"] = *request.PriceType
		}
		if request.Units != nil {
			updates["units"] = *request.Units
			record.Units = request.Units
			changedFields["units"] = *request.Units
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) CreateUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request CreateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateTariffRequest(request)
	if err := validateCreateTariffRequest(request); err != nil {
		return TariffView{}, err
	}
	if request.HubID == uuid.Nil {
		return TariffView{}, invalid("hub_id", "Hub ID is required for a user group tariff.")
	}

	var record models.Tariff
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.validateTariffScope(tx, cpoID, &request.HubID, request.ChargerID, &userGroupID); err != nil {
			return err
		}
		if request.GSTID != nil {
			var gst models.GST
			if err := tx.First(&gst, "id = ? AND cpo_id = ?", *request.GSTID, cpoID).Error; err != nil {
				return mapGSTNotFound(err)
			}
		}

		now := service.now()
		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.Tariff{
			ID:            uuid.New(),
			CPOID:         cpoID,
			HubID:         request.HubID,
			ChargerID:     request.ChargerID,
			GSTID:         request.GSTID,
			UserGroupID:   &userGroupID,
			PricePerKWh:   request.PricePerKWh,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			StartDate:     request.StartDate,
			EndDate:       request.EndDate,
			TariffType:    request.TariffType,
			PriceType:     request.PriceType,
			Units:         request.Units,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return service.handleTariffError("create user group tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_TARIFF_CREATED",
			models.JSONB{
				"tariff_id":     record.ID,
				"user_group_id": record.UserGroupID,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) ListUserGroupTariffs(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	query TenantListQuery,
) (*TariffListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return nil, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return nil, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND user_group_id = ?", *principal.CPOID, userGroupID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []*models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list user group tariffs: %w", err)
	}

	hasNext := len(records) > query.Limit
	if hasNext {
		records = records[:query.Limit]
	}

	views := make([]TariffView, len(records))
	for i, record := range records {
		views[i] = service.tariffView(record)
	}

	response := TariffListResponse{Tariffs: views, HasMore: hasNext}
	if hasNext && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}

	return &response, nil
}

func (service *Service) GetUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND user_group_id = ? AND id = ?", *principal.CPOID, userGroupID, tariffID).Error; err != nil {
		return TariffView{}, service.handleTariffError("load tariff", err)
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) UpdateUserGroupTariff(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	tariffID uuid.UUID,
	request UpdateTariffRequest,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	request = normalizeUpdateTariffRequest(request)
	if err := validateUpdateTariffRequest(request); err != nil {
		return TariffView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Tariff

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND user_group_id = ? AND id = ?", cpoID, userGroupID, tariffID).Error; err != nil {
			return service.handleTariffError("load tariff", err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		hubID := record.HubID
		if request.HubID != nil {
			hubID = *request.HubID
		}
		if err := service.validateTariffScope(tx, cpoID, &hubID, request.ChargerID, &userGroupID); err != nil {
			return err
		}

		if request.HubID != nil {
			updates["hub_id"] = request.HubID
			record.HubID = *request.HubID
			changedFields["hub_id"] = request.HubID
		}
		if request.ChargerID != nil {
			updates["charger_id"] = request.ChargerID
			record.ChargerID = request.ChargerID
			changedFields["charger_id"] = request.ChargerID
		}
		if request.GSTID != nil {
			updates["gst_id"] = request.GSTID
			record.GSTID = request.GSTID
			changedFields["gst_id"] = request.GSTID
		}
		if request.PricePerKWh != nil {
			updates["price_per_kwh"] = *request.PricePerKWh
			record.PricePerKWh = *request.PricePerKWh
			changedFields["price_per_kwh"] = *request.PricePerKWh
		}
		if request.IdleFeePerMin != nil {
			updates["idle_fee_per_min"] = *request.IdleFeePerMin
			record.IdleFeePerMin = *request.IdleFeePerMin
			changedFields["idle_fee_per_min"] = *request.IdleFeePerMin
		}
		if request.Currency != nil {
			updates["currency"] = *request.Currency
			record.Currency = *request.Currency
			changedFields["currency"] = *request.Currency
		}
		if request.StartDate != nil {
			updates["start_date"] = request.StartDate
			record.StartDate = request.StartDate
			changedFields["start_date"] = request.StartDate
		}
		if request.EndDate != nil {
			updates["end_date"] = request.EndDate
			record.EndDate = request.EndDate
			changedFields["end_date"] = request.EndDate
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}
		if request.TariffType != nil {
			updates["tariff_type"] = *request.TariffType
			record.TariffType = request.TariffType
			changedFields["tariff_type"] = *request.TariffType
		}
		if request.PriceType != nil {
			updates["price_type"] = *request.PriceType
			record.PriceType = request.PriceType
			changedFields["price_type"] = *request.PriceType
		}
		if request.Units != nil {
			updates["units"] = *request.Units
			record.Units = request.Units
			changedFields["units"] = *request.Units
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one tariff field must be supplied.",
			}
		}
		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := validateTariffDateRange(record.StartDate, record.EndDate); err != nil {
			return err
		}
		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return service.handleTariffError("update tariff", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_TARIFF_UPDATED",
			models.JSONB{
				"tariff_id":      record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return TariffView{}, err
	}

	return service.tariffView(&record), nil // Fix: Call as method
}

func (service *Service) validateTariffScope(
	tx *gorm.DB,
	cpoID uuid.UUID,
	hubID *uuid.UUID,
	chargerID *uuid.UUID,
	userGroupID *uuid.UUID,
) error {
	if hubID != nil {
		var hub models.Hub
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *hubID).
			First(&hub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "hub_not_found",
					Message: "The hub was not found.",
				}
			}
			return fmt.Errorf("validate tariff scope (hub): %w", err)
		}
	}

	if chargerID != nil {
		var charger models.Charger
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *chargerID).
			First(&charger).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "charger_not_found",
					Message: "The charger was not found in the specified hub.",
				}
			}
			return fmt.Errorf("validate tariff scope (charger): %w", err)
		}
		if hubID != nil && (charger.HubID == nil || *charger.HubID != *hubID) {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_not_in_hub",
				Message: "The charger was not found in the specified hub.",
			}
		}
	}

	if userGroupID != nil {
		var userGroup models.UserGroup
		if err := tx.
			Where("cpo_id = ? AND id = ?", cpoID, *userGroupID).
			First(&userGroup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "user_group_not_found",
					Message: "The user group was not found.",
				}
			}
			return fmt.Errorf("validate tariff scope (user_group): %w", err)
		}
	}

	return nil
}

func (service *Service) GetCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
) (ChargerResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerResponse{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	var record models.Charger
	if err := service.database.WithContext(ctx).
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return ChargerResponse{}, mapChargerNotFound(err)
	}
	return service.chargerView(record, principal), nil
}

func (service *Service) UpdateCharger(
	ctx *gin.Context,
	principal auth.Principal,
	chargerID string,
) (ChargerResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerResponse{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	err := ctx.Request.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	var request UpdateChargerRequest
	if err := json.Unmarshal([]byte(ctx.Request.FormValue("data")), &request); err != nil {
		return ChargerResponse{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "The request body is invalid.",
		}
	}

	request = normalizeUpdateChargerRequest(request)
	if err := validateUpdateChargerRequest(request); err != nil {
		return ChargerResponse{}, err
	}

	cpoID := *principal.CPOID
	var record models.Charger

	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("connector_number ASC")
			}).
			Preload("Hub").
			First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.HubID != nil {
			var hub models.Hub
			if err := tx.First(&hub, "id = ? AND cpo_id = ?", *request.HubID, cpoID).Error; err != nil {
				return mapHubNotFound(err)
			}
			updates["hub_id"] = *request.HubID
			record.HubID = request.HubID
			changedFields["hub_id"] = *request.HubID
		}
		if request.Vendor != nil {
			updates["vendor"] = request.Vendor
			record.Vendor = request.Vendor
			changedFields["vendor"] = *request.Vendor
		}
		if request.Model != nil {
			updates["model"] = request.Model
			record.Model = request.Model
			changedFields["model"] = *request.Model
		}
		if request.SerialNumber != nil {
			updates["serial_number"] = *request.SerialNumber
			record.SerialNumber = *request.SerialNumber
			changedFields["serial_number"] = *request.SerialNumber
		}
		if request.MaxPowerKW != nil {
			updates["max_power_kw"] = *request.MaxPowerKW
			record.MaxPowerKW = *request.MaxPowerKW
			changedFields["max_power_kw"] = *request.MaxPowerKW
		}
		if request.ChargerName != nil {
			updates["charger_name"] = *request.ChargerName
			record.ChargerName = *request.ChargerName
			changedFields["charger_name"] = *request.ChargerName
		}
		if request.ChargerHostName != nil {
			updates["charger_host_name"] = *request.ChargerHostName
			record.ChargerHostName = *request.ChargerHostName
			changedFields["charger_host_name"] = *request.ChargerHostName
		}
		if request.ChargerHostPhoneNo != nil {
			updates["charger_host_phone_no"] = *request.ChargerHostPhoneNo
			record.ChargerHostPhoneNo = *request.ChargerHostPhoneNo
			changedFields["charger_host_phone_no"] = *request.ChargerHostPhoneNo
		}
		if request.ChargerType != nil {
			updates["charger_type"] = *request.ChargerType
			record.ChargerType = *request.ChargerType
			changedFields["charger_type"] = *request.ChargerType
		}
		if request.Segment != nil {
			updates["segment"] = *request.Segment
			record.Segment = *request.Segment
			changedFields["segment"] = *request.Segment
		}
		if request.SubSegment != nil {
			updates["sub_segment"] = *request.SubSegment
			record.SubSegment = *request.SubSegment
			changedFields["sub_segment"] = *request.SubSegment
		}
		if request.ChargerUseType != nil {
			updates["charger_use_type"] = *request.ChargerUseType
			record.ChargerUseType = *request.ChargerUseType
			changedFields["charger_use_type"] = *request.ChargerUseType
		}
		if request.NumberOfConnectors != nil {
			updates["number_of_connectors"] = *request.NumberOfConnectors
			record.NumberOfConnectors = *request.NumberOfConnectors
			changedFields["number_of_connectors"] = *request.NumberOfConnectors
		}
		if request.Parking != nil {
			updates["parking"] = *request.Parking
			record.Parking = *request.Parking
			changedFields["parking"] = *request.Parking
		}
		if request.Protocol != nil {
			updates["protocol"] = *request.Protocol
			record.Protocol = *request.Protocol
			changedFields["protocol"] = *request.Protocol
		}
		if request.TwentyFourSevenOpen != nil {
			updates["twenty_four_seven_open_status"] = *request.TwentyFourSevenOpen
			record.TwentyFourSevenOpen = *request.TwentyFourSevenOpen
			changedFields["twenty_four_seven_open_status"] = *request.TwentyFourSevenOpen
		}

		file, err := ctx.FormFile("charger_image")
		if err == nil {
			filename := uuid.New().String() + filepath.Ext(file.Filename)
			uploads := "uploads"
			if _, err := os.Stat(uploads); os.IsNotExist(err) {
				if err := os.Mkdir(uploads, 0755); err != nil {
					return err
				}
			}

			chargerImagePath := filepath.Join(uploads, filename)
			if err := ctx.SaveUploadedFile(file, chargerImagePath); err != nil {
				return err
			}
			updates["charger_image"] = chargerImagePath
			record.ChargerImage = chargerImagePath
			changedFields["charger_image"] = chargerImagePath
		}

		now := service.now()

		if request.Connectors != nil {
			if len(*request.Connectors) == 0 {
				return &auth.APIError{
					Status:  http.StatusBadRequest,
					Code:    "invalid_request",
					Message: "Connectors list cannot be empty.",
				}
			}

			existingByID := make(map[uuid.UUID]*models.Connector, len(record.Connectors))
			for i := range record.Connectors {
				conn := &record.Connectors[i]
				existingByID[conn.ID] = conn
			}

			seenIDs := map[uuid.UUID]struct{}{}

			for _, connectorReq := range *request.Connectors {
				if connectorReq.ID == uuid.Nil {
					// This is a new connector
					if connectorReq.ConnectorNumber == nil || *connectorReq.ConnectorNumber <= 0 {
						return invalid("connector_number", "Connector number must be greater than zero.")
					}
					if connectorReq.ConnectorType == nil || strings.TrimSpace(*connectorReq.ConnectorType) == "" {
						return invalid("connector_type", "Connector type is required.")
					}
					if connectorReq.ConnectorTotalCapacity == nil || *connectorReq.ConnectorTotalCapacity < 0 {
						return invalid("connector_total_capacity", "Connector total capacity cannot be negative.")
					}

					connectorRecord := models.Connector{
						ID:                     uuid.New(),
						CPOID:                  cpoID,
						ChargerID:              record.ID,
						ConnectorNumber:        *connectorReq.ConnectorNumber,
						ConnectorType:          *connectorReq.ConnectorType,
						ConnectorTotalCapacity: *connectorReq.ConnectorTotalCapacity,
						Status:                 constants.ChargerStatusActive,
						CreatedAt:              now,
						UpdatedAt:              now,
					}

					if err := tx.Create(&connectorRecord).Error; err != nil {
						return mapConnectorWriteError(err, "create connector")
					}
					record.Connectors = append(record.Connectors, connectorRecord)
					continue
				}
				if _, dup := seenIDs[connectorReq.ID]; dup {
					return &auth.APIError{
						Status:  http.StatusBadRequest,
						Code:    "duplicate_connector_id",
						Message: "Connector IDs in the request must be unique.",
					}
				}
				seenIDs[connectorReq.ID] = struct{}{}

				existing, ok := existingByID[connectorReq.ID]
				if !ok {
					return mapConnectorNotFound(gorm.ErrRecordNotFound)
				}

				connUpdates := map[string]any{}
				connChanged := false

				if connectorReq.ConnectorNumber != nil {
					if *connectorReq.ConnectorNumber <= 0 {
						return invalid("connector_number", "Connector number must be greater than zero.")
					}
					connUpdates["connector_number"] = *connectorReq.ConnectorNumber
					existing.ConnectorNumber = *connectorReq.ConnectorNumber
					connChanged = true
				}
				if connectorReq.ConnectorType != nil {
					connType := strings.TrimSpace(*connectorReq.ConnectorType)
					if connType == "" {
						return invalid("connector_type", "Connector type is required.")
					}
					connUpdates["connector_type"] = connType
					existing.ConnectorType = connType
					connChanged = true
				}
				if connectorReq.ConnectorTotalCapacity != nil {
					if *connectorReq.ConnectorTotalCapacity < 0 {
						return invalid("connector_total_capacity", "Connector total capacity cannot be negative.")
					}
					connUpdates["connector_total_capacity"] = *connectorReq.ConnectorTotalCapacity
					existing.ConnectorTotalCapacity = *connectorReq.ConnectorTotalCapacity
					connChanged = true
				}

				if !connChanged {
					continue
				}

				connUpdates["updated_at"] = now
				existing.UpdatedAt = now
				changedFields["connectors"] = len(*request.Connectors)

				if err := tx.Model(&models.Connector{}).
					Where("id = ? AND cpo_id = ? AND charger_id = ?", existing.ID, cpoID, record.ID).
					Updates(connUpdates).Error; err != nil {
					return mapConnectorWriteError(err, "update connector")
				}
			}
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one charger field must be supplied.",
			}
		}

		updates["updated_at"] = now
		record.UpdatedAt = now

		if len(updates) > 1 {
			if err := tx.Model(&models.Charger{}).
				Where("id = ?", record.ID).
				Updates(updates).Error; err != nil {
				return mapChargerWriteError(err, "update charger")
			}
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_UPDATED",
			models.JSONB{
				"charger_id":     record.ChargerID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return ChargerResponse{}, err
	}

	if record.HubID != nil {
		var hub models.Hub
		if err := service.database.WithContext(ctx).First(&hub, "id = ?", *record.HubID).Error; err == nil {
			record.Hub = &hub
		}
	}

	return service.chargerView(record, principal), nil
}

func (service *Service) DeleteCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
) error {
	if err := requireCPOAdminAccess(principal); err != nil {
		return err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.Charger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors").
			First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if err := tx.Where("charger_id = ?", record.ID).Delete(&models.Connector{}).Error; err != nil {
			return mapChargerDeleteError(err)
		}

		if err := tx.Delete(&record).Error; err != nil {
			return mapChargerDeleteError(err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_DELETED",
			models.JSONB{
				"charger_id":    record.ChargerID,
				"ocpp_identity": record.OCPPIdentity,
				"hub_id":        record.HubID,
			},
			service.now(),
		)
	})
}

func normalizeCreateChargerRequest(request CreateChargerRequest) CreateChargerRequest {
	request.Vendor = strings.TrimSpace(request.Vendor)
	request.Model = strings.TrimSpace(request.Model)
	request.SerialNumber = strings.TrimSpace(request.SerialNumber)

	for i := range request.Connectors {
		request.Connectors[i].ConnectorType = strings.TrimSpace(request.Connectors[i].ConnectorType)
	}
	return request
}

func normalizeUpdateChargerRequest(request UpdateChargerRequest) UpdateChargerRequest {
	request.Vendor = trimOptionalString(request.Vendor)
	request.Model = trimOptionalString(request.Model)
	request.SerialNumber = trimOptionalString(request.SerialNumber)

	if request.Connectors != nil {
		connectors := *request.Connectors
		for i := range connectors {
			if connectors[i].ConnectorType != nil {
				value := strings.TrimSpace(*connectors[i].ConnectorType)
				connectors[i].ConnectorType = &value
			}
		}
		request.Connectors = &connectors
	}

	return request
}

func normalizeChargerID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateCreateChargerRequest(request CreateChargerRequest) error {
	if len(request.Vendor) > 100 {
		return invalid("vendor", "Vendor must not exceed 100 characters.")
	}
	if len(request.Model) > 100 {
		return invalid("model", "Model must not exceed 100 characters.")
	}
	if request.SerialNumber == "" || len(request.SerialNumber) > 100 {
		return invalid("serial_number", "Serial number is required and must not exceed 100 characters.")
	}
	if request.MaxPowerKW < 0 {
		return invalid("max_power_kw", "Max power kW must not be negative.")
	}
	if len(request.Connectors) == 0 {
		return invalid("connectors", "At least one connector is required.")
	}

	seenNumbers := map[int]struct{}{}
	for _, connector := range request.Connectors {
		if connector.ConnectorNumber <= 0 {
			return invalid("connector_number", "Connector number must be greater than zero.")
		}
		if strings.TrimSpace(connector.ConnectorType) == "" || len(connector.ConnectorType) > 50 {
			return invalid("connector_type", "Connector type is required and must not exceed 50 characters.")
		}
		if _, dup := seenNumbers[connector.ConnectorNumber]; dup {
			return invalid("connector_number", "Connector numbers must be unique within a charger.")
		}
		seenNumbers[connector.ConnectorNumber] = struct{}{}
	}
	return nil
}

func validateUpdateChargerRequest(request UpdateChargerRequest) error {
	if request.HubID == nil &&
		request.Vendor == nil &&
		request.Model == nil &&
		request.SerialNumber == nil &&
		request.MaxPowerKW == nil &&
		request.Connectors == nil {
		return invalid("charger", "At least one charger field must be supplied.")
	}

	if request.Vendor != nil && len(*request.Vendor) > 100 {
		return invalid("vendor", "Vendor must not exceed 100 characters.")
	}
	if request.Model != nil && len(*request.Model) > 100 {
		return invalid("model", "Model must not exceed 100 characters.")
	}
	if request.SerialNumber != nil && (*request.SerialNumber == "" || len(*request.SerialNumber) > 100) {
		return invalid("serial_number", "Serial number must not exceed 100 characters.")
	}
	if request.MaxPowerKW != nil && *request.MaxPowerKW < 0 {
		return invalid("max_power_kw", "Max power kW must not be negative.")
	}

	if request.Connectors != nil {
		if len(*request.Connectors) == 0 {
			return invalid("connectors", "Connectors list cannot be empty when provided.")
		}

		seenIDs := map[uuid.UUID]struct{}{}
		for _, connector := range *request.Connectors {
			if connector.ID == uuid.Nil {
				return invalid("connector_id", "Connector ID is required.")
			}
			if _, dup := seenIDs[connector.ID]; dup {
				return invalid("connector_id", "Connector IDs in the request must be unique.")
			}
			seenIDs[connector.ID] = struct{}{}

			changed := false
			if connector.ConnectorNumber != nil {
				if *connector.ConnectorNumber <= 0 {
					return invalid("connector_number", "Connector number must be greater than zero.")
				}
				changed = true
			}
			if connector.ConnectorType != nil {
				if strings.TrimSpace(*connector.ConnectorType) == "" || len(*connector.ConnectorType) > 50 {
					return invalid("connector_type", "Connector type must not exceed 50 characters.")
				}
				changed = true
			}
			if !changed {
				return invalid("connectors", "At least one connector field must be supplied for each connector.")
			}
		}
	}

	return nil
}

func requireCPOAdminAccess(principal auth.Principal) error {
	if principal.Scope != constants.AuthScopeCPO {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO access is required.",
		}
	}
	if principal.CPOID == nil {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO tenant context is required.",
		}
	}
	if principal.Role == nil {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO role is required.",
		}
	}
	if *principal.Role == constants.CPORoleAdmin {
		return nil
	}
	return &auth.APIError{
		Status:  http.StatusForbidden,
		Code:    "forbidden",
		Message: "CPO administrator access is required.",
	}
}

func generateUniqueChargerIDTx(tx *gorm.DB) (string, error) {
	for i := 0; i < 32; i++ {
		candidate, err := security.RandomHex(3)
		if err != nil {
			return "", err
		}
		candidate = strings.ToLower(candidate)
		if !chargerIDPattern.MatchString(candidate) {
			continue
		}

		var existing models.Charger
		err = tx.Select("id").Where("charger_id = ?", candidate).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check charger id uniqueness: %w", err)
		}
	}
	return "", &auth.APIError{
		Status:  http.StatusConflict,
		Code:    "charger_conflict",
		Message: "Unable to generate a unique charger ID.",
	}
}



func mapChargerNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "charger_not_found",
			Message: "The charger was not found.",
		}
	}
	return fmt.Errorf("load charger: %w", err)
}

func mapConnectorNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "connector_not_found",
			Message: "The connector was not found.",
		}
	}
	return fmt.Errorf("load connector: %w", err)
}

func mapHubNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "hub_not_found",
			Message: "The hub was not found.",
		}
	}
	return fmt.Errorf("load hub: %w", err)
}

func mapChargerWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_conflict",
				Message: "The charger ID, OCPP identity, or related unique value already exists.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_conflict",
				Message: "The charger references an invalid related record.",
			}
		case "23P01":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "tariff_schedule_conflict",
				Message: "Moving the charger would create an overlapping active tariff schedule.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapConnectorWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "connector_conflict",
				Message: "The connector already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "connector_conflict",
				Message: "The connector references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapChargerDeleteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23001" || postgresError.Code == "23503") {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "charger_in_use",
			Message: "The charger cannot be deleted because it has dependent records.",
		}
	}
	return fmt.Errorf("delete charger: %w", err)
}

func (service *Service) chargerView(record models.Charger, principal auth.Principal) ChargerResponse {
	connectorsView := make([]ConnectorView, 0, len(record.Connectors))

	for _, conn := range record.Connectors {
		connectorsView = append(connectorsView, ConnectorView{
			ID:                     conn.ID,
			CPOID:                  conn.CPOID,
			ChargerID:              conn.ChargerID,
			ConnectorNumber:        conn.ConnectorNumber,
			ConnectorType:          conn.ConnectorType,
			ConnectorTotalCapacity: conn.ConnectorTotalCapacity,
			Status:                 conn.Status,
			CreatedAt:              conn.CreatedAt,
			UpdatedAt:              conn.UpdatedAt,
		})
	}

	var hubName *string
	if record.Hub != nil {
		hubName = &record.Hub.Name
	}

	ocppIdentityForURL := strings.TrimPrefix(record.OCPPIdentity, "ocpp_")

	return ChargerResponse{
		ChargerView: ChargerView{
			ID:                      record.ID,
			CPOID:                   record.CPOID,
			HubID:                   record.HubID,
			HubName:                 hubName,
			ChargerID:               record.ChargerID,
			OCPPIdentity:            record.OCPPIdentity,
			Vendor:                  record.Vendor,
			Model:                   record.Model,
			SerialNumber:            record.SerialNumber,
			MaxPowerKW:              record.MaxPowerKW,
			Status:                  record.Status,
			OCPPVersion:             record.OCPPVersion,
			LastSeenAt:              record.LastSeenAt,
			ChargerName:             record.ChargerName,
			ChargerHostName:         record.ChargerHostName,
			ChargerHostPhoneNo:      record.ChargerHostPhoneNo,
			ChargerType:             record.ChargerType,
			Segment:                 record.Segment,
			SubSegment:              record.SubSegment,
			ChargerImage:            record.ChargerImage,
			ChargerUseType:          record.ChargerUseType,
			NumberOfConnectors:      record.NumberOfConnectors,
			Parking:                 record.Parking,
			Protocol:                record.Protocol,
			TwentyFourSevenOpen:     record.TwentyFourSevenOpen,
			Connectors:              connectorsView,
			ChargerConnectionURLWS:  fmt.Sprintf("ws://%s/%s", service.chargerConnectionURL, ocppIdentityForURL),
			ChargerConnectionURLWSS: fmt.Sprintf("wss://%s/%s", service.chargerConnectionURL, ocppIdentityForURL),
			Assigned:                record.HubID != nil,
			CreatedAt:               record.CreatedAt,
			UpdatedAt:               record.UpdatedAt,
		},
		Email: principal.User.Email,
	}
}

func (service *Service) ListChargers(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (ChargerListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return ChargerListResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var chargers []models.Charger
	if err := databaseQuery.
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&chargers).Error; err != nil {
		return ChargerListResponse{}, fmt.Errorf("list chargers: %w", err)
	}

	hasMore := len(chargers) > query.Limit
	if hasMore {
		chargers = chargers[:query.Limit]
	}
	response := make([]ChargerResponse, 0, len(chargers))
	for _, charger := range chargers {
		response = append(response, service.chargerView(charger, principal))
	}

	result := ChargerListResponse{Chargers: response, HasMore: hasMore}
	if hasMore && len(chargers) > 0 {
		nextBefore := chargers[len(chargers)-1].CreatedAt
		nextBeforeID := chargers[len(chargers)-1].ID
		result.NextBefore = &nextBefore
		result.NextBeforeID = &nextBeforeID
	}
	return result, nil
}

func validateTenantListQuery(query TenantListQuery) (TenantListQuery, error) {
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return TenantListQuery{}, invalid("limit", "Limit must be between 1 and 200.")
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return TenantListQuery{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}
	return query, nil
}

func (service *Service) CreateHub(
	ctx context.Context,
	principal auth.Principal,
	request CreateHubRequest,
) (HubView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubView{}, err
	}

	request = normalizeCreateHubRequest(request)
	if err := validateCreateHubRequest(request); err != nil {
		return HubView{}, err
	}

	open24Hours := true
	if request.Open24Hours != nil {
		open24Hours = *request.Open24Hours
	}

	var sanctionLoad float64
	if request.SanctionLoad != nil {
		sanctionLoad = *request.SanctionLoad
	}
	customerVisible := false
	if request.CustomerVisible != nil {
		customerVisible = *request.CustomerVisible
	}

	cpoID := *principal.CPOID
	var record models.Hub

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(request.ChargerIDs) > 0 {
			var chargers []models.Charger
			if err := tx.Where("id IN ? AND cpo_id = ?", request.ChargerIDs, cpoID).Find(&chargers).Error; err != nil {
				return fmt.Errorf("could not look up chargers: %w", err)
			}
			if len(chargers) != len(request.ChargerIDs) {
				return &auth.APIError{
					Status:  http.StatusNotFound,
					Code:    "charger_not_found",
					Message: "One or more chargers could not be found.",
				}
			}
			for _, charger := range chargers {
				if charger.HubID != nil {
					return &auth.APIError{
						Status:  http.StatusConflict,
						Code:    "charger_already_in_hub",
						Message: fmt.Sprintf("Charger %s is already in a hub.", charger.ChargerID),
					}
				}
			}
		}

		now := service.now()
		record = models.Hub{
			ID:              uuid.New(),
			CPOID:           cpoID,
			Name:            request.Name,
			Address:         request.Address,
			Latitude:        *request.Latitude,
			State:           request.State,
			Longitude:       *request.Longitude,
			Open24Hours:     open24Hours,
			SanctionLoad:    sanctionLoad,
			CustomerVisible: customerVisible,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapHubWriteError(err, "create hub")
		}

		if len(request.ChargerIDs) > 0 {
			if err := tx.Model(&models.Charger{}).
				Where("id IN ?", request.ChargerIDs).
				Updates(map[string]any{
					"hub_id":     record.ID,
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("could not assign chargers to hub: %w", err)
			}
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_CREATED",
			models.JSONB{
				"hub_id":            record.ID,
				"name":              record.Name,
				"open_24_hours":     record.Open24Hours,
				"sanction_load":     record.SanctionLoad,
				"customer_visible":  record.CustomerVisible,
				"chargers_assigned": len(request.ChargerIDs),
			},
			now,
		)
	})
	if err != nil {
		return HubView{}, err
	}

	return hubView(record), nil
}

func (service *Service) ListHubs(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (HubListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return HubListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []models.Hub
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return HubListResponse{}, fmt.Errorf("list hubs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	hubs := make([]HubView, 0, len(records))
	for _, record := range records {
		hubs = append(hubs, hubView(record))
	}
	response := HubListResponse{Hubs: hubs, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	query TenantListQuery,
) (HubResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubResponse{}, err
	}

	var hub models.Hub
	if err := service.database.WithContext(ctx).
		First(&hub, "cpo_id = ? AND id = ?", *principal.CPOID, hubID).Error; err != nil {
		return HubResponse{}, mapHubNotFound(err)
	}

	query, err := validateTenantListQuery(query)
	if err != nil {
		return HubResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ? AND hub_id = ?", *principal.CPOID, hubID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var chargers []models.Charger
	if err := databaseQuery.
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&chargers).Error; err != nil {
		return HubResponse{}, fmt.Errorf("list chargers for hub: %w", err)
	}

	hasMore := len(chargers) > query.Limit
	if hasMore {
		chargers = chargers[:query.Limit]
	}

	chargerResponses := make([]ChargerResponse, 0, len(chargers))
	for _, charger := range chargers {
		chargerResponses = append(chargerResponses, service.chargerView(charger, principal))
	}

	chargerListResponse := ChargerListResponse{
		Chargers: chargerResponses,
		HasMore:  hasMore,
	}

	if hasMore && len(chargers) > 0 {
		nextBefore := chargers[len(chargers)-1].CreatedAt
		nextBeforeID := chargers[len(chargers)-1].ID
		chargerListResponse.NextBefore = &nextBefore
		chargerListResponse.NextBeforeID = &nextBeforeID
	}

	return HubResponse{
		ID:              hub.ID,
		CPOID:           hub.CPOID,
		Name:            hub.Name,
		Address:         hub.Address,
		State:           hub.State,
		Latitude:        hub.Latitude,
		Longitude:       hub.Longitude,
		Open24Hours:     hub.Open24Hours,
		SanctionLoad:    hub.SanctionLoad,
		CustomerVisible: hub.CustomerVisible,
		CreatedAt:       hub.CreatedAt,
		UpdatedAt:       hub.UpdatedAt,
		Chargers:        &chargerListResponse,
	}, nil
}

func (service *Service) UpdateHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request UpdateHubRequest,
) (HubView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubView{}, err
	}

	request = normalizeUpdateHubRequest(request)
	if err := validateUpdateHubRequest(request); err != nil {
		return HubView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Hub
	changed := false

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, hubID).Error; err != nil {
			return mapHubNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil && record.Name != *request.Name {
			updates["name"] = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.Address != nil && record.Address != *request.Address {
			updates["address"] = *request.Address
			changedFields["address"] = *request.Address
		}
		if request.Latitude != nil && record.Latitude != *request.Latitude {
			updates["latitude"] = *request.Latitude
			changedFields["latitude"] = *request.Latitude
		}
		if request.Longitude != nil && record.Longitude != *request.Longitude {
			updates["longitude"] = *request.Longitude
			changedFields["longitude"] = *request.Longitude
		}
		if request.Open24Hours != nil && record.Open24Hours != *request.Open24Hours {
			updates["open_24_hours"] = *request.Open24Hours
			changedFields["open_24_hours"] = *request.Open24Hours
		}
		if request.SanctionLoad != nil && record.SanctionLoad != *request.SanctionLoad {
			updates["sanction_load"] = *request.SanctionLoad
			changedFields["sanction_load"] = *request.SanctionLoad
		}
		if request.CustomerVisible != nil && record.CustomerVisible != *request.CustomerVisible {
			updates["customer_visible"] = *request.CustomerVisible
			changedFields["customer_visible"] = *request.CustomerVisible
		}
		if request.State != nil && record.State != *request.State {
			updates["state"] = *request.State
			changedFields["state"] = *request.State
		}

		if len(changedFields) == 0 {
			return nil
		}
		changed = true

		for key, value := range changedFields {
			switch key {
			case "name":
				record.Name = value.(string)
			case "address":
				record.Address = value.(string)
			case "latitude":
				record.Latitude = value.(float64)
			case "longitude":
				record.Longitude = value.(float64)
			case "open_24_hours":
				record.Open24Hours = value.(bool)
			case "sanction_load":
				record.SanctionLoad = value.(float64)
			case "customer_visible":
				record.CustomerVisible = value.(bool)
			case "state":
				record.State = value.(string)
			}
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now
		if err := tx.Model(&models.Hub{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapHubWriteError(err, "update hub")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_UPDATED",
			models.JSONB{
				"hub_id":         record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return HubView{}, err
	}
	if !changed {
		if err := service.database.WithContext(ctx).
			First(&record, "cpo_id = ? AND id = ?", cpoID, hubID).Error; err != nil {
			return HubView{}, mapHubNotFound(err)
		}
	}

	return hubView(record), nil
}

func (service *Service) DeleteHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) error {
	if err := requireCPOAdminAccess(principal); err != nil {
		return err
	}

	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tariffCount int64
		if err := tx.Model(&models.Tariff{}).
			Where("hub_id = ? AND cpo_id = ?", hubID, *principal.CPOID).
			Count(&tariffCount).Error; err != nil {
			return fmt.Errorf("checking for tariffs associated with hub: %w", err)
		}
		if tariffCount > 0 {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_has_tariffs",
				Message: "The hub cannot be deleted because it has associated tariffs.",
			}
		}

		if err := tx.Model(&models.Charger{}).
			Where("hub_id = ? AND cpo_id = ?", hubID, *principal.CPOID).
			Update("hub_id", nil).Error; err != nil {
			return fmt.Errorf("disassociating chargers from hub: %w", err)
		}

		if err := tx.Exec("DELETE FROM user_group_hubs WHERE hub_id = ?", hubID).Error; err != nil {
			return fmt.Errorf("deleting user group hub associations: %w", err)
		}

		if err := tx.Exec("DELETE FROM customer_favorite_hubs WHERE hub_id = ?", hubID).Error; err != nil {
			return fmt.Errorf("deleting customer favorite hub associations: %w", err)
		}

		if result := tx.Delete(&models.Hub{}, "id = ? AND cpo_id = ?", hubID, *principal.CPOID); result.Error != nil {
			return fmt.Errorf("deleting hub: %w", result.Error)
		} else if result.RowsAffected == 0 {
			return mapHubNotFound(gorm.ErrRecordNotFound)
		}

		if err := writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"HUB_DELETED",
			models.JSONB{
				"hub_id": hubID,
			},
			service.now(),
		); err != nil {
			return err
		}

		hubResourceID := hubID.String()
		_, err := service.events.Emit(tx, platformops.EventInput{
			Type:         "cpo.hub.deleted",
			ActorUserID:  &principal.User.ID,
			ResourceType: "HUB",
			ResourceID:   &hubResourceID,
			Data:         models.JSONB{},
		})
		return err
	})
}

func (service *Service) ListCustomers(
	ctx context.Context,
	principal auth.Principal,
	query CPOAdminCustomerListQuery,
) (CPOAdminCustomerListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return CPOAdminCustomerListResponse{}, err
	}

	query.Search = strings.TrimSpace(query.Search)
	if len(query.Search) > maxSearchLength {
		return CPOAdminCustomerListResponse{}, invalid(
			"q",
			"Search text must not exceed 200 characters.",
		)
	}
	if query.Limit == 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit < 1 || query.Limit > maxListLimit {
		return CPOAdminCustomerListResponse{}, invalid(
			"limit",
			"Limit must be between 1 and 200.",
		)
	}
	if query.Status != nil && !query.Status.Valid() {
		return CPOAdminCustomerListResponse{}, invalid(
			"status",
			"Status must be ACTIVE or BLOCKED.",
		)
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return CPOAdminCustomerListResponse{}, invalid(
			"cursor",
			"before and before_id must be supplied together.",
		)
	}

	cpoID := *principal.CPOID
	databaseQuery := service.database.WithContext(ctx).Model(&models.Customer{}).
		Where("cpo_id = ?", cpoID)

	if query.Search != "" {
		search := strings.ToLower(query.Search)
		databaseQuery = databaseQuery.Where(
			`strpos(lower(email), ?) > 0 OR strpos(lower(full_name), ?) > 0 OR strpos(lower(coalesce(phone, '')), ?) > 0`,
			search, search, search,
		)
	}
	if query.Status != nil {
		databaseQuery = databaseQuery.Where("status = ?", *query.Status)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.Customer
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return CPOAdminCustomerListResponse{}, fmt.Errorf("list CPO customers: %w", err)
	}

	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}

	result := make([]CPOAdminCustomerView, 0, len(records))
	for _, record := range records {
		result = append(result, cpoAdminCustomerView(record))
	}

	response := CPOAdminCustomerListResponse{Customers: result, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetCustomer(
	ctx context.Context,
	principal auth.Principal,
	customerID uuid.UUID,
) (CPOAdminCustomerView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return CPOAdminCustomerView{}, err
	}

	var record models.Customer
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, customerID).Error; err != nil {
		return CPOAdminCustomerView{}, mapCustomerNotFound(err)
	}

	return cpoAdminCustomerView(record), nil
}

func cpoAdminCustomerView(record models.Customer) CPOAdminCustomerView {
	return CPOAdminCustomerView{
		ID:                record.ID,
		CPOID:             record.CPOID,
		Email:             record.Email,
		FullName:          record.FullName,
		Phone:             record.Phone,
		Status:            record.Status,
		IsVerified:        record.IsVerified,
		LastLoginAt:       record.LastLoginAt,
		UsergroupAssigned: record.UserGroupID != nil,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}
}

func mapCustomerNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "customer_not_found",
			Message: "The customer was not found for this CPO.",
		}
	}
	return fmt.Errorf("load customer: %w", err)
}

func (service *Service) AssignChargerToHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	chargerID uuid.UUID,
) (ChargerResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerResponse{}, err
	}
	if chargerID == uuid.Nil {
		return ChargerResponse{}, invalid("charger_id", "Charger ID is required.")
	}

	cpoID := *principal.CPOID
	var charger models.Charger

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hub models.Hub
		if err := tx.First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if charger.HubID != nil && *charger.HubID == hubID {
			return nil
		}

		previousHubID := charger.HubID
		now := service.now()
		if err := tx.Model(&charger).
			Where("id = ? AND cpo_id = ?", charger.ID, cpoID).
			Updates(map[string]any{
				"hub_id":     hub.ID,
				"updated_at": now,
			}).Error; err != nil {
			return mapChargerWriteError(err, "assign charger to hub")
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_HUB_REASSIGNED",
			models.JSONB{
				"charger_id":      charger.ID,
				"previous_hub_id": previousHubID,
				"new_hub_id":      hub.ID,
			},
			now,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ChargerResponse{}, err
	}
	if err := service.database.WithContext(ctx).
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("connector_number ASC")
		}).
		Preload("Hub").
		First(&charger, "id = ? AND cpo_id = ?", chargerID, cpoID).Error; err != nil {
		return ChargerResponse{}, fmt.Errorf("reload charger after assignment: %w", err)
	}

	return service.chargerView(charger, principal), nil
}

func normalizeCreateHubRequest(request CreateHubRequest) CreateHubRequest {
	request.Name = strings.TrimSpace(request.Name)
	request.Address = strings.TrimSpace(request.Address)
	request.State = strings.TrimSpace(request.State)
	return request
}

func normalizeUpdateHubRequest(request UpdateHubRequest) UpdateHubRequest {
	request.Name = trimOptionalString(request.Name)
	request.Address = trimOptionalString(request.Address)
	request.State = trimOptionalString(request.State)
	return request
}

func validateCreateHubRequest(request CreateHubRequest) error {
	if request.Name == "" || len(request.Name) > 255 {
		return invalid("name", "Hub name is required and must not exceed 255 characters.")
	}
	if request.Address == "" || len(request.Address) > 5000 {
		return invalid("address", "Hub address is required and must not exceed 5000 characters.")
	}
	if request.State == "" || len(request.State) > 100 {
		return invalid("state", "Hub state is required and must not exceed 100 characters.")
	}
	if request.Latitude == nil {
		return invalid("latitude", "Latitude is required.")
	}
	if *request.Latitude < -90 || *request.Latitude > 90 {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude == nil {
		return invalid("longitude", "Longitude is required.")
	}
	if *request.Longitude < -180 || *request.Longitude > 180 {
		return invalid("longitude", "Longitude must be between -180 and 180.")
	}
	if request.SanctionLoad != nil && *request.SanctionLoad < 0 {
		return invalid("sanction_load", "Sanction load must not be negative.")
	}
	return nil
}

func validateUpdateHubRequest(request UpdateHubRequest) error {
	if request.Name == nil &&
		request.Address == nil &&
		request.Latitude == nil &&
		request.State == nil &&
		request.Longitude == nil &&
		request.Open24Hours == nil &&
		request.SanctionLoad == nil &&
		request.CustomerVisible == nil {
		return invalid("hub", "At least one hub field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 255) {
		return invalid("name", "Hub name must not exceed 255 characters.")
	}
	if request.Address != nil && (*request.Address == "" || len(*request.Address) > 5000) {
		return invalid("address", "Hub address must not exceed 5000 characters.")
	}
	if request.State != nil && (*request.State == "" || len(*request.State) > 100) {
		return invalid("state", "Hub state must not exceed 100 characters.")
	}
	if request.Latitude != nil && (*request.Latitude < -90 || *request.Latitude > 90) {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude != nil && (*request.Longitude < -180 || *request.Longitude > 180) {
		return invalid("longitude", "Longitude must be between -180 and 180.")
	}
	if request.SanctionLoad != nil && *request.SanctionLoad < 0 {
		return invalid("sanction_load", "Sanction load must not be negative.")
	}
	return nil
}

func mapHubWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_conflict",
				Message: "The hub already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_conflict",
				Message: "The hub references an invalid related record.",
			}
		case "23514":
			if postgresError.ConstraintName == "chk_hubs_sanction_load" {
				return invalid("sanction_load", "Sanction load must not be negative.")
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func hubView(record models.Hub) HubView {
	return HubView{
		ID:              record.ID,
		CPOID:           record.CPOID,
		Name:            record.Name,
		Address:         record.Address,
		State:           record.State,
		Latitude:        record.Latitude,
		Longitude:       record.Longitude,
		Open24Hours:     record.Open24Hours,
		SanctionLoad:    record.SanctionLoad,
		CustomerVisible: record.CustomerVisible,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func normalizeCreateTariffRequest(request CreateTariffRequest) CreateTariffRequest {
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	if request.Currency == "" {
		request.Currency = "INR"
	}
	return request
}

func normalizeUpdateTariffRequest(request UpdateTariffRequest) UpdateTariffRequest {
	if request.Currency != nil {
		value := strings.ToUpper(strings.TrimSpace(*request.Currency))
		request.Currency = &value
	}
	return request
}

func validateCreateTariffRequest(request CreateTariffRequest) error {
	if request.HubID == uuid.Nil {
		return invalid("hub_id", "Hub ID is required.")
	}
	if request.PricePerKWh.Sign() <= 0 {
		return invalid("price_per_kwh", "Price per kWh must be greater than zero.")
	}
	if request.IdleFeePerMin.Sign() < 0 {
		return invalid("idle_fee_per_min", "Idle fee per minute must not be negative.")
	}
	if len(request.Currency) != 3 {
		return invalid("currency", "Currency must be a 3-letter code.")
	}
	if err := validateTariffDateRange(request.StartDate, request.EndDate); err != nil {
		return err
	}
	return nil
}

func validateUpdateTariffRequest(request UpdateTariffRequest) error {
	if request.HubID == nil &&
		request.ChargerID == nil &&
		request.GSTID == nil &&
		request.UserGroupID == nil &&
		request.PricePerKWh == nil &&
		request.IdleFeePerMin == nil &&
		request.Currency == nil &&
		request.IsActive == nil &&
		request.StartDate == nil &&
		request.EndDate == nil {
		return invalid("tariff", "At least one tariff field must be supplied.")
	}

	if request.PricePerKWh != nil && request.PricePerKWh.Sign() <= 0 {
		return invalid("price_per_kwh", "Price per kWh must be greater than zero.")
	}
	if request.IdleFeePerMin != nil && request.IdleFeePerMin.Sign() < 0 {
		return invalid("idle_fee_per_min", "Idle fee per minute must not be negative.")
	}
	if request.Currency != nil && len(*request.Currency) != 3 {
		return invalid("currency", "Currency must be a 3-letter code.")
	}
	return nil
}

func validateTariffDateRange(startDate, endDate *time.Time) error {
	if startDate == nil && endDate == nil {
		return nil
	}
	if startDate == nil || endDate == nil {
		return invalid("schedule", "start_date and end_date must be supplied together.")
	}
	if !startDate.Before(*endDate) {
		return invalid("date_range", "Start date must be strictly before end date.")
	}
	return nil
}

func mapGSTNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "gst_not_found",
			Message: "The GST profile was not found.",
		}
	}
	return fmt.Errorf("load gst: %w", err)
}

func mapUserGroupNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "user_group_not_found",
			Message: "The user group was not found.",
		}
	}
	return fmt.Errorf("load user group: %w", err)
}

func (service *Service) tariffView(record *models.Tariff) TariffView {
	return TariffView{
		ID:            record.ID,
		CPOID:         record.CPOID,
		HubID:         record.HubID,
		ChargerID:     record.ChargerID,
		GSTID:         record.GSTID,
		UserGroupID:   record.UserGroupID,
		PricePerKWh:   record.PricePerKWh,
		IdleFeePerMin: record.IdleFeePerMin,
		Currency:      record.Currency,
		IsActive:      record.IsActive,
		StartDate:     record.StartDate,
		EndDate:       record.EndDate,
		TariffType:    record.TariffType,
		PriceType:     record.PriceType,
		Units:         record.Units,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}

func (service *Service) handleTariffError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "tariff_not_found",
			Message: "The tariff was not found.",
		}
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23503" {
		switch postgresError.ConstraintName {
		case "tariffs_hub_id_fkey":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "hub_not_found",
				Message: "The hub for this tariff does not exist.",
			}
		case "tariffs_charger_id_fkey":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "charger_not_found",
				Message: "The charger for this tariff does not exist.",
			}
		case "tariffs_user_group_id_fkey":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "user_group_not_found",
				Message: "The user group for this tariff does not exist.",
			}
		case "tariffs_gst_id_fkey":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_not_found",
				Message: "The GST record for this tariff does not exist.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (service *Service) CreateGST(
	ctx context.Context,
	principal auth.Principal,
	request CreateGSTRequest,
) (GSTView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return GSTView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateGSTRequest(request)
	if err := validateCreateGSTRequest(request); err != nil {
		return GSTView{}, err
	}

	var record models.GST
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()

		isActive := true
		if request.IsActive != nil {
			isActive = *request.IsActive
		}

		record = models.GST{
			ID:        uuid.New(),
			CPOID:     cpoID,
			Name:      request.Name,
			State:     request.State,
			SGSTRate:  request.SGSTRate,
			CGSTRate:  request.CGSTRate,
			IGSTRate:  request.IGSTRate,
			IsActive:  isActive,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapGSTWriteError(err, "create gst")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"GST_CREATED",
			models.JSONB{
				"gst_id":    record.ID,
				"name":      record.Name,
				"state":     record.State,
				"sgst_rate": record.SGSTRate,
				"cgst_rate": record.CGSTRate,
				"igst_rate": record.IGSTRate,
			},
			now,
		)
	})
	if err != nil {
		return GSTView{}, err
	}

	return gstView(record), nil
}

func (service *Service) ListGSTs(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (GSTListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return GSTListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return GSTListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}
	var records []models.GST
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return GSTListResponse{}, fmt.Errorf("list GST profiles: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	gsts := make([]GSTView, 0, len(records))
	for _, record := range records {
		gsts = append(gsts, gstView(record))
	}
	response := GSTListResponse{GSTs: gsts, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetGST(
	ctx context.Context,
	principal auth.Principal,
	gstID uuid.UUID,
) (GSTView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return GSTView{}, err
	}

	var record models.GST
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, gstID).Error; err != nil {
		return GSTView{}, mapGSTNotFound(err)
	}

	return gstView(record), nil
}

func (service *Service) UpdateGST(
	ctx context.Context,
	principal auth.Principal,
	gstID uuid.UUID,
	request UpdateGSTRequest,
) (GSTView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return GSTView{}, err
	}

	request = normalizeUpdateGSTRequest(request)
	if err := validateUpdateGSTRequest(request); err != nil {
		return GSTView{}, err
	}

	cpoID := *principal.CPOID
	var record models.GST

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, gstID).Error; err != nil {
			return mapGSTNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil {
			updates["name"] = *request.Name
			record.Name = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.State != nil {
			updates["state"] = *request.State
			record.State = *request.State
			changedFields["state"] = *request.State
		}
		if request.SGSTRate != nil {
			updates["sgst_rate"] = *request.SGSTRate
			record.SGSTRate = request.SGSTRate
			changedFields["sgst_rate"] = *request.SGSTRate
		}
		if request.CGSTRate != nil {
			updates["cgst_rate"] = *request.CGSTRate
			record.CGSTRate = request.CGSTRate
			changedFields["cgst_rate"] = *request.CGSTRate
		}
		if request.IGSTRate != nil {
			updates["igst_rate"] = *request.IGSTRate
			record.IGSTRate = request.IGSTRate
			changedFields["igst_rate"] = *request.IGSTRate
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one GST field must be supplied.",
			}
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if err := tx.Model(&models.GST{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapGSTWriteError(err, "update gst")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"GST_UPDATED",
			models.JSONB{
				"gst_id":         record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})
	if err != nil {
		return GSTView{}, err
	}

	return gstView(record), nil
}

func normalizeCreateGSTRequest(request CreateGSTRequest) CreateGSTRequest {
	request.Name = strings.TrimSpace(request.Name)
	request.State = strings.TrimSpace(request.State)
	return request
}

func normalizeUpdateGSTRequest(request UpdateGSTRequest) UpdateGSTRequest {
	request.Name = trimOptionalString(request.Name)
	request.State = trimOptionalString(request.State)
	return request
}

func validateCreateGSTRequest(request CreateGSTRequest) error {
	if request.Name == "" || len(request.Name) > 100 {
		return invalid("name", "GST name is required and must not exceed 100 characters.")
	}
	if request.State == "" || len(request.State) > 100 {
		return invalid("state", "GST state is required and must not exceed 100 characters.")
	}
	if request.SGSTRate == nil {
		return invalid("sgst_rate", "SGST rate is required.")
	}
	if request.CGSTRate == nil {
		return invalid("cgst_rate", "CGST rate is required.")
	}
	if request.IGSTRate == nil {
		return invalid("igst_rate", "IGST rate is required.")
	}
	if request.SGSTRate.Sign() < 0 {
		return invalid("sgst_rate", "SGST rate must not be negative.")
	}
	if request.CGSTRate.Sign() < 0 {
		return invalid("cgst_rate", "CGST rate must not be negative.")
	}
	if request.IGSTRate.Sign() < 0 {
		return invalid("igst_rate", "IGST rate must not be negative.")
	}
	if request.SGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("sgst_rate", "SGST rate must not exceed 100.")
	}
	if request.CGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("cgst_rate", "CGST rate must not exceed 100.")
	}
	if request.IGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
		return invalid("igst_rate", "IGST rate must not exceed 100.")
	}
	return nil
}

func validateUpdateGSTRequest(request UpdateGSTRequest) error {
	if request.Name == nil &&
		request.State == nil &&
		request.SGSTRate == nil &&
		request.CGSTRate == nil &&
		request.IGSTRate == nil &&
		request.IsActive == nil {
		return invalid("gst", "At least one GST field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 100) {
		return invalid("name", "GST name must not exceed 100 characters.")
	}
	if request.State != nil && (*request.State == "" || len(*request.State) > 100) {
		return invalid("state", "GST state must not exceed 100 characters.")
	}
	if request.SGSTRate != nil {
		if request.SGSTRate.Sign() < 0 || request.SGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("sgst_rate", "SGST rate must be between 0 and 100.")
		}
	}
	if request.CGSTRate != nil {
		if request.CGSTRate.Sign() < 0 || request.CGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("cgst_rate", "CGST rate must be between 0 and 100.")
		}
	}
	if request.IGSTRate != nil {
		if request.IGSTRate.Sign() < 0 || request.IGSTRate.Cmp(decimal.NewFromInt(100)) > 0 {
			return invalid("igst_rate", "IGST rate must be between 0 and 100.")
		}
	}
	return nil
}

func mapGSTWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_conflict",
				Message: "The GST profile already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "gst_conflict",
				Message: "The GST profile references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func gstView(record models.GST) GSTView {
	return GSTView{
		ID:        record.ID,
		CPOID:     record.CPOID,
		Name:      record.Name,
		State:     record.State,
		SGSTRate:  record.SGSTRate,
		CGSTRate:  record.CGSTRate,
		IGSTRate:  record.IGSTRate,
		IsActive:  record.IsActive,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func (service *Service) UpdateChargerStatus(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
	request UpdateChargerStatusRequest,
) (ChargerStatusResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerStatusResponse{}, err
	}

	if !request.Status.Valid() {
		return ChargerStatusResponse{}, invalid("status", "Invalid charger status.")
	}

	var charger models.Charger
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
			return mapChargerNotFound(err)
		}

		if request.OCPPIdentity != "" && charger.OCPPIdentity != request.OCPPIdentity {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "ocpp_identity_mismatch",
				Message: "OCPP identity does not match the charger.",
			}
		}

		now := service.now()
		if err := tx.Model(&charger).
			Updates(map[string]any{
				"status":     request.Status,
				"updated_at": now,
			}).Error; err != nil {
			return mapChargerWriteError(err, "update charger status")
		}

		return writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"CHARGER_STATUS_UPDATED",
			models.JSONB{
				"charger_id": charger.ID,
				"status":     request.Status,
			},
			now,
		)
	})

	if err != nil {
		return ChargerStatusResponse{}, err
	}

	return ChargerStatusResponse{
		ChargerID:    charger.ID,
		OCPPIdentity: charger.OCPPIdentity,
		Status:       charger.Status,
	}, nil
}

func (service *Service) GetChargerStatus(
	ctx context.Context,
	principal auth.Principal,
	chargerID uuid.UUID,
) (ChargerStatusResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerStatusResponse{}, err
	}

	var charger models.Charger
	if err := service.database.WithContext(ctx).
		First(&charger, "id = ? AND cpo_id = ?", chargerID, *principal.CPOID).Error; err != nil {
		return ChargerStatusResponse{}, mapChargerNotFound(err)
	}

	return ChargerStatusResponse{
		ChargerID:    charger.ID,
		OCPPIdentity: charger.OCPPIdentity,
		Status:       charger.Status,
	}, nil
}

type ImageDownload struct {
	Content      io.ReadSeeker
	OriginalName string
	DetectedMIME string
	ModTime      time.Time
}

func (service *Service) DownloadChargerImage(ctx context.Context, principal auth.Principal, chargerID string) (*ImageDownload, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return nil, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	var record models.Charger
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return nil, mapChargerNotFound(err)
	}

	if record.ChargerImage == "" {
		return nil, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "image_not_found",
			Message: "The charger does not have an image.",
		}
	}

	if strings.Contains(record.ChargerImage, "..") {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_image_path",
			Message: "The image path is invalid.",
		}
	}

	imagePath := record.ChargerImage

	file, err := os.Open(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "image_not_found",
				Message: "The charger image file was not found.",
			}
		}
		return nil, fmt.Errorf("open charger image: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat charger image: %w", err)
	}

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		file.Close()
		return nil, fmt.Errorf("read charger image for mime type detection: %w", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("seek charger image after mime type detection: %w", err)
	}

	mimeType := http.DetectContentType(buffer)

	return &ImageDownload{
		Content:      file,
		OriginalName: filepath.Base(imagePath),
		DetectedMIME: mimeType,
		ModTime:      info.ModTime(),
	}, nil
}

func (service *Service) UpdateHubCustomerVisibility(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
	request UpdateHubCustomerVisibilityRequest,
) (HubView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubView{}, err
	}

	var hub models.Hub
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&hub, "id = ? AND cpo_id = ?", hubID, *principal.CPOID).Error; err != nil {
			return mapHubNotFound(err)
		}

		now := service.now()
		if err := tx.Model(&hub).
			Updates(map[string]any{
				"customer_visible": request.CustomerVisible,
				"updated_at":       now,
			}).Error; err != nil {
			return mapHubWriteError(err, "update hub customer visibility")
		}
		hub.CustomerVisible = request.CustomerVisible

		return writeAudit(
			tx,
			principal.UserID,
			*principal.CPOID,
			"HUB_CUSTOMER_VISIBILITY_UPDATED",
			models.JSONB{
				"hub_id":           hub.ID,
				"customer_visible": request.CustomerVisible,
			},
			now,
		)
	})

	if err != nil {
		return HubView{}, err
	}

	return hubView(hub), nil
}

func (service *Service) CreateUserGroup(
	ctx context.Context,
	principal auth.Principal,
	request CreateUserGroupRequest,
) (UserGroupView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return UserGroupView{}, err
	}

	cpoID := *principal.CPOID
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)

	if request.Name == "" {
		return UserGroupView{}, invalid("name", "User group name is required.")
	}

	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}

	now := service.now()
	record := models.UserGroup{
		ID:          uuid.New(),
		CPOID:       cpoID,
		Name:        request.Name,
		Description: request.Description,
		IsActive:    isActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return mapUserGroupWriteError(err, "create user group")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_CREATED",
			models.JSONB{
				"user_group_id": record.ID,
				"name":          record.Name,
			},
			now,
		)
	})

	if err != nil {
		return UserGroupView{}, err
	}

	return userGroupView(record), nil
}

func (service *Service) ListUserGroups(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (UserGroupListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return UserGroupListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return UserGroupListResponse{}, err
	}

	databaseQuery := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID)
	if query.Before != nil {
		databaseQuery = databaseQuery.Where(
			"(created_at, id) < (?, ?)",
			*query.Before,
			*query.BeforeID,
		)
	}

	var records []models.UserGroup
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return UserGroupListResponse{}, fmt.Errorf("list user groups: %w", err)
	}

	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}

	userGroups := make([]UserGroupView, 0, len(records))
	for _, record := range records {
		userGroups = append(userGroups, userGroupView(record))
	}

	response := UserGroupListResponse{UserGroups: userGroups, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
) (UserGroupView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return UserGroupView{}, err
	}

	var record models.UserGroup
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, userGroupID).Error; err != nil {
		return UserGroupView{}, mapUserGroupNotFound(err)
	}

	var members []models.Customer
	if err := service.database.WithContext(ctx).
		Where("user_group_id = ?", userGroupID).
		Find(&members).Error; err != nil {
		return UserGroupView{}, fmt.Errorf("list user group members: %w", err)
	}

	memberViews := make([]CPOAdminCustomerView, 0, len(members))
	for _, member := range members {
		memberViews = append(memberViews, cpoAdminCustomerView(member))
	}

	return userGroupView(record, memberViews), nil
}

func (service *Service) UpdateUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request UpdateUserGroupRequest,
) (UserGroupView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return UserGroupView{}, err
	}

	request.Name = trimOptionalString(request.Name)
	request.Description = trimOptionalString(request.Description)

	if request.Name == nil && request.Description == nil && request.IsActive == nil {
		return UserGroupView{}, invalid("user_group", "At least one user group field must be supplied.")
	}

	if request.Name != nil && *request.Name == "" {
		return UserGroupView{}, invalid("name", "User group name is required.")
	}

	cpoID := *principal.CPOID
	var record models.UserGroup

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, userGroupID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil {
			updates["name"] = *request.Name
			record.Name = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.Description != nil {
			updates["description"] = *request.Description
			record.Description = *request.Description
			changedFields["description"] = *request.Description
		}
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if err := tx.Model(&models.UserGroup{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapUserGroupWriteError(err, "update user group")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_UPDATED",
			models.JSONB{
				"user_group_id":  record.ID,
				"changed_fields": changedFields,
			},
			now,
		)
	})

	if err != nil {
		return UserGroupView{}, err
	}

	return userGroupView(record), nil
}

func (service *Service) DeleteUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
) error {
	if err := requireCPOAdminAccess(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, userGroupID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		if err := tx.Delete(&record).Error; err != nil {
			return mapUserGroupDeleteError(err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_DELETED",
			models.JSONB{"user_group_id": userGroupID},
			service.now(),
		)
	})
}

func (service *Service) AddMemberToUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	request AddMemberToUserGroupRequest,
) error {
	if err := requireCPOAdminAccess(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userGroup models.UserGroup
		if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", userGroupID, cpoID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		var customer models.Customer
		if err := tx.First(&customer, "id = ? AND cpo_id = ?", request.CustomerID, cpoID).Error; err != nil {
			return mapCustomerNotFound(err)
		}

		if customer.UserGroupID != nil {
			if *customer.UserGroupID == userGroupID {
				return nil // Already in the group, do nothing.
			}
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "customer_already_in_group",
				Message: "The customer is already a member of another user group.",
			}
		}

		now := service.now()
		if err := tx.Model(&customer).
			Updates(map[string]any{
				"user_group_id": userGroupID,
				"updated_at":    now,
			}).Error; err != nil {
			return fmt.Errorf("add member to user group: %w", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_MEMBER_ADDED",
			models.JSONB{
				"user_group_id": userGroupID,
				"customer_id":   request.CustomerID,
			},
			now,
		)
	})
}

func (service *Service) RemoveMemberFromUserGroup(
	ctx context.Context,
	principal auth.Principal,
	userGroupID uuid.UUID,
	customerID uuid.UUID,
) error {
	if err := requireCPOAdminAccess(principal); err != nil {
		return err
	}

	cpoID := *principal.CPOID
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var userGroup models.UserGroup
		if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", userGroupID, cpoID).Error; err != nil {
			return mapUserGroupNotFound(err)
		}

		var customer models.Customer
		if err := tx.First(&customer, "id = ? AND cpo_id = ?", customerID, cpoID).Error; err != nil {
			return mapCustomerNotFound(err)
		}

		if customer.UserGroupID == nil || *customer.UserGroupID != userGroupID {
			return nil // Not in the group, do nothing.
		}

		now := service.now()
		if err := tx.Model(&customer).
			Updates(map[string]any{
				"user_group_id": nil,
				"updated_at":    now,
			}).Error; err != nil {
			return fmt.Errorf("remove member from user group: %w", err)
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"USER_GROUP_MEMBER_REMOVED",
			models.JSONB{
				"user_group_id": userGroupID,
				"customer_id":   customerID,
			},
			now,
		)
	})
}
func mapUserGroupDeleteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && (postgresError.Code == "23001" || postgresError.Code == "23503") {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "user_group_in_use",
			Message: "The user group cannot be deleted because it has dependent records, such as tariffs.",
		}
	}
	return fmt.Errorf("delete user group: %w", err)
}

func userGroupView(record models.UserGroup, members ...[]CPOAdminCustomerView) UserGroupView {
	view := UserGroupView{
		ID:          record.ID,
		CPOID:       record.CPOID,
		Name:        record.Name,
		Description: record.Description,
		IsActive:    record.IsActive,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
	if len(members) > 0 {
		view.Members = members[0]
	}
	return view
}

func mapUserGroupWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "user_group_conflict",
				Message: "The user group already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "user_group_conflict",
				Message: "The user group references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (service *Service) GetSettings(
	ctx context.Context,
	principal auth.Principal,
) (SettingsView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return SettingsView{}, err
	}

	cpoID := *principal.CPOID
	var settings models.Settings
	if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpoID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SettingsView{}, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "settings_not_found",
				Message: "Settings for this CPO not found.",
			}
		}
		return SettingsView{}, fmt.Errorf("failed to get settings: %w", err)
	}

	return SettingsView{
		InvoiceLogo: settings.InvoiceLogo,
		InvoiceNote: settings.InvoiceNote,
	}, nil
}

func (service *Service) CreateOrUpdateSettings(
	ctx *gin.Context,
	principal auth.Principal,
) (SettingsView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return SettingsView{}, err
	}

	cpoID := *principal.CPOID
	err := ctx.Request.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		return SettingsView{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_request",
			Message: "Invalid multipart form.",
		}
	}

	invoiceNote := ctx.Request.FormValue("invoice_note")

	var settings models.Settings
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cpo_id = ?", cpoID).First(&settings).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to get settings: %w", err)
			}
			// Settings not found, create new
			settings = models.Settings{
				CPOID: cpoID,
			}
		}

		file, header, err := ctx.Request.FormFile("invoice_logo")
		if err == nil {
			defer file.Close()
			//- TODO: delete old file if it exists
			filename := uuid.New().String() + filepath.Ext(header.Filename)
			uploadsDir := "uploads"
			if _, err := os.Stat(uploadsDir); os.IsNotExist(err) {
				if err := os.Mkdir(uploadsDir, 0755); err != nil {
					return fmt.Errorf("failed to create uploads directory: %w", err)
				}
			}
			filePath := filepath.Join(uploadsDir, filename)
			out, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer out.Close()
			_, err = io.Copy(out, file)
			if err != nil {
				return fmt.Errorf("failed to save file: %w", err)
			}
			settings.InvoiceLogo = &filePath
		} else if !errors.Is(err, http.ErrMissingFile) {
			return fmt.Errorf("failed to get invoice logo: %w", err)
		}

		if invoiceNote != "" {
			settings.InvoiceNote = &invoiceNote
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cpo_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"invoice_logo", "invoice_note", "updated_at"}),
		}).Create(&settings).Error; err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}

		return nil
	})

	if err != nil {
		return SettingsView{}, err
	}

	return SettingsView{
		InvoiceLogo: settings.InvoiceLogo,
		InvoiceNote: settings.InvoiceNote,
	}, nil
}

// DownloadInvoiceLogo retrieves the invoice logo file for the authenticated CPO.
func (service *Service) DownloadInvoiceLogo(ctx context.Context, principal auth.Principal) (*ImageDownload, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return nil, err
	}

	cpoID := *principal.CPOID
	var settings models.Settings
	if err := service.database.WithContext(ctx).
		Where("cpo_id = ?", cpoID).
		First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "settings_not_found",
				Message: "Settings for this CPO not found.",
			}
		}
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}

	if settings.InvoiceLogo == nil || *settings.InvoiceLogo == "" {
		return nil, &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "invoice_logo_not_found",
			Message: "No invoice logo has been uploaded.",
		}
	}

	imagePath := *settings.InvoiceLogo
	if strings.Contains(imagePath, "..") {
		return nil, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_image_path",
			Message: "The image path is invalid.",
		}
	}

	file, err := os.Open(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &auth.APIError{
				Status:  http.StatusNotFound,
				Code:    "invoice_logo_not_found",
				Message: "The invoice logo file was not found.",
			}
		}
		return nil, fmt.Errorf("open invoice logo: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat invoice logo: %w", err)
	}

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		file.Close()
		return nil, fmt.Errorf("read invoice logo for mime detection: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("seek invoice logo: %w", err)
	}

	mimeType := http.DetectContentType(buffer)

	return &ImageDownload{
		Content:      file,
		OriginalName: filepath.Base(imagePath),
		DetectedMIME: mimeType,
		ModTime:      info.ModTime(),
	}, nil
}
