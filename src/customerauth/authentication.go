package customerauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	customerLoginMailTemplate = "CUSTOMER_LOGIN_OTP"
	customerResetMailTemplate = "CUSTOMER_PASSWORD_RESET_OTP"
)

var (
	errInvalidCredentials = &APIError{http.StatusUnauthorized, "invalid_credentials", "The supplied credentials are invalid."}
	errInvalidRefresh     = &APIError{http.StatusUnauthorized, "invalid_refresh_token", "The refresh token is invalid."}
	errUnauthorized       = &APIError{http.StatusUnauthorized, "unauthorized", "Customer authentication is required."}
	errForbidden          = &APIError{http.StatusForbidden, "forbidden", "This customer session cannot access the operation."}
)

func (service *Service) Login(ctx context.Context, appID string, request LoginRequest, metadata RequestMetadata) (ChallengeResponse, error) {
	appID = strings.TrimSpace(appID)
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if appID == "" {
		return ChallengeResponse{}, missingAppIDError()
	}
	if !validEmail(email) || request.Password == "" {
		return ChallengeResponse{}, errInvalidCredentials
	}
	if !service.mailEnabled {
		return ChallengeResponse{}, errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_LOGIN", rateLimitAddress(metadata)); err != nil {
		return ChallengeResponse{}, err
	}

	cpo, err := activeCPO(service.database.WithContext(ctx), appID)
	if err != nil {
		return ChallengeResponse{}, err
	}
	var customer models.Customer
	find := service.database.WithContext(ctx).Where(
		"cpo_id = ? AND lower(btrim(email)) = ?", cpo.ID, email,
	).First(&customer)
	hash := service.dummyHash
	if find.Error == nil {
		hash = customer.PasswordHash
	} else if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
		return ChallengeResponse{}, fmt.Errorf("find CPO customer account: %w", find.Error)
	}
	matches, verifyErr := security.VerifyPassword(request.Password, hash)
	if verifyErr != nil {
		return ChallengeResponse{}, fmt.Errorf("verify customer password: %w", verifyErr)
	}
	now := service.now()
	if find.Error != nil || !matches || customer.Status != constants.CustomerStatusActive ||
		(customer.LockedUntil != nil && customer.LockedUntil.After(now)) {
		if find.Error == nil && !matches {
			if err := service.recordFailedLogin(ctx, customer.ID, now); err != nil {
				return ChallengeResponse{}, err
			}
		}
		return ChallengeResponse{}, errInvalidCredentials
	}
	if err := service.database.WithContext(ctx).Model(&models.Customer{}).Where("id = ? AND cpo_id = ?", customer.ID, cpo.ID).
		Updates(map[string]any{"failed_login_attempts": 0, "locked_until": nil, "updated_at": now}).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("reset customer failed-login state: %w", err)
	}
	return service.createAuthChallenge(ctx, customer, constants.ChallengeCustomerLogin, metadata, customerLoginMailTemplate)
}

func (service *Service) VerifyLogin(ctx context.Context, appID string, request ChallengeRequest, metadata RequestMetadata) (TokenResponse, error) {
	if strings.TrimSpace(appID) == "" {
		return TokenResponse{}, missingAppIDError()
	}
	if request.ChallengeID == uuid.Nil || !validOTP(request.Code) {
		return TokenResponse{}, errInvalidChallenge
	}
	if err := service.checkRateLimit(ctx, "VERIFY_CUSTOMER_LOGIN", rateLimitAddress(metadata)); err != nil {
		return TokenResponse{}, err
	}
	var response TokenResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, customer, err := service.lockAuthChallenge(tx, request.ChallengeID, constants.ChallengeCustomerLogin)
		if err != nil {
			outcome = err
			return nil
		}
		context, err := service.loadCustomerContext(tx, customer.ID, customer.CPOID, false)
		if err != nil || context.CPO.AppID != strings.TrimSpace(appID) {
			outcome = errInvalidChallenge
			return nil
		}
		now := service.now()
		if !authChallengeUsable(challenge, now) {
			outcome = errInvalidChallenge
			return nil
		}
		if !service.verifyAuthOTP(challenge, request.Code) {
			if err := recordAuthChallengeFailure(tx, challenge, now); err != nil {
				return err
			}
			outcome = errInvalidChallenge
			return nil
		}
		if err := tx.Model(&models.CustomerAuthChallenge{}).Where("id = ? AND consumed_at IS NULL", challenge.ID).
			Update("consumed_at", now).Error; err != nil {
			return fmt.Errorf("consume customer login challenge: %w", err)
		}
		response, err = service.createCustomerSession(tx, context, metadata, now)
		return err
	})
	if err != nil {
		return TokenResponse{}, err
	}
	if outcome != nil {
		return TokenResponse{}, outcome
	}
	return response, nil
}

