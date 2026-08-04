package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (service *Service) createSession(
	tx *gorm.DB,
	user models.User,
	scope constants.AuthScope,
	cpoID *uuid.UUID,
	role *constants.CPORole,
	metadata RequestMetadata,
	now time.Time,
) (TokenResponse, error) {
	appID, appIDMode, err := service.loadCPOAppIdentityTx(tx, scope, cpoID)
	if err != nil {
		return TokenResponse{}, err
	}
	session := models.AuthSession{
		ID:           uuid.New(),
		UserID:       user.ID,
		Scope:        scope,
		CPOID:        cpoID,
		Role:         role,
		TokenVersion: 1,
		IPAddress:    metadata.IPAddress,
		UserAgent:    boundedUserAgent(metadata.UserAgent),
		CreatedAt:    now,
		LastSeenAt:   now,
		ExpiresAt:    now.Add(service.config.SessionTTL),
	}
	if err := tx.Create(&session).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("create authentication session: %w", err)
	}
	refreshToken, err := security.RandomToken(32)
	if err != nil {
		return TokenResponse{}, err
	}
	refreshRecord := models.AuthRefreshToken{
		ID:        uuid.New(),
		SessionID: session.ID,
		TokenHash: hashRefreshToken(refreshToken),
		ExpiresAt: session.ExpiresAt,
		CreatedAt: now,
	}
	if err := tx.Create(&refreshRecord).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("store refresh token: %w", err)
	}
	accessToken, accessExpiry, err := service.tokens.Issue(
		now,
		user.ID,
		session.ID,
		scope,
		cpoID,
		role,
		session.TokenVersion,
	)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := tx.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"last_login_at": now,
			"mfa_enabled":   true,
			"is_verified":   true,
			"updated_at":    now,
		}).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("record successful login: %w", err)
	}
	if err := service.audit(
		tx,
		&user.ID,
		cpoID,
		"AUTH_LOGIN_SUCCEEDED",
		"AUTH_SESSION",
		&session.ID,
		models.JSONB{"scope": scope},
		now,
	); err != nil {
		return TokenResponse{}, err
	}
	if user.MustChangePassword {
		if err := service.outbox.EnqueueMessage(
			tx,
			user.Email,
			"PASSWORD_CHANGE_REMINDER",
			cmsmail.MessagePayload{RecipientName: user.FullName},
		); err != nil {
			return TokenResponse{}, err
		}
	}
	return TokenResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessExpiry,
		RefreshToken:         refreshToken,
		SessionExpiresAt:     session.ExpiresAt,
		TokenType:            "Bearer",
		CPOAppID:             appID,
		CPOAppIDMode:         appIDMode,
		MustChangePassword:   user.MustChangePassword,
	}, nil
}

