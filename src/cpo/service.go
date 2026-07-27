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
	cpoRecord := models.CPO{
		ID:             uuid.New(),
		Slug:           request.Slug,
		BusinessName:   request.BusinessName,
		CompanyType:    request.CompanyType,
		GSTIN:          request.GSTIN,
		Address:        request.Address,
		City:           request.City,
		State:          request.State,
		Pincode:        request.Pincode,
		Status:         constants.CPOStatusPending,
		AppID:          dummyAppID,
		AppIDMode:      constants.CPOAppIDModeDummy,
		AppIDUpdatedAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
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
			if err := service.outbox.EnqueueMessage(
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
			); err != nil {
				return err
			}
		default:
			return fmt.Errorf("find initial CPO administrator: %w", result.Error)
		}

		membership := models.CPOMembership{
			ID:        uuid.New(),
			CPOID:     cpoRecord.ID,
			UserID:    admin.ID,
			Role:      constants.CPORoleAdmin,
			Status:    constants.MembershipStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return mapWriteError(err, "create initial CPO administrator membership")
		}
		if !identityCreated {
			if err := service.outbox.EnqueueMessage(
				tx,
				admin.Email,
				"CPO_MEMBERSHIP_ASSIGNED",
				cmsmail.MessagePayload{
					RecipientName: admin.FullName,
					CPOName:       cpoRecord.BusinessName,
					CPOID:         cpoRecord.ID.String(),
					CPOAppID:      cpoRecord.AppID,
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
) ([]View, error) {
	if err := requirePlatform(principal); err != nil {
		return nil, err
	}
	var records []models.CPO
	if err := service.database.WithContext(ctx).
		Order("created_at DESC").
		Limit(100).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list CPOs: %w", err)
	}
	result := make([]View, 0, len(records))
	for _, record := range records {
		result = append(result, view(record))
	}
	return result, nil
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

func (service *Service) Activate(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (View, error) {
	return service.transitionStatus(ctx, principal, cpoID, constants.CPOStatusActive)
}

func (service *Service) Suspend(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
) (View, error) {
	return service.transitionStatus(ctx, principal, cpoID, constants.CPOStatusSuspended)
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

func (service *Service) transitionStatus(
	ctx context.Context,
	principal auth.Principal,
	cpoID uuid.UUID,
	status constants.CPOStatus,
) (View, error) {
	if err := requirePlatform(principal); err != nil {
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
		if record.Status != status {
			if err := tx.Model(&models.CPO{}).
				Where("id = ?", cpoID).
				Updates(map[string]any{
					"status":     status,
					"updated_at": now,
				}).Error; err != nil {
				return fmt.Errorf("update CPO status: %w", err)
			}
			record.Status = status
			record.UpdatedAt = now
		}
		if status == constants.CPOStatusSuspended {
			reason := "CPO_SUSPENDED"
			if err := tx.Model(&models.AuthSession{}).
				Where("cpo_id = ? AND revoked_at IS NULL", cpoID).
				Updates(map[string]any{
					"revoked_at":    now,
					"revoke_reason": reason,
				}).Error; err != nil {
				return fmt.Errorf("revoke suspended CPO sessions: %w", err)
			}
			if err := tx.Exec(`
				UPDATE auth_refresh_tokens
				SET revoked_at = ?
				WHERE used_at IS NULL
				  AND revoked_at IS NULL
				  AND session_id IN (
				      SELECT id FROM auth_sessions WHERE cpo_id = ?
				  )
			`, now, cpoID).Error; err != nil {
				return fmt.Errorf("revoke suspended CPO refresh tokens: %w", err)
			}
		}
		if err := writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_STATUS_"+string(status),
			models.JSONB{"status": status},
			now,
		); err != nil {
			return err
		}
		if !changed {
			return nil
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
			models.JSONB{"status": status},
		)
	})
	if err != nil {
		return View{}, err
	}
	return view(record), nil
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
		ID:             record.ID,
		Slug:           record.Slug,
		BusinessName:   record.BusinessName,
		CompanyType:    record.CompanyType,
		GSTIN:          record.GSTIN,
		Address:        record.Address,
		City:           record.City,
		State:          record.State,
		Pincode:        record.Pincode,
		Status:         record.Status,
		AppID:          record.AppID,
		AppIDMode:      record.AppIDMode,
		AppIDUpdatedAt: record.AppIDUpdatedAt,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func (service *Service) CreateProfile(
	ctx context.Context,
	principal auth.Principal,
	request CreateProfileRequest,
) (View, error) {
	if err := requireCPOProfileAccess(principal); err != nil {
		return View{}, err
	}

	cpoID := *principal.CPOID

	request = normalizeCreateProfileRequest(request)
	if err := validateCreateProfileRequest(request); err != nil {
		return View{}, err
	}

	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}

		if record.BusinessName != "" ||
			record.Address != "" ||
			record.City != "" ||
			record.State != "" ||
			record.Pincode != "" ||
			record.CompanyType != "" ||
			(record.GSTIN != nil && *record.GSTIN != "") {
			return &auth.APIError{
				Status:  http.StatusConflict,
				Code:    "profile_exists",
				Message: "CPO profile already exists.",
			}
		}

		now := service.now()

		gstin := request.GSTIN
		record.BusinessName = request.BusinessName
		record.CompanyType = request.CompanyType
		record.GSTIN = &gstin
		record.Address = request.Address
		record.City = request.City
		record.State = request.State
		record.Pincode = request.Pincode
		record.UpdatedAt = now

		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(map[string]any{
				"business_name": record.BusinessName,
				"company_type":  record.CompanyType,
				"gstin":         record.GSTIN,
				"address":       record.Address,
				"city":          record.City,
				"state":         record.State,
				"pincode":       record.Pincode,
				"updated_at":    now,
			}).Error; err != nil {
			return mapWriteError(err, "create CPO profile")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PROFILE_CREATED",
			models.JSONB{
				"business_name": record.BusinessName,
				"company_type":  record.CompanyType,
			},
			now,
		)
	})
	if err != nil {
		return View{}, err
	}

	return view(record), nil
}

