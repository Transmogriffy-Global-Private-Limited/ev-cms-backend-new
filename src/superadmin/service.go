package superadmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
	maxReason       = 500
	maxTitle        = 200
	maxBody         = 10000
)

type Service struct {
	database   *gorm.DB
	events     *platformops.Service
	outbox     *cmsmail.Outbox
	mailEnable bool
	now        func() time.Time
}

func NewService(
	database *gorm.DB,
	events *platformops.Service,
	outbox *cmsmail.Outbox,
	mailEnabled bool,
) *Service {
	return &Service{
		database: database, events: events, outbox: outbox,
		mailEnable: mailEnabled, now: func() time.Time { return time.Now().UTC() },
	}
}

func requirePlatform(principal auth.Principal) error {
	if principal.Scope != constants.AuthScopePlatform {
		return &auth.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "You do not have access to this operation."}
	}
	return nil
}

func invalid(field, message string) error {
	return &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_" + field, Message: message}
}

func notFound(code, message string) error {
	return &auth.APIError{Status: http.StatusNotFound, Code: code, Message: message}
}

func conflict(code, message string) error {
	return &auth.APIError{Status: http.StatusConflict, Code: code, Message: message}
}

func boundedLimit(value int) int {
	if value < 1 || value > maxPageSize {
		return defaultPageSize
	}
	return value
}

func validateReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 || len(reason) > maxReason {
		return "", invalid("reason", "Reason must contain between 3 and 500 characters.")
	}
	return reason, nil
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := netmail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 320 {
		return "", invalid("email", "Email must be a valid address.")
	}
	return value, nil
}

func pageQuery(query PageQuery) (PageQuery, error) {
	query.Limit = boundedLimit(query.Limit)
	if query.BeforeID != nil && query.Before == nil {
		return PageQuery{}, invalid("before_id", "before_id may be supplied only with before.")
	}
	return query, nil
}

func (service *Service) InviteOrGrant(
	ctx context.Context,
	principal auth.Principal,
	request InviteAdministratorRequest,
) (AdministratorView, error) {
	if err := requirePlatform(principal); err != nil {
		return AdministratorView{}, err
	}
	email, err := normalizeEmail(request.Email)
	if err != nil {
		return AdministratorView{}, err
	}
	fullName := strings.TrimSpace(request.FullName)
	if fullName == "" || len(fullName) > 255 {
		return AdministratorView{}, invalid("full_name", "Full name is required and must not exceed 255 characters.")
	}

	var view AdministratorView
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformAuthority(tx); err != nil {
			return err
		}
		now := service.now()
		var user models.User
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", email).First(&user)
		identityCreated := false
		temporaryPassword := ""
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			if !service.mailEnable || service.outbox == nil {
				return &auth.APIError{Status: http.StatusServiceUnavailable, Code: "mail_unavailable", Message: "Email delivery is required to invite a new platform administrator."}
			}
			temporaryPassword, err = security.RandomToken(24)
			if err != nil {
				return err
			}
			passwordHash, hashErr := security.HashPassword(temporaryPassword)
			if hashErr != nil {
				return hashErr
			}
			user = models.User{
				ID: uuid.New(), Email: email, FullName: fullName,
				PasswordHash: passwordHash, IsActive: true, IsVerified: true,
				MFAEnabled: true, MustChangePassword: true,
				PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("create platform administrator identity: %w", err)
			}
			identityCreated = true
		} else if result.Error != nil {
			return fmt.Errorf("find platform administrator identity: %w", result.Error)
		} else if !user.IsActive {
			return conflict("identity_inactive", "The global identity is inactive and must be restored separately.")
		}

		var admin models.PlatformAdmin
		adminResult := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&admin, "user_id = ?", user.ID)
		if errors.Is(adminResult.Error, gorm.ErrRecordNotFound) {
			admin = models.PlatformAdmin{
				UserID: user.ID, IsActive: true, StatusReason: "Authority granted",
				StatusChangedAt: now, StatusChangedByID: &principal.UserID,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&admin).Error; err != nil {
				return fmt.Errorf("grant platform authority: %w", err)
			}
		} else if adminResult.Error != nil {
			return fmt.Errorf("load platform authority: %w", adminResult.Error)
		} else if !admin.IsActive {
			admin.IsActive = true
			admin.StatusReason = "Authority restored"
			admin.StatusChangedAt = now
			admin.StatusChangedByID = &principal.UserID
			admin.UpdatedAt = now
			if err := tx.Save(&admin).Error; err != nil {
				return fmt.Errorf("restore platform authority: %w", err)
			}
		}

		if service.mailEnable && service.outbox != nil {
			template := "PLATFORM_ADMIN_GRANTED"
			payload := cmsmail.MessagePayload{RecipientName: user.FullName}
			if identityCreated {
				template = "PLATFORM_ADMIN_INVITE"
				payload.TemporaryPassword = temporaryPassword
			}
			if err := service.outbox.EnqueueMessageWithContext(tx, user.Email, template, payload, cmsmail.MessageContext{UserID: &user.ID}); err != nil {
				return err
			}
		}
		if err := service.audit(tx, principal.UserID, "PLATFORM_ADMIN_GRANTED", "PLATFORM_ADMIN", &user.ID, models.JSONB{"identity_created": identityCreated}, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, "platform.admin.granted", "PLATFORM_ADMIN", user.ID, models.JSONB{"identity_created": identityCreated}); err != nil {
			return err
		}
		view = administratorView(user, admin)
		return nil
	})
	return view, err
}