func (service *Service) Refresh(
	ctx context.Context,
	request RefreshRequest,
	metadata RequestMetadata,
) (TokenResponse, error) {
	if request.RefreshToken == "" || len(request.RefreshToken) > 256 {
		return TokenResponse{}, errInvalidRefresh
	}
	if err := service.checkRateLimit(
		ctx,
		"REFRESH",
		rateLimitAddress(metadata),
	); err != nil {
		return TokenResponse{}, err
	}

	var response TokenResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AuthRefreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ?", hashRefreshToken(request.RefreshToken)).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				outcome = errInvalidRefresh
				return nil
			}
			return fmt.Errorf("lock refresh token: %w", err)
		}
		var session models.AuthSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&session, "id = ?", current.SessionID).Error; err != nil {
			return fmt.Errorf("load refresh session: %w", err)
		}
		now := service.now()
		if current.UsedAt != nil {
			if err := service.revokeSessionTx(tx, session.ID, "REFRESH_TOKEN_REUSE", now); err != nil {
				return err
			}
			outcome = errInvalidRefresh
			return nil
		}
		if current.RevokedAt != nil ||
			!now.Before(current.ExpiresAt) ||
			session.RevokedAt != nil ||
			!now.Before(session.ExpiresAt) {
			outcome = errInvalidRefresh
			return nil
		}
		user, role, appID, appIDMode, err := service.validateSessionContextTx(tx, session, now)
		if err != nil {
			if revokeErr := service.revokeSessionTx(tx, session.ID, "AUTHORITY_CHANGED", now); revokeErr != nil {
				return revokeErr
			}
			outcome = errInvalidRefresh
			return nil
		}

		replacementToken, err := security.RandomToken(32)
		if err != nil {
			return err
		}
		replacement := models.AuthRefreshToken{
			ID:        uuid.New(),
			SessionID: session.ID,
			TokenHash: hashRefreshToken(replacementToken),
			ExpiresAt: session.ExpiresAt,
			CreatedAt: now,
		}
		if err := tx.Create(&replacement).Error; err != nil {
			return fmt.Errorf("create replacement refresh token: %w", err)
		}
		if err := tx.Model(&models.AuthRefreshToken{}).
			Where("id = ? AND used_at IS NULL", current.ID).
			Updates(map[string]any{
				"used_at":        now,
				"replacement_id": replacement.ID,
			}).Error; err != nil {
			return fmt.Errorf("rotate refresh token: %w", err)
		}
		if err := tx.Model(&models.AuthSession{}).
			Where("id = ?", session.ID).
			Updates(map[string]any{
				"last_seen_at": now,
				"ip_address":   metadata.IPAddress,
				"user_agent":   boundedUserAgent(metadata.UserAgent),
			}).Error; err != nil {
			return fmt.Errorf("update refreshed session: %w", err)
		}
		accessToken, accessExpiry, err := service.tokens.Issue(
			now,
			user.ID,
			session.ID,
			session.Scope,
			session.CPOID,
			role,
			session.TokenVersion,
		)
		if err != nil {
			return err
		}
		response = TokenResponse{
			AccessToken:          accessToken,
			AccessTokenExpiresAt: accessExpiry,
			RefreshToken:         replacementToken,
			SessionExpiresAt:     session.ExpiresAt,
			TokenType:            "Bearer",
			CPOAppID:             appID,
			CPOAppIDMode:         appIDMode,
			MustChangePassword:   user.MustChangePassword,
		}
		return nil
	})
	if err != nil {
		return TokenResponse{}, err
	}
	if outcome != nil {
		return TokenResponse{}, outcome
	}
	return response, nil
}

func (service *Service) ValidateAccess(
	ctx context.Context,
	rawToken string,
) (Principal, error) {
	claims, err := service.tokens.Parse(rawToken, service.now())
	if err != nil {
		return Principal{}, errUnauthorized
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, errUnauthorized
	}

	var principal Principal
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session models.AuthSession
		if err := tx.First(&session, "id = ? AND user_id = ?", claims.SessionID, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errUnauthorized
			}
			return fmt.Errorf("load access session: %w", err)
		}
		now := service.now()
		if session.RevokedAt != nil ||
			!now.Before(session.ExpiresAt) ||
			session.TokenVersion != claims.TokenVersion ||
			session.Scope != claims.Scope ||
			!equalUUIDPointers(session.CPOID, claims.CPOID) ||
			!equalRolePointers(session.Role, claims.Role) {
			return errUnauthorized
		}
		user, role, appID, appIDMode, err := service.validateSessionContextTx(tx, session, now)
		if err != nil || !equalRolePointers(role, claims.Role) {
			return errUnauthorized
		}
		if now.Sub(session.LastSeenAt) >= 5*time.Minute {
			if err := tx.Model(&models.AuthSession{}).
				Where("id = ?", session.ID).
				Update("last_seen_at", now).Error; err != nil {
				return fmt.Errorf("touch access session: %w", err)
			}
		}
		principal = Principal{
			UserID:       user.ID,
			SessionID:    session.ID,
			Scope:        session.Scope,
			CPOID:        session.CPOID,
			Role:         role,
			CPOAppID:     appID,
			CPOAppIDMode: appIDMode,
			TokenVersion: session.TokenVersion,
			User: UserView{
				ID:                 user.ID,
				Email:              user.Email,
				FullName:           user.FullName,
				IsVerified:         user.IsVerified,
				MFAEnabled:         user.MFAEnabled,
				MustChangePassword: user.MustChangePassword,
				LastLoginAt:        user.LastLoginAt,
			},
		}
		return nil
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			return Principal{}, apiErr
		}
		return Principal{}, err
	}
	return principal, nil
}