func (service *Service) GetProfile(
	ctx context.Context,
	principal auth.Principal,
) (View, error) {
	if err := requireCPOProfileAccess(principal); err != nil {
		return View{}, err
	}

	record, err := service.find(ctx, *principal.CPOID)
	if err != nil {
		return View{}, err
	}
	return view(record), nil
}

func (service *Service) UpdateProfile(
	ctx context.Context,
	principal auth.Principal,
	request UpdateProfileRequest,
) (View, error) {
	if err := requireCPOProfileAccess(principal); err != nil {
		return View{}, err
	}

	cpoID := *principal.CPOID

	request = normalizeUpdateProfileRequest(request)
	if err := validateUpdateProfileRequest(request); err != nil {
		return View{}, err
	}

	var record models.CPO
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", cpoID).Error; err != nil {
			return mapNotFound(err)
		}

		updates := map[string]any{}
		changedFields := models.JSONB{}

		if request.BusinessName != nil {
			updates["business_name"] = *request.BusinessName
			record.BusinessName = *request.BusinessName
			changedFields["business_name"] = *request.BusinessName
		}
		if request.CompanyType != nil {
			updates["company_type"] = *request.CompanyType
			record.CompanyType = *request.CompanyType
			changedFields["company_type"] = *request.CompanyType
		}
		if request.GSTIN != nil {
			updates["gstin"] = *request.GSTIN
			record.GSTIN = request.GSTIN
			changedFields["gstin"] = *request.GSTIN
		}
		if request.Address != nil {
			updates["address"] = *request.Address
			record.Address = *request.Address
			changedFields["address"] = *request.Address
		}
		if request.City != nil {
			updates["city"] = *request.City
			record.City = *request.City
			changedFields["city"] = *request.City
		}
		if request.State != nil {
			updates["state"] = *request.State
			record.State = *request.State
			changedFields["state"] = *request.State
		}
		if request.Pincode != nil {
			updates["pincode"] = *request.Pincode
			record.Pincode = *request.Pincode
			changedFields["pincode"] = *request.Pincode
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one profile field must be supplied.",
			}
		}

		if err := tx.Model(&models.CPO{}).
			Where("id = ?", cpoID).
			Updates(updates).Error; err != nil {
			return mapWriteError(err, "update CPO profile")
		}

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CPO_PROFILE_UPDATED",
			models.JSONB{"fields": changedFields},
			now,
		)
	})
	if err != nil {
		return View{}, err
	}

	return view(record), nil
}

