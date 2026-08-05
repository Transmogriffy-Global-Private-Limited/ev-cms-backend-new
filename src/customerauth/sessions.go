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
	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
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
		cmsmiddleware.SetRequestActor(ctx, cmsmiddleware.RequestActor{
			AuthScope: string(constants.AuthScopeCustomer), CPOID: principal.CPOID.String(), CustomerID: principal.CustomerID.String(),
		})
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
		if len(supplied) != len(principal.CPOAppID) || subtle.ConstantTimeCompare([]byte(supplied), []byte(principal.CPOAppID)) != 1 {
			writeError(ctx, &APIError{http.StatusForbidden, "cpo_app_id_mismatch", "The CPO app ID does not match the authenticated customer."})
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

func CurrentCustomerID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return principal.CustomerID, true
}

// CurrentUserID is retained for source compatibility with existing app
// modules. Customer IDs are now the app-account IDs; no global users row is
// created or returned for a customer.
func CurrentUserID(ctx *gin.Context) (uuid.UUID, bool) { return CurrentCustomerID(ctx) }

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
	if err != nil || claims.Scope != constants.AuthScopeCustomer || claims.CPOID == nil || claims.Role != nil {
		return Principal{}, errUnauthorized
	}
	customerID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, errUnauthorized
	}
	var principal Principal
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session models.CustomerAuthSession
		if err := tx.First(&session, "id = ? AND customer_id = ?", claims.SessionID, customerID).Error; err != nil {
			return errUnauthorized
		}
		now := service.now()
		if session.RevokedAt != nil || !now.Before(session.ExpiresAt) || session.CPOID != *claims.CPOID || session.TokenVersion != claims.TokenVersion {
			return errUnauthorized
		}
		context, err := service.loadCustomerContext(tx, customerID, session.CPOID, false)
		if err != nil {
			return errUnauthorized
		}
		if now.Sub(session.LastSeenAt) >= 5*time.Minute {
			if err := tx.Model(&models.CustomerAuthSession{}).Where("id = ?", session.ID).Update("last_seen_at", now).Error; err != nil {
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
	return MeResponse{User: principal.User, Customer: principal.Customer, CPO: principal.CPO, Wallet: principal.Wallet}
}

func (service *Service) ListSessions(ctx context.Context, principal Principal) ([]SessionView, error) {
	var sessions []models.CustomerAuthSession
	if err := service.database.WithContext(ctx).Where(
		"cpo_id = ? AND customer_id = ? AND revoked_at IS NULL AND expires_at > ?", principal.CPOID, principal.CustomerID, service.now(),
	).Order("created_at DESC").Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list customer sessions: %w", err)
	}
	result := make([]SessionView, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, SessionView{ID: session.ID, IPAddress: session.IPAddress, UserAgent: session.UserAgent, CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt, ExpiresAt: session.ExpiresAt, IsCurrent: session.ID == principal.SessionID})
	}
	return result, nil
}

func (service *Service) Logout(ctx context.Context, principal Principal) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := revokeSessionTx(tx, principal.SessionID, "CUSTOMER_LOGOUT", now); err != nil {
			return err
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_LOGOUT", "CUSTOMER_AUTH_SESSION", principal.SessionID, models.JSONB{}, now)
	})
}

func (service *Service) LogoutAll(ctx context.Context, principal Principal) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if err := revokeCustomerSessionsTx(tx, principal.CPOID, principal.CustomerID, "CUSTOMER_LOGOUT_ALL", now); err != nil {
			return err
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_LOGOUT_ALL", "CUSTOMER", principal.CustomerID, models.JSONB{}, now)
	})
}

func (service *Service) RevokeSession(ctx context.Context, principal Principal, sessionID uuid.UUID) error {
	if sessionID == uuid.Nil {
		return &APIError{http.StatusBadRequest, "invalid_session_id", "The session ID is invalid."}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.CustomerAuthSession{}).Where("id = ? AND cpo_id = ? AND customer_id = ?", sessionID, principal.CPOID, principal.CustomerID).Count(&count).Error; err != nil {
			return fmt.Errorf("find customer session: %w", err)
		}
		if count != 1 {
			return &APIError{http.StatusNotFound, "session_not_found", "The session was not found."}
		}
		now := service.now()
		if err := revokeSessionTx(tx, sessionID, "CUSTOMER_REVOKED", now); err != nil {
			return err
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_SESSION_REVOKED", "CUSTOMER_AUTH_SESSION", sessionID, models.JSONB{}, now)
	})
}

