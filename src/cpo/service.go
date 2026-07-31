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
			currentMembership.Status != constants.MembershipStatusActive
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
		if targetFound &&
			(targetMembership.Role == constants.CPORoleAdmin ||
				targetMembership.Role == constants.CPORoleOwner) {
			role = targetMembership.Role
		}
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