func normalizeCreateProfileRequest(request CreateProfileRequest) CreateProfileRequest {
	request.BusinessName = strings.TrimSpace(request.BusinessName)
	request.CompanyType = constants.CPOCompanyType(strings.TrimSpace(string(request.CompanyType)))
	request.GSTIN = strings.ToUpper(strings.TrimSpace(request.GSTIN))
	request.Address = strings.TrimSpace(request.Address)
	request.City = strings.TrimSpace(request.City)
	request.State = strings.TrimSpace(request.State)
	request.Pincode = strings.TrimSpace(request.Pincode)
	return request
}

func normalizeUpdateProfileRequest(request UpdateProfileRequest) UpdateProfileRequest {
	request.BusinessName = trimOptionalString(request.BusinessName)
	request.Address = trimOptionalString(request.Address)
	request.City = trimOptionalString(request.City)
	request.State = trimOptionalString(request.State)
	request.Pincode = trimOptionalString(request.Pincode)

	if request.GSTIN != nil {
		value := strings.ToUpper(strings.TrimSpace(*request.GSTIN))
		request.GSTIN = &value
	}

	return request
}

func validateCreateProfileRequest(request CreateProfileRequest) error {
	if request.BusinessName == "" || len(request.BusinessName) > 255 {
		return invalid("business_name", "Business name is required and must not exceed 255 characters.")
	}
	if !request.CompanyType.Valid() {
		return invalid("company_type", "Company type must be INDIVIDUAL or COMPANY.")
	}
	if request.GSTIN == "" || !gstinPattern.MatchString(request.GSTIN) {
		return invalid("gstin", "GSTIN must contain 15 uppercase letters or digits.")
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

func validateUpdateProfileRequest(request UpdateProfileRequest) error {
	if request.BusinessName == nil &&
		request.CompanyType == nil &&
		request.GSTIN == nil &&
		request.Address == nil &&
		request.City == nil &&
		request.State == nil &&
		request.Pincode == nil {
		return invalid("profile", "At least one profile field must be supplied.")
	}

	if request.BusinessName != nil {
		if *request.BusinessName == "" || len(*request.BusinessName) > 255 {
			return invalid("business_name", "Business name must not exceed 255 characters.")
		}
	}
	if request.CompanyType != nil && !request.CompanyType.Valid() {
		return invalid("company_type", "Company type must be INDIVIDUAL or COMPANY.")
	}
	if request.GSTIN != nil {
		if *request.GSTIN == "" || !gstinPattern.MatchString(*request.GSTIN) {
			return invalid("gstin", "GSTIN must contain 15 uppercase letters or digits.")
		}
	}
	if request.Address != nil && len(*request.Address) > 5000 {
		return invalid("address", "Address must not exceed 5000 characters.")
	}
	if request.City != nil && len(*request.City) > 100 {
		return invalid("city", "City must not exceed 100 characters.")
	}
	if request.State != nil && len(*request.State) > 100 {
		return invalid("state", "State must not exceed 100 characters.")
	}
	if request.Pincode != nil && len(*request.Pincode) > 10 {
		return invalid("pincode", "Pincode must not exceed 10 characters.")
	}
	return nil
}

func requireCPOProfileAccess(principal auth.Principal) error {
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
	if principal.Role == nil || (*principal.Role != constants.CPORoleOwner && *principal.Role != constants.CPORoleAdmin) {
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO owner or admin access is required.",
		}
	}
	return nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