func (service *Service) validateSessionContextTx(
	tx *gorm.DB,
	session models.AuthSession,
	now time.Time,
) (
	models.User,
	*constants.CPORole,
	*string,
	*constants.CPOAppIDMode,
	error,
) {
	var user models.User
	if err := tx.First(&user, "id = ? AND is_active = true", session.UserID).Error; err != nil {
		return models.User{}, nil, nil, nil, err
	}
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return models.User{}, nil, nil, nil, errUnauthorized
	}
	role, err := service.resolveLoginScopeTx(
		tx,
		session.UserID,
		session.Scope,
		session.CPOID,
	)
	if err != nil {
		return models.User{}, nil, nil, nil, err
	}
	appID, appIDMode, err := service.loadCPOAppIdentityTx(
		tx,
		session.Scope,
		session.CPOID,
	)
	if err != nil {
		return models.User{}, nil, nil, nil, err
	}
	return user, role, appID, appIDMode, nil
}

func (service *Service) Me(principal Principal) MeResponse {
	return MeResponse{
		User:         principal.User,
		Scope:        principal.Scope,
		CPOID:        principal.CPOID,
		Role:         principal.Role,
		CPOAppID:     principal.CPOAppID,
		CPOAppIDMode: principal.CPOAppIDMode,
	}
}

func (service *Service) loadCPOAppIdentityTx(
	tx *gorm.DB,
	scope constants.AuthScope,
	cpoID *uuid.UUID,
) (*string, *constants.CPOAppIDMode, error) {
	if scope == constants.AuthScopePlatform {
		return nil, nil, nil
	}
	if scope != constants.AuthScopeCPO || cpoID == nil {
		return nil, nil, errUnauthorized
	}
	var record struct {
		AppID     string
		AppIDMode constants.CPOAppIDMode
	}
	if err := tx.Model(&models.CPO{}).
		Select("app_id", "app_id_mode").
		Where("id = ?", *cpoID).
		Take(&record).Error; err != nil {
		return nil, nil, fmt.Errorf("load CPO app identity: %w", err)
	}
	return &record.AppID, &record.AppIDMode, nil
}

func (service *Service) ListSessions(
	ctx context.Context,
	principal Principal,
) ([]SessionView, error) {
	var sessions []models.AuthSession
	if err := service.database.WithContext(ctx).
		Where(
			"user_id = ? AND revoked_at IS NULL AND expires_at > ?",
			principal.UserID,
			service.now(),
		).
		Order("created_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list authentication sessions: %w", err)
	}
	result := make([]SessionView, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, SessionView{
			ID:         session.ID,
			Scope:      session.Scope,
			CPOID:      session.CPOID,
			Role:       session.Role,
			IPAddress:  session.IPAddress,
			UserAgent:  session.UserAgent,
			CreatedAt:  session.CreatedAt,
			LastSeenAt: session.LastSeenAt,
			ExpiresAt:  session.ExpiresAt,
			IsCurrent:  session.ID == principal.SessionID,
		})
	}
	return result, nil
}

func (service *Service) Logout(
	ctx context.Context,
	principal Principal,
) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := service.revokeSessionTx(tx, principal.SessionID, "LOGOUT", now); err != nil {
			return err
		}
		return service.audit(
			tx,
			&principal.UserID,
			principal.CPOID,
			"AUTH_LOGOUT",
			"AUTH_SESSION",
			&principal.SessionID,
			models.JSONB{},
			now,
		)
	})
}

func (service *Service) LogoutAll(
	ctx context.Context,
	principal Principal,
) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := service.revokeUserSessionsTx(
			tx,
			principal.UserID,
			"LOGOUT_ALL",
			now,
		); err != nil {
			return err
		}
		return service.audit(
			tx,
			&principal.UserID,
			principal.CPOID,
			"AUTH_LOGOUT_ALL",
			"USER",
			&principal.UserID,
			models.JSONB{},
			now,
		)
	})
}

func (service *Service) RevokeSession(
	ctx context.Context,
	principal Principal,
	sessionID uuid.UUID,
) error {
	if sessionID == uuid.Nil {
		return &APIError{
			Status: 400, Code: "invalid_session_id", Message: "The session ID is invalid.",
		}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.AuthSession{}).
			Where("id = ? AND user_id = ?", sessionID, principal.UserID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("find session for revocation: %w", err)
		}
		if count != 1 {
			return &APIError{
				Status: 404, Code: "session_not_found", Message: "The session was not found.",
			}
		}
		now := service.now()
		if err := service.revokeSessionTx(tx, sessionID, "USER_REVOKED", now); err != nil {
			return err
		}
		return service.audit(
			tx,
			&principal.UserID,
			principal.CPOID,
			"AUTH_SESSION_REVOKED",
			"AUTH_SESSION",
			&sessionID,
			models.JSONB{},
			now,
		)
	})
}

