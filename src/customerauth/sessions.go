package customerauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CPOAppIDHeader      = "X-CPO-App-ID"
	principalContextKey = "ev_cms_customer_principal"
)

type customerContext struct {
	User     models.User
	Customer models.Customer
	CPO      models.CPO
	Wallet   models.Wallet
}

func (service *Service) Authenticate() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		header := strings.TrimSpace(ctx.GetHeader("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(ctx, errUnauthorized)
			ctx.Abort()
			return
		}
		principal, err := service.ValidateAccess(ctx.Request.Context(), parts[1])
		if err != nil {
			writeError(ctx, err)
			ctx.Abort()
			return
		}
		ctx.Set(principalContextKey, principal)
		ctx.Next()
	}
}

func RequireAppID() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		principal, ok := CurrentPrincipal(ctx)
		if !ok {
			writeError(ctx, errForbidden)
			ctx.Abort()
			return
		}
		supplied := strings.TrimSpace(ctx.GetHeader(CPOAppIDHeader))
		if supplied == "" {
			writeError(ctx, missingAppIDError())
			ctx.Abort()
			return
		}
		if len(supplied) != len(principal.CPOAppID) ||
			subtle.ConstantTimeCompare([]byte(supplied), []byte(principal.CPOAppID)) != 1 {
			writeError(ctx, &APIError{
				http.StatusForbidden, "cpo_app_id_mismatch",
				"The CPO app ID does not match the authenticated customer.",
			})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func CurrentPrincipal(ctx *gin.Context) (Principal, bool) {
	value, exists := ctx.Get(principalContextKey)
	if !exists {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}

func CurrentUserID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return principal.UserID, true
}

func CurrentCustomerID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return principal.CustomerID, true
}

func CurrentCPOID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return principal.CPOID, true
}

func CurrentCPOAppID(ctx *gin.Context) (string, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return "", false
	}
	return principal.CPOAppID, true
}

func (service *Service) ValidateAccess(ctx context.Context, rawToken string) (Principal, error) {
	claims, err := service.tokens.Parse(rawToken, service.now())
	if err != nil || claims.Scope != constants.AuthScopeCustomer ||
		claims.CPOID == nil || claims.Role != nil {
		return Principal{}, errUnauthorized
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, errUnauthorized
	}
	var principal Principal
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session models.AuthSession
		if err := tx.First(
			&session, "id = ? AND user_id = ?", claims.SessionID, userID,
		).Error; err != nil {
			return errUnauthorized
		}
		now := service.now()
		if session.RevokedAt != nil || !now.Before(session.ExpiresAt) ||
			session.Scope != constants.AuthScopeCustomer ||
			session.CPOID == nil || session.CustomerID == nil ||
			*session.CPOID != *claims.CPOID ||
			session.TokenVersion != claims.TokenVersion {
			return errUnauthorized
		}
		context, err := service.loadCustomerContext(tx, userID, session.CPOID)
		if err != nil || context.Customer.ID != *session.CustomerID {
			return errUnauthorized
		}
		if now.Sub(session.LastSeenAt) >= 5*time.Minute {
			if err := tx.Model(&models.AuthSession{}).Where("id = ?", session.ID).
				Update("last_seen_at", now).Error; err != nil {
				return fmt.Errorf("touch customer session: %w", err)
			}
		}
		principal = principalFromContext(context, session)
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

func (service *Service) Me(principal Principal) MeResponse {
	return MeResponse{
		User: principal.User, Customer: principal.Customer,
		CPO: principal.CPO, Wallet: principal.Wallet,
	}
}

func (service *Service) ListSessions(
	ctx context.Context,
	principal Principal,
) ([]SessionView, error) {
	var sessions []models.AuthSession
	if err := service.database.WithContext(ctx).Where(
		"scope = ? AND cpo_id = ? AND customer_id = ? AND revoked_at IS NULL AND expires_at > ?",
		constants.AuthScopeCustomer, principal.CPOID, principal.CustomerID, service.now(),
	).Order("created_at DESC").Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list customer sessions: %w", err)
	}
	result := make([]SessionView, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, SessionView{
			ID: session.ID, IPAddress: session.IPAddress, UserAgent: session.UserAgent,
			CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt,
			ExpiresAt: session.ExpiresAt, IsCurrent: session.ID == principal.SessionID,
		})
	}
	return result, nil
}

