package cpo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
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

const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomChargerID() string {
	b := make([]byte, 6)

	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}

	return string(b)
}

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

var chargerIDPattern = regexp.MustCompile(`^[a-z0-9]{6}$`)

func (service *Service) CreateCharger(
	ctx context.Context,
	principal auth.Principal,
	request CreateChargerRequest,
) (ChargerView, error) {
	if err := requireCPOChargerAccess(principal); err != nil {
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

		return writeAudit(
			tx,
			principal.UserID,
			cpoID,
			"CHARGER_CREATED",
			models.JSONB{
				"charger_id":    record.ChargerID,
				"ocpp_identity": record.OCPPIdentity,
				"hub_id":        record.HubID,
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
	if err := requireCPOChargerAccess(principal); err != nil {
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
	if err := requireCPOChargerAccess(principal); err != nil {
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

		if len(changedFields) == 0 {
			return &auth.APIError{
				Status:  http.StatusBadRequest,
				Code:    "invalid_request",
				Message: "At least one charger field must be supplied.",
			}
		}

		now := service.now()
		updates["updated_at"] = now
		record.UpdatedAt = now

		if err := tx.Model(&models.Charger{}).
			Where("id = ?", record.ID).
			Updates(updates).Error; err != nil {
			return mapChargerWriteError(err, "update charger")
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
	request DeleteChargerRequest,
) error {
	if err := requireCPOChargerAccess(principal); err != nil {
		return err
	}

	chargerID := normalizeChargerID(request.ChargerID)
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
			First(&record, "cpo_id = ? AND charger_id = ?", cpoID, chargerID).Error; err != nil {
			return mapChargerNotFound(err)
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
	return request
}

func normalizeUpdateChargerRequest(request UpdateChargerRequest) UpdateChargerRequest {
	request.Vendor = trimOptionalString(request.Vendor)
	request.Model = trimOptionalString(request.Model)
	request.SerialNumber = trimOptionalString(request.SerialNumber)
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
	return nil
}

func validateUpdateChargerRequest(request UpdateChargerRequest) error {
	if request.HubID == nil &&
		request.Vendor == nil &&
		request.Model == nil &&
		request.SerialNumber == nil &&
		request.MaxPowerKW == nil {
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
	return nil
}

func requireCPOChargerAccess(principal auth.Principal) error {
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
	switch *principal.Role {
	case constants.CPORoleOwner, constants.CPORoleAdmin, constants.CPORoleOperator:
		return nil
	default:
		return &auth.APIError{
			Status:  http.StatusForbidden,
			Code:    "forbidden",
			Message: "CPO owner, admin, or operator access is required.",
		}
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

		chargerID := randomChargerID()
		candidate := "ocpp_" + strings.ToLower(chargerID)

		var existing models.Charger
		err := tx.Select("id").
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

func mapChargerDeleteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23503" {
		return &auth.APIError{
			Status:  http.StatusConflict,
			Code:    "charger_conflict",
			Message: "The charger cannot be deleted because it has dependent records.",
		}
	}
	return fmt.Errorf("delete charger: %w", err)
}

func chargerView(record models.Charger) ChargerView {
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
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

func (service *Service) ListChargers(
	ctx context.Context,
	principal auth.Principal,
) ([]ChargerView, error) {

	if err := requireCPOChargerAccess(principal); err != nil {
		return nil, err
	}

	var chargers []models.Charger

	if err := service.database.WithContext(ctx).
		Where("cpo_id = ?", *principal.CPOID).
		Order("created_at DESC").
		Find(&chargers).Error; err != nil {
		return nil, mapChargerWriteError(err, "list chargers")
	}

	response := make([]ChargerView, 0, len(chargers))

	for _, charger := range chargers {
		response = append(response, chargerView(charger))
	}

	return response, nil
}

func (service *Service) CreateHub(
	ctx context.Context,
	principal auth.Principal,
	request CreateHubRequest,
) (HubView, error) {
	if err := requireCPOChargerAccess(principal); err != nil {
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
			Latitude:    request.Latitude,
			Longitude:   request.Longitude,
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

func (service *Service) GetHub(
	ctx context.Context,
	principal auth.Principal,
	hubID uuid.UUID,
) (HubView, error) {
	if err := requireCPOChargerAccess(principal); err != nil {
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
	if err := requireCPOChargerAccess(principal); err != nil {
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
	if request.Latitude < -90 || request.Latitude > 90 {
		return invalid("latitude", "Latitude must be between -90 and 90.")
	}
	if request.Longitude < -180 || request.Longitude > 180 {
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
	if err := requireCPOChargerAccess(principal); err != nil {
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

func (service *Service) GetTariff(
	ctx context.Context,
	principal auth.Principal,
	tariffID uuid.UUID,
) (TariffView, error) {
	if err := requireCPOChargerAccess(principal); err != nil {
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
	if err := requireCPOChargerAccess(principal); err != nil {
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
	if request.Currency == "" {
		request.Currency = "INR"
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