func (service *Service) ResendLogin(ctx context.Context, appID string, request ResendRequest, metadata RequestMetadata) (ChallengeResponse, error) {
	return service.resendAuthChallenge(ctx, appID, request, metadata, constants.ChallengeCustomerLogin, customerLoginMailTemplate)
}

func (service *Service) Refresh(ctx context.Context, appID string, request RefreshRequest, metadata RequestMetadata) (TokenResponse, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return TokenResponse{}, missingAppIDError()
	}
	if request.RefreshToken == "" || len(request.RefreshToken) > 256 {
		return TokenResponse{}, errInvalidRefresh
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_REFRESH", rateLimitAddress(metadata)); err != nil {
		return TokenResponse{}, err
	}

	var response TokenResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.CustomerAuthRefreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hashRefreshToken(request.RefreshToken)).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				outcome = errInvalidRefresh
				return nil
			}
			return fmt.Errorf("lock customer refresh token: %w", err)
		}
		var session models.CustomerAuthSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ?", current.SessionID).Error; err != nil {
			return fmt.Errorf("load customer refresh session: %w", err)
		}
		now := service.now()
		if current.UsedAt != nil {
			if err := revokeSessionTx(tx, session.ID, "REFRESH_TOKEN_REUSE", now); err != nil {
				return err
			}
			outcome = errInvalidRefresh
			return nil
		}
		if current.RevokedAt != nil || !now.Before(current.ExpiresAt) || session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
			outcome = errInvalidRefresh
			return nil
		}
		customerContext, err := service.loadCustomerContext(tx, session.CustomerID, session.CPOID, false)
		if err != nil {
			if err := revokeSessionTx(tx, session.ID, "CUSTOMER_AUTHORITY_CHANGED", now); err != nil {
				return err
			}
			outcome = errInvalidRefresh
			return nil
		}
		if customerContext.CPO.AppID != appID {
			outcome = errInvalidRefresh
			return nil
		}
		replacementToken, err := security.RandomToken(32)
		if err != nil {
			return err
		}
		replacement := models.CustomerAuthRefreshToken{ID: uuid.New(), SessionID: session.ID, TokenHash: hashRefreshToken(replacementToken), ExpiresAt: session.ExpiresAt, CreatedAt: now}
		if err := tx.Create(&replacement).Error; err != nil {
			return fmt.Errorf("create customer replacement refresh token: %w", err)
		}
		if err := tx.Model(&models.CustomerAuthRefreshToken{}).Where("id = ? AND used_at IS NULL", current.ID).
			Updates(map[string]any{"used_at": now, "replacement_id": replacement.ID}).Error; err != nil {
			return fmt.Errorf("rotate customer refresh token: %w", err)
		}
		if err := tx.Model(&models.CustomerAuthSession{}).Where("id = ?", session.ID).
			Updates(map[string]any{"last_seen_at": now, "ip_address": metadata.IPAddress, "user_agent": boundedUserAgent(metadata.UserAgent)}).Error; err != nil {
			return fmt.Errorf("update customer refreshed session: %w", err)
		}
		accessToken, accessExpiry, err := service.tokens.Issue(now, session.CustomerID, session.ID, constants.AuthScopeCustomer, &session.CPOID, nil, session.TokenVersion)
		if err != nil {
			return err
		}
		response = tokenResponse(accessToken, accessExpiry, replacementToken, session.ExpiresAt, customerContext)
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

