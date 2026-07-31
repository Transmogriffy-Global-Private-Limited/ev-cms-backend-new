package cpo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
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
	slugPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	gstinPattern = regexp.MustCompile(`^[0-9A-Z]{15}$`)
	appIDPattern = regexp.MustCompile(`^[a-z0-9_-]{16,100}$`)
)

type Service struct {
	database    *gorm.DB
	outbox      *cmsmail.Outbox
	mailEnabled bool
	events      *platformops.Service
	now         func() time.Time
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
) *Service {
	return &Service{
		database:    database,
		outbox:      outbox,
		mailEnabled: mailEnabled,
		now:         func() time.Time { return time.Now().UTC() },
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
		return sessionRevocationCounts{}, fmt.Errorf(
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
		return sessionRevocationCounts{}, fmt.Errorf(
			"revoke CPO sessions: %w",
			sessionResult.Error,
		)
	}
	return sessionRevocationCounts{
		sessions:      sessionResult.RowsAffected,
		refreshTokens: refreshResult.RowsAffected,
	}, nil
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
	request.Slug = strings.ToLower(strings.TrimSpace(request.Slug))
	request.BusinessName = strings.TrimSpace(request.BusinessName)
	request.Address = strings.TrimSpace(request.Address)
	request.City = strings.TrimSpace(request.City)
	request.State = strings.TrimSpace(request.State)
	request.Pincode = strings.TrimSpace(request.Pincode)
	request.Admin.Email = strings.ToLower(strings.TrimSpace(request.Admin.Email))
	request.Admin.FullName = strings.TrimSpace(request.Admin.FullName)
	if request.GSTIN != nil {
		value := strings.ToUpper(strings.TrimSpace(*request.GSTIN))
		request.GSTIN = &value
	}
	return request
}

func normalizeProfileRequest(request UpdateProfileRequest) UpdateProfileRequest {
	request.BusinessName = strings.TrimSpace(request.BusinessName)
	request.Address = strings.TrimSpace(request.Address)
	request.City = strings.TrimSpace(request.City)
	request.State = strings.TrimSpace(request.State)
	request.Pincode = strings.TrimSpace(request.Pincode)
	if request.GSTIN != nil {
		value := strings.ToUpper(strings.TrimSpace(*request.GSTIN))
		if value == "" {
			request.GSTIN = nil
		} else {
			request.GSTIN = &value
		}
	}
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
	if request.GSTIN != nil && !gstinPattern.MatchString(*request.GSTIN) {
		return invalid(
			"gstin",
			"GSTIN must contain 15 uppercase letters or digits.",
		)
	}
	if len(request.Address) > 5000 || len(request.City) > 100 ||
		len(request.State) > 100 || len(request.Pincode) > 10 {
		return invalid(
			"profile",
			"One or more CPO profile fields exceed their maximum length.",
		)
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
	if len(request.Slug) > 80 || !slugPattern.MatchString(request.Slug) {
		return invalid("slug", "Slug must contain lowercase words separated by single hyphens.")
	}
	if request.BusinessName == "" || len(request.BusinessName) > 255 {
		return invalid("business_name", "Business name is required and must not exceed 255 characters.")
	}
	if !request.CompanyType.Valid() {
		return invalid("company_type", "Company type must be INDIVIDUAL or COMPANY.")
	}
	if request.GSTIN != nil && !gstinPattern.MatchString(*request.GSTIN) {
		return invalid("gstin", "GSTIN must contain 15 uppercase letters or digits.")
	}
	if len(request.Address) > 5000 || len(request.City) > 100 ||
		len(request.State) > 100 || len(request.Pincode) > 10 {
		return invalid("profile", "One or more CPO profile fields exceed their maximum length.")
	}
	if !validEmail(request.Admin.Email) {
		return invalid("admin.email", "Administrator email is invalid.")
	}
	if request.Admin.FullName == "" || len(request.Admin.FullName) > 255 {
		return invalid("admin.full_name", "Administrator full name is required and must not exceed 255 characters.")
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

func (service *Service) CreateCharger(
	ctx context.Context,
	principal auth.Principal,
	request CreateChargerRequest,
) (ChargerView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerView{}, err
	}

	cpoID := *principal.CPOID
	request = normalizeCreateChargerRequest(request)
	if err := validateCreateChargerRequest(request); err != nil {
		return ChargerView{}, err
	}

	var record models.Charger
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hub models.Hub
		if err := tx.First(&hub, "id = ? AND cpo_id = ?", request.HubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		chargerID, err := generateUniqueChargerIDTx(tx)
		if err != nil {
			return err
		}

		ocppIdentity, err := generateUniqueOCPPIdentityTx(tx)
		if err != nil {
			return err
		}

		now := service.now()
		record = models.Charger{
			ID:           uuid.New(),
			CPOID:        cpoID,
			HubID:        request.HubID,
			ChargerID:    chargerID,
			OCPPIdentity: ocppIdentity,
			Vendor:       request.Vendor,
			Model:        request.Model,
			SerialNumber: request.SerialNumber,
			MaxPowerKW:   request.MaxPowerKW,
			Status:       constants.ChargerStatus("OFFLINE"),
			OCPPVersion:  "1.6J",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapChargerWriteError(err, "create charger")
		}

		for _, connector := range request.Connectors {
			connectorType := strings.TrimSpace(connector.ConnectorType)
			connectorRecord := models.Connector{
				ID:              uuid.New(),
				CPOID:           cpoID,
				ChargerID:       record.ID,
				ConnectorNumber: connector.ConnectorNumber,
				ConnectorType:   connectorType,
				MaxCurrent:      connector.MaxCurrent,
				MaxVoltage:      connector.MaxVoltage,
				Status:          constants.ChargerStatus("AVAILABLE"),
				CreatedAt:       now,
				UpdatedAt:       now,
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
		return ChargerView{}, err
	}

	return chargerView(record), nil
}

func (service *Service) GetCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
) (ChargerView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerView{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerView{}, &auth.APIError{
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
		First(&record, "cpo_id = ? AND charger_id = ?", *principal.CPOID, chargerID).Error; err != nil {
		return ChargerView{}, mapChargerNotFound(err)
	}
	return chargerView(record), nil
}

func (service *Service) UpdateCharger(
	ctx context.Context,
	principal auth.Principal,
	chargerID string,
	request UpdateChargerRequest,
) (ChargerView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return ChargerView{}, err
	}

	chargerID = normalizeChargerID(chargerID)
	if !chargerIDPattern.MatchString(chargerID) {
		return ChargerView{}, &auth.APIError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_charger_id",
			Message: "The charger ID is invalid.",
		}
	}

	request = normalizeUpdateChargerRequest(request)
	if err := validateUpdateChargerRequest(request); err != nil {
		return ChargerView{}, err
	}

	cpoID := *principal.CPOID
	var record models.Charger

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("connector_number ASC")
			}).
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
			record.HubID = *request.HubID
			changedFields["hub_id"] = *request.HubID
		}
		if request.Vendor != nil {
			updates["vendor"] = *request.Vendor
			record.Vendor = *request.Vendor
			changedFields["vendor"] = *request.Vendor
		}
		if request.Model != nil {
			updates["model"] = *request.Model
			record.Model = *request.Model
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
					return &auth.APIError{
						Status:  http.StatusBadRequest,
						Code:    "invalid_connector_id",
						Message: "Connector ID is required.",
					}
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
				if connectorReq.MaxCurrent != nil {
					if *connectorReq.MaxCurrent < 0 {
						return invalid("max_current", "Max current cannot be negative.")
					}
					connUpdates["max_current"] = *connectorReq.MaxCurrent
					existing.MaxCurrent = *connectorReq.MaxCurrent
					connChanged = true
				}
				if connectorReq.MaxVoltage != nil {
					if *connectorReq.MaxVoltage < 0 {
						return invalid("max_voltage", "Max voltage cannot be negative.")
					}
					connUpdates["max_voltage"] = *connectorReq.MaxVoltage
					existing.MaxVoltage = *connectorReq.MaxVoltage
					connChanged = true
				}
				if !connChanged {
					return &auth.APIError{
						Status:  http.StatusBadRequest,
						Code:    "invalid_request",
						Message: "At least one connector field must be supplied.",
					}
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
		return ChargerView{}, err
	}

	return chargerView(record), nil
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
	if request.HubID == uuid.Nil {
		return invalid("hub_id", "Hub ID is required.")
	}
	if request.Vendor == "" || len(request.Vendor) > 100 {
		return invalid("vendor", "Vendor is required and must not exceed 100 characters.")
	}
	if request.Model == "" || len(request.Model) > 100 {
		return invalid("model", "Model is required and must not exceed 100 characters.")
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
		if connector.MaxCurrent < 0 {
			return invalid("max_current", "Max current cannot be negative.")
		}
		if connector.MaxVoltage < 0 {
			return invalid("max_voltage", "Max voltage cannot be negative.")
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

	if request.Vendor != nil && (*request.Vendor == "" || len(*request.Vendor) > 100) {
		return invalid("vendor", "Vendor must not exceed 100 characters.")
	}
	if request.Model != nil && (*request.Model == "" || len(*request.Model) > 100) {
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
			if connector.MaxCurrent != nil {
				if *connector.MaxCurrent < 0 {
					return invalid("max_current", "Max current cannot be negative.")
				}
				changed = true
			}
			if connector.MaxVoltage != nil {
				if *connector.MaxVoltage < 0 {
					return invalid("max_voltage", "Max voltage cannot be negative.")
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

func generateUniqueOCPPIdentityTx(tx *gorm.DB) (string, error) {
	for i := 0; i < 32; i++ {
		randomHex, err := security.RandomHex(3)
		if err != nil {
			return "", err
		}
		candidate := "ocpp_" + strings.ToLower(randomHex)

		var existing models.Charger
		err = tx.Select("id").
			Where("ocpp_identity = ?", candidate).
			Take(&existing).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check ocpp identity uniqueness: %w", err)
		}
	}

	return "", &auth.APIError{
		Status:  http.StatusConflict,
		Code:    "charger_conflict",
		Message: "Unable to generate a unique OCPP identity.",
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
	if errors.As(err, &postgresError) && postgresError.Code == "23503" {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "charger_in_use",
			Message: "The charger cannot be deleted because it has dependent records.",
		}
	}
	return fmt.Errorf("delete charger: %w", err)
}

func chargerView(record models.Charger) ChargerView {
	connectorsView := make([]ConnectorView, 0, len(record.Connectors))

	for _, conn := range record.Connectors {
		connectorsView = append(connectorsView, ConnectorView{
			ID:              conn.ID,
			CPOID:           conn.CPOID,
			ChargerID:       conn.ChargerID,
			ConnectorNumber: conn.ConnectorNumber,
			ConnectorType:   conn.ConnectorType,
			MaxCurrent:      conn.MaxCurrent,
			MaxVoltage:      conn.MaxVoltage,
			Status:          conn.Status,
			CreatedAt:       conn.CreatedAt,
			UpdatedAt:       conn.UpdatedAt,
		})
	}

	return ChargerView{
		ID:           record.ID,
		CPOID:        record.CPOID,
		HubID:        record.HubID,
		ChargerID:    record.ChargerID,
		OCPPIdentity: record.OCPPIdentity,
		Vendor:       record.Vendor,
		Model:        record.Model,
		SerialNumber: record.SerialNumber,
		MaxPowerKW:   record.MaxPowerKW,
		Status:       record.Status,
		OCPPVersion:  record.OCPPVersion,
		LastSeenAt:   record.LastSeenAt,
		Connectors:   connectorsView,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
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
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&chargers).Error; err != nil {
		return ChargerListResponse{}, fmt.Errorf("list chargers: %w", err)
	}

	hasMore := len(chargers) > query.Limit
	if hasMore {
		chargers = chargers[:query.Limit]
	}
	response := make([]ChargerView, 0, len(chargers))
	for _, charger := range chargers {
		response = append(response, chargerView(charger))
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

	cpoID := *principal.CPOID
	var record models.Hub

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()

		record = models.Hub{
			ID:          uuid.New(),
			CPOID:       cpoID,
			Name:        request.Name,
			Address:     request.Address,
			Latitude:    *request.Latitude,
			Longitude:   *request.Longitude,
			Open24Hours: open24Hours,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapHubWriteError(err, "create hub")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"HUB_CREATED",
			models.JSONB{
				"hub_id":        record.ID,
				"name":          record.Name,
				"open_24_hours": record.Open24Hours,
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
) (HubView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return HubView{}, err
	}

	var record models.Hub
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, hubID).Error; err != nil {
		return HubView{}, mapHubNotFound(err)
	}

	return hubView(record), nil
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

	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "cpo_id = ? AND id = ?", cpoID, hubID).Error; err != nil {
			return mapHubNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.Name != nil {
			updates["name"] = *request.Name
			record.Name = *request.Name
			changedFields["name"] = *request.Name
		}
		if request.Address != nil {
			updates["address"] = *request.Address
			record.Address = *request.Address
			changedFields["address"] = *request.Address
		}
		if request.Latitude != nil {
			updates["latitude"] = *request.Latitude
			record.Latitude = *request.Latitude
			changedFields["latitude"] = *request.Latitude
		}
		if request.Longitude != nil {
			updates["longitude"] = *request.Longitude
			record.Longitude = *request.Longitude
			changedFields["longitude"] = *request.Longitude
		}
		if request.Open24Hours != nil {
			updates["open_24_hours"] = *request.Open24Hours
			record.Open24Hours = *request.Open24Hours
			changedFields["open_24_hours"] = *request.Open24Hours
		}

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one hub field must be supplied.",
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

	return hubView(record), nil
}

func normalizeCreateHubRequest(request CreateHubRequest) CreateHubRequest {
	request.Name = strings.TrimSpace(request.Name)
	request.Address = strings.TrimSpace(request.Address)
	return request
}

func normalizeUpdateHubRequest(request UpdateHubRequest) UpdateHubRequest {
	request.Name = trimOptionalString(request.Name)
	request.Address = trimOptionalString(request.Address)
	return request
}

func validateCreateHubRequest(request CreateHubRequest) error {
	if request.Name == "" || len(request.Name) > 255 {
		return invalid("name", "Hub name is required and must not exceed 255 characters.")
	}
	if request.Address == "" || len(request.Address) > 5000 {
		return invalid("address", "Hub address is required and must not exceed 5000 characters.")
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
	return nil
}

func validateUpdateHubRequest(request UpdateHubRequest) error {
	if request.Name == nil &&
		request.Address == nil &&
		request.Latitude == nil &&
		request.Longitude == nil &&
		request.Open24Hours == nil {
		return invalid("hub", "At least one hub field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 255) {
		return invalid("name", "Hub name must not exceed 255 characters.")
	}
	if request.Address != nil && (*request.Address == "" || len(*request.Address) > 5000) {
		return invalid("address", "Hub address must not exceed 5000 characters.")
	}
	if request.Latitude != nil && (*request.Latitude < -90 || *request.Latitude > 90) {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude != nil && (*request.Longitude < -180 || *request.Longitude > 180) {
		return invalid("longitude", "Longitude must be between -180 and 180.")
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
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func hubView(record models.Hub) HubView {
	return HubView{
		ID:          record.ID,
		CPOID:       record.CPOID,
		Name:        record.Name,
		Address:     record.Address,
		Latitude:    record.Latitude,
		Longitude:   record.Longitude,
		Open24Hours: record.Open24Hours,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func (service *Service) CreateTariff(
	ctx context.Context,
	principal auth.Principal,
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
		var hub models.Hub
		if err := tx.First(&hub, "id = ? AND cpo_id = ?", request.HubID, cpoID).Error; err != nil {
			return mapHubNotFound(err)
		}

		if request.ChargerID != nil {
			var charger models.Charger
			if err := tx.First(&charger, "id = ? AND cpo_id = ?", *request.ChargerID, cpoID).Error; err != nil {
				return mapChargerNotFound(err)
			}
			if charger.HubID != request.HubID {
				return &auth.APIError{
					Status:  http.StatusBadRequest,
					Code:    "charger_hub_mismatch",
					Message: "The charger must belong to the selected hub.",
				}
			}
		}

		if request.GSTID != nil {
			var gst models.GST
			if err := tx.First(&gst, "id = ? AND cpo_id = ?", *request.GSTID, cpoID).Error; err != nil {
				return mapGSTNotFound(err)
			}
		}

		if request.UserGroupID != nil {
			var userGroup models.UserGroup
			if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", *request.UserGroupID, cpoID).Error; err != nil {
				return mapUserGroupNotFound(err)
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
			UserGroupID:   request.UserGroupID,
			PricePerKWh:   request.PricePerKWh,
			IdleFeePerMin: request.IdleFeePerMin,
			Currency:      request.Currency,
			IsActive:      isActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := tx.Create(&record).Error; err != nil {
			return mapTariffWriteError(err, "create tariff")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"TARIFF_CREATED",
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

	return tariffView(record), nil
}

func (service *Service) ListTariffs(
	ctx context.Context,
	principal auth.Principal,
	query TenantListQuery,
) (TariffListResponse, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffListResponse{}, err
	}
	query, err := validateTenantListQuery(query)
	if err != nil {
		return TariffListResponse{}, err
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
	var records []models.Tariff
	if err := databaseQuery.
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return TariffListResponse{}, fmt.Errorf("list tariffs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	tariffs := make([]TariffView, 0, len(records))
	for _, record := range records {
		tariffs = append(tariffs, tariffView(record))
	}
	response := TariffListResponse{Tariffs: tariffs, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		nextBefore := records[len(records)-1].CreatedAt
		nextBeforeID := records[len(records)-1].ID
		response.NextBefore = &nextBefore
		response.NextBeforeID = &nextBeforeID
	}
	return response, nil
}

func (service *Service) GetTariff(
	ctx context.Context,
	principal auth.Principal,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOAdminAccess(principal); err != nil {
		return TariffView{}, err
	}

	var record models.Tariff
	if err := service.database.WithContext(ctx).
		First(&record, "cpo_id = ? AND id = ?", *principal.CPOID, tariffID).Error; err != nil {
		return TariffView{}, mapTariffNotFound(err)
	}

	return tariffView(record), nil
}

func (service *Service) UpdateTariff(
	ctx context.Context,
	principal auth.Principal,
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
			First(&record, "cpo_id = ? AND id = ?", cpoID, tariffID).Error; err != nil {
			return mapTariffNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		effectiveHubID := record.HubID
		if request.HubID != nil {
			var hub models.Hub
			if err := tx.First(&hub, "id = ? AND cpo_id = ?", *request.HubID, cpoID).Error; err != nil {
				return mapHubNotFound(err)
			}
			effectiveHubID = *request.HubID
			updates["hub_id"] = *request.HubID
			record.HubID = *request.HubID
			changedFields["hub_id"] = *request.HubID
		}

		effectiveChargerID := record.ChargerID
		if request.ChargerID != nil {
			var charger models.Charger
			if err := tx.First(&charger, "id = ? AND cpo_id = ?", *request.ChargerID, cpoID).Error; err != nil {
				return mapChargerNotFound(err)
			}
			if charger.HubID != effectiveHubID {
				return &auth.APIError{
					Status:  http.StatusBadRequest,
					Code:    "charger_hub_mismatch",
					Message: "The charger must belong to the selected hub.",
				}
			}
			effectiveChargerID = request.ChargerID
			updates["charger_id"] = request.ChargerID
			record.ChargerID = request.ChargerID
			changedFields["charger_id"] = request.ChargerID
		} else if effectiveChargerID != nil && request.HubID != nil {
			var charger models.Charger
			if err := tx.First(&charger, "id = ? AND cpo_id = ?", *effectiveChargerID, cpoID).Error; err != nil {
				return mapChargerNotFound(err)
			}
			if charger.HubID != effectiveHubID {
				return &auth.APIError{
					Status:  http.StatusBadRequest,
					Code:    "charger_hub_mismatch",
					Message: "The existing charger must belong to the selected hub.",
				}
			}
		}

		if request.GSTID != nil {
			var gst models.GST
			if err := tx.First(&gst, "id = ? AND cpo_id = ?", *request.GSTID, cpoID).Error; err != nil {
				return mapGSTNotFound(err)
			}
			updates["gst_id"] = request.GSTID
			record.GSTID = request.GSTID
			changedFields["gst_id"] = request.GSTID
		}

		if request.UserGroupID != nil {
			var userGroup models.UserGroup
			if err := tx.First(&userGroup, "id = ? AND cpo_id = ?", *request.UserGroupID, cpoID).Error; err != nil {
				return mapUserGroupNotFound(err)
			}
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
		if request.IsActive != nil {
			updates["is_active"] = *request.IsActive
			record.IsActive = *request.IsActive
			changedFields["is_active"] = *request.IsActive
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

		if err := tx.Model(&models.Tariff{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapTariffWriteError(err, "update tariff")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"TARIFF_UPDATED",
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

	return tariffView(record), nil
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
		request.IsActive == nil {
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

func mapTariffNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &auth.APIError{
			Status:  http.StatusNotFound,
			Code:    "tariff_not_found",
			Message: "The tariff was not found.",
		}
	}
	return fmt.Errorf("load tariff: %w", err)
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

func mapTariffWriteError(err error, operation string) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "tariff_conflict",
				Message: "The tariff already exists or conflicts with another record.",
			}
		case "23503":
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "tariff_conflict",
				Message: "The tariff references an invalid related record.",
			}
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func tariffView(record models.Tariff) TariffView {
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
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
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
			SGSTRate:  *request.SGSTRate,
			CGSTRate:  *request.CGSTRate,
			IGSTRate:  *request.IGSTRate,
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
		if request.SGSTRate != nil {
			updates["sgst_rate"] = *request.SGSTRate
			record.SGSTRate = *request.SGSTRate
			changedFields["sgst_rate"] = *request.SGSTRate
		}
		if request.CGSTRate != nil {
			updates["cgst_rate"] = *request.CGSTRate
			record.CGSTRate = *request.CGSTRate
			changedFields["cgst_rate"] = *request.CGSTRate
		}
		if request.IGSTRate != nil {
			updates["igst_rate"] = *request.IGSTRate
			record.IGSTRate = *request.IGSTRate
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
	return request
}

func normalizeUpdateGSTRequest(request UpdateGSTRequest) UpdateGSTRequest {
	request.Name = trimOptionalString(request.Name)
	return request
}

func validateCreateGSTRequest(request CreateGSTRequest) error {
	if request.Name == "" || len(request.Name) > 100 {
		return invalid("name", "GST name is required and must not exceed 100 characters.")
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
		request.SGSTRate == nil &&
		request.CGSTRate == nil &&
		request.IGSTRate == nil &&
		request.IsActive == nil {
		return invalid("gst", "At least one GST field must be supplied.")
	}

	if request.Name != nil && (*request.Name == "" || len(*request.Name) > 100) {
		return invalid("name", "GST name must not exceed 100 characters.")
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
		SGSTRate:  record.SGSTRate,
		CGSTRate:  record.CGSTRate,
		IGSTRate:  record.IGSTRate,
		IsActive:  record.IsActive,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