func (service *Service) ListAdministrators(
	ctx context.Context,
	principal auth.Principal,
	query AdministratorQuery,
) (AdministratorPage, error) {
	if err := requirePlatform(principal); err != nil {
		return AdministratorPage{}, err
	}
	page, err := pageQuery(query.PageQuery)
	if err != nil {
		return AdministratorPage{}, err
	}
	requested := page.Limit + 1
	database := service.database.WithContext(ctx).
		Model(&models.PlatformAdmin{}).
		Joins("JOIN users ON users.id = platform_admins.user_id").
		Order("platform_admins.created_at DESC, platform_admins.user_id DESC").
		Limit(requested)
	if !query.IncludeInactive {
		database = database.Where("platform_admins.is_active = true")
	}
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("platform_admins.created_at < ?", page.Before.UTC())
		} else {
			database = database.Where("(platform_admins.created_at < ?) OR (platform_admins.created_at = ? AND platform_admins.user_id < ?)", page.Before.UTC(), page.Before.UTC(), *page.BeforeID)
		}
	}
	var records []struct {
		models.PlatformAdmin
		Email            string
		FullName         string
		IdentityActive   bool
		IdentityVerified bool
	}
	if err := database.Select("platform_admins.*, users.email, users.full_name, users.is_active AS identity_active, users.is_verified AS identity_verified").Find(&records).Error; err != nil {
		return AdministratorPage{}, fmt.Errorf("list platform administrators: %w", err)
	}
	hasMore := len(records) > page.Limit
	if hasMore {
		records = records[:page.Limit]
	}
	result := make([]AdministratorView, 0, len(records))
	for _, record := range records {
		result = append(result, AdministratorView{
			UserID: record.UserID, Email: record.Email, FullName: record.FullName,
			IdentityActive: record.IdentityActive, IdentityVerified: record.IdentityVerified,
			AuthorityActive: record.IsActive, StatusReason: record.StatusReason,
			StatusChangedAt: record.StatusChangedAt, StatusChangedByID: record.StatusChangedByID,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		})
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(result) > 0 {
		next = &result[len(result)-1].CreatedAt
		nextID = &result[len(result)-1].UserID
	}
	return AdministratorPage{Administrators: result, NextBefore: next, NextBeforeID: nextID, HasMore: hasMore}, nil
}

