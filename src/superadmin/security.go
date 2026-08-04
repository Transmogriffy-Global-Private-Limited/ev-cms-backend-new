package superadmin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (service *Service) ListLockedIdentities(ctx context.Context, principal auth.Principal, query SecurityQuery) (LockedIdentityPage, error) {
	if err := requirePlatform(principal); err != nil {
		return LockedIdentityPage{}, err
	}
	page, err := pageQuery(query.PageQuery)
	if err != nil {
		return LockedIdentityPage{}, err
	}
	now := service.now()
	database := service.database.WithContext(ctx).Where("locked_until > ?", now).Order("locked_until ASC, id ASC").Limit(page.Limit + 1)
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("locked_until > ?", page.Before.UTC())
		} else {
			database = database.Where("(locked_until > ?) OR (locked_until = ? AND id > ?)", page.Before.UTC(), page.Before.UTC(), *page.BeforeID)
		}
	}
	var users []models.User
	if err := database.Find(&users).Error; err != nil {
		return LockedIdentityPage{}, fmt.Errorf("list locked identities: %w", err)
	}
	hasMore := len(users) > page.Limit
	if hasMore {
		users = users[:page.Limit]
	}
	result := make([]LockedIdentityView, 0, len(users))
	for _, user := range users {
		if user.LockedUntil == nil {
			continue
		}
		result = append(result, LockedIdentityView{UserID: user.ID, Email: user.Email, FullName: user.FullName, LockedUntil: *user.LockedUntil})
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(result) > 0 {
		next = &result[len(result)-1].LockedUntil
		nextID = &result[len(result)-1].UserID
	}
	return LockedIdentityPage{Identities: result, NextBefore: next, NextBeforeID: nextID, HasMore: hasMore}, nil
}

func (service *Service) UnlockIdentity(ctx context.Context, principal auth.Principal, userID uuid.UUID, request ReasonRequest) error {
	if err := requirePlatform(principal); err != nil {
		return err
	}
	if userID == uuid.Nil {
		return invalid("user_id", "User ID is invalid.")
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return err
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("identity_not_found", "The identity was not found.")
			}
			return err
		}
		now := service.now()
		if user.LockedUntil == nil || !user.LockedUntil.After(now) {
			return conflict("identity_not_locked", "The identity is not currently locked.")
		}
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{"failed_login_attempts": 0, "locked_until": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := service.audit(tx, principal.UserID, "SECURITY_IDENTITY_UNLOCKED", "USER", &userID, models.JSONB{"reason": reason}, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.security.identity_unlocked", "USER", userID, models.JSONB{"reason": reason})
	})
}

func (service *Service) RevokeUserSessions(ctx context.Context, principal auth.Principal, userID uuid.UUID, request SessionRevocationRequest) (SessionRevocationResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return SessionRevocationResponse{}, err
	}
	if userID == uuid.Nil {
		return SessionRevocationResponse{}, invalid("user_id", "User ID is invalid.")
	}
	reason, err := validateReason(request.Reason)
	if err != nil {
		return SessionRevocationResponse{}, err
	}
	scope := request.Scope
	if scope == "" {
		scope = constants.AuthScopePlatform
	}
	if scope != constants.AuthScopePlatform && scope != constants.AuthScopeCPO && scope != "ALL" {
		return SessionRevocationResponse{}, invalid("scope", "Scope must be PLATFORM, CPO, or ALL.")
	}
	if scope == constants.AuthScopeCPO && request.CPOID == nil {
		return SessionRevocationResponse{}, invalid("cpo_id", "cpo_id is required for CPO scope.")
	}
	var scopePtr *constants.AuthScope
	if scope != "ALL" {
		scopePtr = &scope
	}
	var response SessionRevocationResponse
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return notFound("identity_not_found", "The identity was not found.")
		}
		now := service.now()
		sessions, refresh, revokeErr := auth.RevokeUserSessionsTx(tx, userID, scopePtr, request.CPOID, "PLATFORM_SECURITY_REVOKED", now)
		if revokeErr != nil {
			return revokeErr
		}
		response = SessionRevocationResponse{RevokedSessions: sessions, RevokedRefreshTokens: refresh}
		data := models.JSONB{"reason": reason, "scope": scope, "revoked_sessions": sessions, "revoked_refresh_tokens": refresh}
		if request.CPOID != nil {
			data["cpo_id"] = request.CPOID.String()
		}
		if err := service.audit(tx, principal.UserID, "SECURITY_SESSIONS_REVOKED", "USER", &userID, data, now); err != nil {
			return err
		}
		return service.emit(tx, principal.UserID, "platform.security.sessions_revoked", "USER", userID, data)
	})
	return response, err
}