func (service *Service) loadCustomerContext(tx *gorm.DB, customerID, cpoID uuid.UUID, allowLocked bool) (customerContext, error) {
	if customerID == uuid.Nil || cpoID == uuid.Nil {
		return customerContext{}, errUnauthorized
	}
	var result customerContext
	if err := tx.Where("id = ? AND status = ?", cpoID, constants.CPOStatusActive).First(&result.CPO).Error; err != nil {
		return result, errUnauthorized
	}
	if err := tx.Where("id = ? AND cpo_id = ? AND status = ?", customerID, cpoID, constants.CustomerStatusActive).First(&result.Customer).Error; err != nil {
		return result, errUnauthorized
	}
	if !allowLocked && result.Customer.LockedUntil != nil && result.Customer.LockedUntil.After(service.now()) {
		return result, errUnauthorized
	}
	if err := tx.Where("cpo_id = ? AND customer_id = ?", cpoID, customerID).First(&result.Wallet).Error; err != nil {
		return result, fmt.Errorf("load customer wallet: %w", err)
	}
	return result, nil
}

func principalFromContext(context customerContext, session models.CustomerAuthSession) Principal {
	return Principal{
		UserID: context.Customer.ID, CustomerID: context.Customer.ID, CPOID: context.CPO.ID, SessionID: session.ID, CPOAppID: context.CPO.AppID, TokenVersion: session.TokenVersion,
		User:     userView(context.Customer),
		Customer: CustomerView{ID: context.Customer.ID, Status: string(context.Customer.Status), UserGroupID: context.Customer.UserGroupID},
		CPO:      CPOView{ID: context.CPO.ID, BusinessName: context.CPO.BusinessName, AppID: context.CPO.AppID, AppIDMode: string(context.CPO.AppIDMode)},
		Wallet:   WalletView{ID: context.Wallet.ID, Balance: context.Wallet.Balance.StringFixed(2), Currency: context.Wallet.Currency},
	}
}

func revokeSessionTx(tx *gorm.DB, sessionID uuid.UUID, reason string, now time.Time) error {
	if err := tx.Model(&models.CustomerAuthSession{}).Where("id = ? AND revoked_at IS NULL", sessionID).Updates(map[string]any{"revoked_at": now, "revoke_reason": reason}).Error; err != nil {
		return fmt.Errorf("revoke customer session: %w", err)
	}
	if err := tx.Model(&models.CustomerAuthRefreshToken{}).Where("session_id = ? AND used_at IS NULL AND revoked_at IS NULL", sessionID).Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke customer refresh tokens: %w", err)
	}
	return nil
}

func revokeCustomerSessionsTx(tx *gorm.DB, cpoID, customerID uuid.UUID, reason string, now time.Time) error {
	sessionIDs := tx.Model(&models.CustomerAuthSession{}).Select("id").Where("cpo_id = ? AND customer_id = ?", cpoID, customerID)
	if err := tx.Model(&models.CustomerAuthRefreshToken{}).Where("used_at IS NULL AND revoked_at IS NULL").Where("session_id IN (?)", sessionIDs).Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke all customer refresh tokens: %w", err)
	}
	if err := tx.Model(&models.CustomerAuthSession{}).Where("cpo_id = ? AND customer_id = ? AND revoked_at IS NULL", cpoID, customerID).Updates(map[string]any{"revoked_at": now, "revoke_reason": reason}).Error; err != nil {
		return fmt.Errorf("revoke all customer sessions: %w", err)
	}
	return nil
}

func createCustomerAudit(tx *gorm.DB, customerID, cpoID uuid.UUID, action, entity string, entityID uuid.UUID, details models.JSONB, now time.Time) error {
	if details == nil {
		details = models.JSONB{}
	}
	details["actor_customer_id"] = customerID.String()
	record := models.AuditLog{ID: uuid.New(), CPOID: &cpoID, Action: action, Entity: entity, EntityID: &entityID, Details: details, CreatedAt: now}
	if err := tx.Create(&record).Error; err != nil {
		return fmt.Errorf("record customer authentication audit: %w", err)
	}
	return nil
}