func (service *Service) Logout(ctx context.Context, principal Principal) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := revokeSessionTx(tx, principal.SessionID, "CUSTOMER_LOGOUT", now); err != nil {
			return err
		}
		return createAudit(
			tx, principal.UserID, principal.CPOID, "CUSTOMER_LOGOUT",
			"AUTH_SESSION", principal.SessionID, models.JSONB{}, now,
		)
	})
}

func (service *Service) LogoutAll(ctx context.Context, principal Principal) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := revokeCustomerSessionsTx(
			tx, principal.CPOID, principal.CustomerID, "CUSTOMER_LOGOUT_ALL", now,
		); err != nil {
			return err
		}
		return createAudit(
			tx, principal.UserID, principal.CPOID, "CUSTOMER_LOGOUT_ALL",
			"CUSTOMER", principal.CustomerID, models.JSONB{}, now,
		)
	})
}

func (service *Service) RevokeSession(
	ctx context.Context,
	principal Principal,
	sessionID uuid.UUID,
) error {
	if sessionID == uuid.Nil {
		return &APIError{http.StatusBadRequest, "invalid_session_id", "The session ID is invalid."}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.AuthSession{}).Where(
			"id = ? AND scope = ? AND cpo_id = ? AND customer_id = ?",
			sessionID, constants.AuthScopeCustomer, principal.CPOID, principal.CustomerID,
		).Count(&count).Error; err != nil {
			return fmt.Errorf("find customer session: %w", err)
		}
		if count != 1 {
			return &APIError{http.StatusNotFound, "session_not_found", "The session was not found."}
		}
		now := service.now()
		if err := revokeSessionTx(tx, sessionID, "CUSTOMER_REVOKED", now); err != nil {
			return err
		}
		return createAudit(
			tx, principal.UserID, principal.CPOID, "CUSTOMER_SESSION_REVOKED",
			"AUTH_SESSION", sessionID, models.JSONB{}, now,
		)
	})
}

func (service *Service) loadCustomerContext(
	tx *gorm.DB,
	userID uuid.UUID,
	cpoID *uuid.UUID,
) (customerContext, error) {
	return service.loadCustomerContextWithLockPolicy(tx, userID, cpoID, false)
}

func (service *Service) loadCustomerContextForRecovery(
	tx *gorm.DB,
	userID uuid.UUID,
	cpoID *uuid.UUID,
) (customerContext, error) {
	return service.loadCustomerContextWithLockPolicy(tx, userID, cpoID, true)
}

func (service *Service) loadCustomerContextWithLockPolicy(
	tx *gorm.DB,
	userID uuid.UUID,
	cpoID *uuid.UUID,
	allowLocked bool,
) (customerContext, error) {
	if cpoID == nil || *cpoID == uuid.Nil {
		return customerContext{}, errUnauthorized
	}
	var result customerContext
	if err := tx.First(
		&result.User, "id = ? AND is_active = true", userID,
	).Error; err != nil {
		return result, errUnauthorized
	}
	if !allowLocked && result.User.LockedUntil != nil &&
		result.User.LockedUntil.After(service.now()) {
		return result, errUnauthorized
	}
	if err := tx.Where(
		"id = ? AND status = ?", *cpoID, constants.CPOStatusActive,
	).First(&result.CPO).Error; err != nil {
		return result, errUnauthorized
	}
	if err := tx.Where(
		"cpo_id = ? AND user_id = ? AND status = ?",
		*cpoID, userID, constants.CustomerStatusActive,
	).First(&result.Customer).Error; err != nil {
		return result, errUnauthorized
	}
	if err := tx.Where(
		"cpo_id = ? AND customer_id = ?", *cpoID, result.Customer.ID,
	).First(&result.Wallet).Error; err != nil {
		return result, fmt.Errorf("load customer wallet: %w", err)
	}
	return result, nil
}