func (service *Service) createCustomerSession(tx *gorm.DB, customerContext customerContext, metadata RequestMetadata, now time.Time) (TokenResponse, error) {
	session := models.CustomerAuthSession{
		ID: uuid.New(), CPOID: customerContext.CPO.ID, CustomerID: customerContext.Customer.ID,
		TokenVersion: 1, IPAddress: metadata.IPAddress, UserAgent: boundedUserAgent(metadata.UserAgent),
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(service.config.SessionTTL),
	}
	if err := tx.Create(&session).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("create customer session: %w", err)
	}
	refreshToken, err := security.RandomToken(32)
	if err != nil {
		return TokenResponse{}, err
	}
	refresh := models.CustomerAuthRefreshToken{ID: uuid.New(), SessionID: session.ID, TokenHash: hashRefreshToken(refreshToken), ExpiresAt: session.ExpiresAt, CreatedAt: now}
	if err := tx.Create(&refresh).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("store customer refresh token: %w", err)
	}
	accessToken, accessExpiry, err := service.tokens.Issue(now, customerContext.Customer.ID, session.ID, constants.AuthScopeCustomer, &session.CPOID, nil, session.TokenVersion)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := tx.Model(&models.Customer{}).Where("id = ? AND cpo_id = ?", customerContext.Customer.ID, customerContext.CPO.ID).
		Updates(map[string]any{"last_login_at": now, "is_verified": true, "updated_at": now}).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("record customer login: %w", err)
	}
	if err := createCustomerAudit(tx, customerContext.Customer.ID, customerContext.CPO.ID, "CUSTOMER_LOGIN_SUCCEEDED", "CUSTOMER_AUTH_SESSION", session.ID, models.JSONB{}, now); err != nil {
		return TokenResponse{}, err
	}
	customerContext.Customer.LastLoginAt = &now
	return tokenResponse(accessToken, accessExpiry, refreshToken, session.ExpiresAt, customerContext), nil
}

func tokenResponse(accessToken string, accessExpiry time.Time, refreshToken string, sessionExpiry time.Time, context customerContext) TokenResponse {
	return TokenResponse{AccessToken: accessToken, AccessTokenExpiresAt: accessExpiry, RefreshToken: refreshToken, SessionExpiresAt: sessionExpiry, TokenType: "Bearer", CustomerID: context.Customer.ID, CPOID: context.CPO.ID, CPOAppID: context.CPO.AppID}
}

func (service *Service) createAuthChallenge(ctx context.Context, customer models.Customer, purpose constants.AuthChallengePurpose, metadata RequestMetadata, template string) (ChallengeResponse, error) {
	var response ChallengeResponse
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		response, err = service.createAuthChallengeTx(tx, customer, purpose, metadata, template, service.now())
		return err
	})
	return response, err
}

func (service *Service) createAuthChallengeTx(tx *gorm.DB, customer models.Customer, purpose constants.AuthChallengePurpose, metadata RequestMetadata, template string, now time.Time) (ChallengeResponse, error) {
	code, err := security.RandomDigits(6)
	if err != nil {
		return ChallengeResponse{}, err
	}
	challenge := models.CustomerAuthChallenge{
		ID: uuid.New(), CPOID: customer.CPOID, CustomerID: customer.ID, Purpose: purpose,
		ExpiresAt: now.Add(service.config.OTPExpiry), MaxAttempts: 5,
		ResendAvailableAt: now.Add(service.config.OTPResendCooldown), RequestIP: metadata.IPAddress,
		UserAgent: boundedUserAgent(metadata.UserAgent), CreatedAt: now,
	}
	challenge.CodeHash = service.authOTPHash(challenge.ID, purpose, code)
	if err := tx.Model(&models.CustomerAuthChallenge{}).Where(
		"cpo_id = ? AND customer_id = ? AND purpose = ? AND consumed_at IS NULL AND invalidated_at IS NULL",
		customer.CPOID, customer.ID, purpose,
	).Update("invalidated_at", now).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("invalidate prior customer challenges: %w", err)
	}
	if err := tx.Create(&challenge).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("create customer authentication challenge: %w", err)
	}
	payload := authChallengeOTPPayload(customer, challenge, code)
	if err := service.outbox.EnqueueMessageWithContext(
		tx,
		customer.Email,
		template,
		payload,
		cmsmail.MessageContext{CPOID: &customer.CPOID},
	); err != nil {
		return ChallengeResponse{}, err
	}
	return ChallengeResponse{ChallengeID: challenge.ID, ExpiresAt: challenge.ExpiresAt, ResendAvailableAt: challenge.ResendAvailableAt}, nil
}

func authChallengeOTPPayload(customer models.Customer, challenge models.CustomerAuthChallenge, code string) cmsmail.MessagePayload {
	payload := cmsmail.MessagePayload{RecipientName: customer.FullName, Code: code, ExpiresAt: challenge.ExpiresAt}
	if challenge.Purpose == constants.ChallengeCustomerReset {
		payload.ChallengeID = challenge.ID.String()
	}
	return payload
}