func administratorView(user models.User, admin models.PlatformAdmin) AdministratorView {
	return AdministratorView{
		UserID: user.ID, Email: user.Email, FullName: user.FullName,
		IdentityActive: user.IsActive, IdentityVerified: user.IsVerified,
		AuthorityActive: admin.IsActive, StatusReason: admin.StatusReason,
		StatusChangedAt: admin.StatusChangedAt, StatusChangedByID: admin.StatusChangedByID,
		CreatedAt: admin.CreatedAt, UpdatedAt: admin.UpdatedAt,
	}
}

func (service *Service) SetAdministratorStatus(
	ctx context.Context,
	principal auth.Principal,
	userID uuid.UUID,
	active bool,
	request ReasonRequest,
) (AdministratorView, error) {
	if err := requirePlatform(principal); err != nil {
		return AdministratorView{}, err
	}
	if userID == uuid.Nil {
		return AdministratorView{}, invalid("user_id", "User ID is invalid.")
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return AdministratorView{}, err
	}
	var view AdministratorView
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformAuthority(tx); err != nil {
			return err
		}
		var admin models.PlatformAdmin
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&admin, "user_id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("platform_admin_not_found", "The platform administrator was not found.")
			}
			return err
		}
		var user models.User
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		if admin.IsActive == active {
			view = administratorView(user, admin)
			return nil
		}
		if !active {
			var activeAdmins []models.PlatformAdmin
			if err := tx.Where("is_active = true").Find(&activeAdmins).Error; err != nil {
				return err
			}
			if len(activeAdmins) <= 1 {
				return conflict("last_platform_admin", "The last active platform administrator cannot be deactivated.")
			}
		}
		now := service.now()
		admin.IsActive = active
		admin.StatusReason = reason
		admin.StatusChangedAt = now
		admin.StatusChangedByID = &principal.UserID
		admin.UpdatedAt = now
		if err := tx.Save(&admin).Error; err != nil {
			return err
		}
		if !active {
			if _, _, err := auth.RevokeUserSessionsTx(tx, userID, authScopePlatform(), nil, "PLATFORM_AUTHORITY_REVOKED", now); err != nil {
				return err
			}
		}
		action := "PLATFORM_ADMIN_ACTIVATED"
		event := "platform.admin.activated"
		if !active {
			action = "PLATFORM_ADMIN_DEACTIVATED"
			event = "platform.admin.deactivated"
		}
		if err := service.audit(tx, principal.UserID, action, "PLATFORM_ADMIN", &userID, models.JSONB{"reason": reason}, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, event, "PLATFORM_ADMIN", userID, models.JSONB{"reason": reason}); err != nil {
			return err
		}
		view = administratorView(user, admin)
		return nil
	})
	return view, err
}

func authScopePlatform() *constants.AuthScope { value := constants.AuthScopePlatform; return &value }

func lockPlatformAuthority(tx *gorm.DB) error {
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "ev_cms_platform_authority").Error; err != nil {
		return fmt.Errorf("lock platform authority: %w", err)
	}
	return nil
}

func (service *Service) audit(tx *gorm.DB, actor uuid.UUID, action, entity string, entityID *uuid.UUID, details models.JSONB, now time.Time) error {
	actorID := actor
	record := models.AuditLog{ID: uuid.New(), UserID: &actorID, Action: action, Entity: entity, EntityID: entityID, Details: details, CreatedAt: now}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("write superadmin audit: %w", err)
	}
	return nil
}

func (service *Service) emit(tx *gorm.DB, actor uuid.UUID, eventType, resourceType string, resourceID uuid.UUID, data models.JSONB) error {
	if service.events == nil {
		return nil
	}
	actorID := actor
	resource := resourceID.String()
	_, err := service.events.Emit(tx, platformops.EventInput{Type: eventType, ActorUserID: &actorID, ResourceType: resourceType, ResourceID: &resource, Data: data})
	return err
}

func (service *Service) emitCollection(tx *gorm.DB, actor uuid.UUID, eventType, resourceType string, data models.JSONB) error {
	if service.events == nil {
		return nil
	}
	actorID := actor
	_, err := service.events.Emit(tx, platformops.EventInput{Type: eventType, ActorUserID: &actorID, ResourceType: resourceType, Data: data})
	return err
}