func principalFromContext(context customerContext, session models.AuthSession) Principal {
	return Principal{
		UserID: context.User.ID, CustomerID: context.Customer.ID,
		CPOID: context.CPO.ID, SessionID: session.ID,
		CPOAppID: context.CPO.AppID, TokenVersion: session.TokenVersion,
		User: UserView{
			ID: context.User.ID, Email: context.User.Email,
			FullName: context.User.FullName, Phone: context.User.Phone,
			IsVerified: context.User.IsVerified, LastLoginAt: context.User.LastLoginAt,
		},
		Customer: CustomerView{
			ID: context.Customer.ID, Status: string(context.Customer.Status),
			UserGroupID: context.Customer.UserGroupID,
		},
		CPO: CPOView{
			ID: context.CPO.ID, BusinessName: context.CPO.BusinessName,
			AppID: context.CPO.AppID, AppIDMode: string(context.CPO.AppIDMode),
		},
		Wallet: WalletView{
			ID: context.Wallet.ID, Balance: context.Wallet.Balance.StringFixed(2),
			Currency: context.Wallet.Currency,
		},
	}
}

func revokeSessionTx(tx *gorm.DB, sessionID uuid.UUID, reason string, now time.Time) error {
	if err := tx.Model(&models.AuthSession{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Updates(map[string]any{"revoked_at": now, "revoke_reason": reason}).Error; err != nil {
		return fmt.Errorf("revoke customer session: %w", err)
	}
	if err := tx.Model(&models.AuthRefreshToken{}).
		Where("session_id = ? AND used_at IS NULL AND revoked_at IS NULL", sessionID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke customer refresh tokens: %w", err)
	}
	return nil
}

func revokeCustomerSessionsTx(
	tx *gorm.DB,
	cpoID uuid.UUID,
	customerID uuid.UUID,
	reason string,
	now time.Time,
) error {
	if err := tx.Model(&models.AuthSession{}).Where(
		"scope = ? AND cpo_id = ? AND customer_id = ? AND revoked_at IS NULL",
		constants.AuthScopeCustomer, cpoID, customerID,
	).Updates(map[string]any{"revoked_at": now, "revoke_reason": reason}).Error; err != nil {
		return fmt.Errorf("revoke all customer sessions: %w", err)
	}
	if err := tx.Exec(`
		UPDATE auth_refresh_tokens
		SET revoked_at = ?
		WHERE used_at IS NULL
		  AND revoked_at IS NULL
		  AND session_id IN (
		      SELECT id FROM auth_sessions
		      WHERE scope = ? AND cpo_id = ? AND customer_id = ?
		  )
	`, now, constants.AuthScopeCustomer, cpoID, customerID).Error; err != nil {
		return fmt.Errorf("revoke all customer refresh tokens: %w", err)
	}
	return nil
}

func revokeUserSessionsTx(
	tx *gorm.DB,
	userID uuid.UUID,
	reason string,
	now time.Time,
) error {
	if err := tx.Model(&models.AuthSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{"revoked_at": now, "revoke_reason": reason}).Error; err != nil {
		return fmt.Errorf("revoke identity sessions: %w", err)
	}
	if err := tx.Exec(`
		UPDATE auth_refresh_tokens
		SET revoked_at = ?
		WHERE used_at IS NULL
		  AND revoked_at IS NULL
		  AND session_id IN (SELECT id FROM auth_sessions WHERE user_id = ?)
	`, now, userID).Error; err != nil {
		return fmt.Errorf("revoke identity refresh tokens: %w", err)
	}
	return nil
}

func createAudit(
	tx *gorm.DB,
	userID uuid.UUID,
	cpoID uuid.UUID,
	action string,
	entity string,
	entityID uuid.UUID,
	details models.JSONB,
	now time.Time,
) error {
	record := models.AuditLog{
		ID: uuid.New(), CPOID: &cpoID, UserID: &userID, Action: action,
		Entity: entity, EntityID: &entityID, Details: details, CreatedAt: now,
	}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("record customer authentication audit: %w", err)
	}
	return nil
}