func (service *Service) resendAuthChallenge(ctx context.Context, appID string, request ResendRequest, metadata RequestMetadata, purpose constants.AuthChallengePurpose, template string) (ChallengeResponse, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return ChallengeResponse{}, missingAppIDError()
	}
	if request.ChallengeID == uuid.Nil {
		return ChallengeResponse{}, errInvalidChallenge
	}
	if !service.mailEnabled {
		return ChallengeResponse{}, errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "RESEND_"+string(purpose), rateLimitAddress(metadata)); err != nil {
		return ChallengeResponse{}, err
	}
	var response ChallengeResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, customer, err := service.lockAuthChallenge(tx, request.ChallengeID, purpose)
		if err != nil {
			outcome = err
			return nil
		}
		context, err := service.loadCustomerContext(tx, customer.ID, customer.CPOID, purpose == constants.ChallengeCustomerReset)
		now := service.now()
		if err != nil || context.CPO.AppID != appID || challenge.ConsumedAt != nil || challenge.InvalidatedAt != nil || !now.Before(challenge.ExpiresAt) || now.Before(challenge.ResendAvailableAt) {
			outcome = errInvalidChallenge
			return nil
		}
		if err := tx.Model(&models.CustomerAuthChallenge{}).Where("id = ?", challenge.ID).Update("invalidated_at", now).Error; err != nil {
			return fmt.Errorf("invalidate customer challenge: %w", err)
		}
		response, err = service.createAuthChallengeTx(tx, customer, purpose, metadata, template, now)
		return err
	})
	if err != nil {
		return ChallengeResponse{}, err
	}
	if outcome != nil {
		return ChallengeResponse{}, outcome
	}
	return response, nil
}

func (service *Service) lockAuthChallenge(tx *gorm.DB, id uuid.UUID, purpose constants.AuthChallengePurpose) (models.CustomerAuthChallenge, models.Customer, error) {
	var challenge models.CustomerAuthChallenge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND purpose = ?", id, purpose).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return challenge, models.Customer{}, errInvalidChallenge
		}
		return challenge, models.Customer{}, fmt.Errorf("lock customer authentication challenge: %w", err)
	}
	var customer models.Customer
	if err := tx.Where("id = ? AND cpo_id = ?", challenge.CustomerID, challenge.CPOID).First(&customer).Error; err != nil {
		return challenge, customer, fmt.Errorf("load challenged customer account: %w", err)
	}
	return challenge, customer, nil
}

func (service *Service) authOTPHash(id uuid.UUID, purpose constants.AuthChallengePurpose, code string) []byte {
	mac := hmac.New(sha256.New, service.config.OTPHMACKey)
	_, _ = mac.Write([]byte(id.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (service *Service) verifyAuthOTP(challenge models.CustomerAuthChallenge, code string) bool {
	return hmac.Equal(service.authOTPHash(challenge.ID, challenge.Purpose, code), challenge.CodeHash)
}

func authChallengeUsable(challenge models.CustomerAuthChallenge, now time.Time) bool {
	return challenge.ConsumedAt == nil && challenge.InvalidatedAt == nil && challenge.Attempts < challenge.MaxAttempts && now.Before(challenge.ExpiresAt)
}

func recordAuthChallengeFailure(tx *gorm.DB, challenge models.CustomerAuthChallenge, now time.Time) error {
	updates := map[string]any{"attempts": challenge.Attempts + 1}
	if challenge.Attempts+1 >= challenge.MaxAttempts {
		updates["invalidated_at"] = now
	}
	if err := tx.Model(&models.CustomerAuthChallenge{}).Where("id = ?", challenge.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("record customer challenge failure: %w", err)
	}
	return nil
}

func (service *Service) recordFailedLogin(ctx context.Context, customerID uuid.UUID, now time.Time) error {
	result := service.database.WithContext(ctx).Exec(`
		UPDATE customers
		SET failed_login_attempts = failed_login_attempts + 1,
		    locked_until = CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END,
		    updated_at = ?
		WHERE id = ?
	`, service.config.LoginMaxAttempts, now.Add(service.config.LoginLockDuration), now, customerID)
	if result.Error != nil {
		return fmt.Errorf("record failed customer login: %w", result.Error)
	}
	return nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func missingAppIDError() *APIError {
	return &APIError{http.StatusBadRequest, "missing_cpo_app_id", "X-CPO-App-ID is required."}
}