func (service *Service) revokeSessionTx(
	tx *gorm.DB,
	sessionID uuid.UUID,
	reason string,
	now time.Time,
) error {
	if err := tx.Model(&models.AuthSession{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Updates(map[string]any{
			"revoked_at":    now,
			"revoke_reason": reason,
		}).Error; err != nil {
		return fmt.Errorf("revoke authentication session: %w", err)
	}
	if err := tx.Model(&models.AuthRefreshToken{}).
		Where("session_id = ? AND used_at IS NULL AND revoked_at IS NULL", sessionID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke session refresh tokens: %w", err)
	}
	return nil
}

func (service *Service) revokeUserSessionsTx(
	tx *gorm.DB,
	userID uuid.UUID,
	reason string,
	now time.Time,
) error {
	if err := tx.Model(&models.AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{
			"revoked_at":    now,
			"revoke_reason": reason,
		}).Error; err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if err := tx.Exec(`
		UPDATE auth_refresh_tokens
		SET revoked_at = ?
		WHERE used_at IS NULL
		  AND revoked_at IS NULL
		  AND session_id IN (
		      SELECT id FROM auth_sessions WHERE user_id = ?
		  )
	`, now, userID).Error; err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}
	return nil
}

// RevokeUserSessionsTx is the identity-owned primitive used by platform
// governance and security operations. The caller owns the surrounding
// transaction and audit/event records.
func RevokeUserSessionsTx(
	tx *gorm.DB,
	userID uuid.UUID,
	scope *constants.AuthScope,
	cpoID *uuid.UUID,
	reason string,
	now time.Time,
) (int64, int64, error) {
	if tx == nil || userID == uuid.Nil {
		return 0, 0, errors.New("session revocation transaction and user are required")
	}
	if scope != nil && *scope == constants.AuthScopeCPO && cpoID == nil {
		return 0, 0, errors.New("CPO session revocation requires a CPO")
	}
	query := tx.Model(&models.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if scope != nil {
		query = query.Where("scope = ?", *scope)
	}
	if cpoID != nil {
		query = query.Where("cpo_id = ?", *cpoID)
	}
	sessionIDs := tx.Model(&models.AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID)
	if scope != nil {
		sessionIDs = sessionIDs.Where("scope = ?", *scope)
	}
	if cpoID != nil {
		sessionIDs = sessionIDs.Where("cpo_id = ?", *cpoID)
	}
	sessionIDs = sessionIDs.Select("id")
	refreshResult := tx.Model(&models.AuthRefreshToken{}).
		Where("used_at IS NULL AND revoked_at IS NULL AND session_id IN (?)", sessionIDs).
		Update("revoked_at", now)
	if refreshResult.Error != nil {
		return 0, 0, fmt.Errorf("revoke target refresh tokens: %w", refreshResult.Error)
	}
	sessionResult := tx.Model(&models.AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID)
	if scope != nil {
		sessionResult = sessionResult.Where("scope = ?", *scope)
	}
	if cpoID != nil {
		sessionResult = sessionResult.Where("cpo_id = ?", *cpoID)
	}
	sessionResult = sessionResult.Updates(map[string]any{
		"revoked_at":    now,
		"revoke_reason": reason,
	})
	if sessionResult.Error != nil {
		return 0, 0, fmt.Errorf("revoke target sessions: %w", sessionResult.Error)
	}
	return sessionResult.RowsAffected, refreshResult.RowsAffected, nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func equalUUIDPointers(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalRolePointers(left, right *constants.CPORole) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (service *Service) audit(
	tx *gorm.DB,
	userID *uuid.UUID,
	cpoID *uuid.UUID,
	action string,
	entity string,
	entityID *uuid.UUID,
	details models.JSONB,
	now time.Time,
) error {
	record := models.AuditLog{
		ID:        uuid.New(),
		CPOID:     cpoID,
		UserID:    userID,
		Action:    action,
		Entity:    entity,
		EntityID:  entityID,
		Details:   details,
		CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}
	return nil
}
