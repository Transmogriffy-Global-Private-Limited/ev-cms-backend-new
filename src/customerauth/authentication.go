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
	errInvalidCredentials = &APIError{
		http.StatusUnauthorized, "invalid_credentials",
		"The supplied credentials are invalid.",
	}
	errInvalidRefresh = &APIError{
		http.StatusUnauthorized, "invalid_refresh_token",
		"The refresh token is invalid.",
	}
	errUnauthorized = &APIError{
		http.StatusUnauthorized, "unauthorized",
		"Customer authentication is required.",
	}
	errForbidden = &APIError{
		http.StatusForbidden, "forbidden",
		"This customer session cannot access the operation.",
	}
)

func (service *Service) Login(
	ctx context.Context,
	appID string,
	request LoginRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
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

	var cpo models.CPO
	var user models.User
	var customer models.Customer
	query := service.database.WithContext(ctx)
	var err error
	if cpo, err = activeCPO(query, appID); err != nil {
		return ChallengeResponse{}, err
	}
	findUser := query.Where("lower(btrim(email)) = ?", email).First(&user)
	hash := service.dummyHash
	if findUser.Error == nil {
		hash = user.PasswordHash
	} else if !errors.Is(findUser.Error, gorm.ErrRecordNotFound) {
		return ChallengeResponse{}, fmt.Errorf("find customer login identity: %w", findUser.Error)
	}
	matches, verifyErr := security.VerifyPassword(request.Password, hash)
	if verifyErr != nil {
		return ChallengeResponse{}, fmt.Errorf("verify customer login password: %w", verifyErr)
	}
	now := service.now()
	if findUser.Error != nil || !matches || !user.IsActive ||
		(user.LockedUntil != nil && user.LockedUntil.After(now)) {
		if findUser.Error == nil && !matches {
			if err := service.recordFailedLogin(ctx, user.ID, now); err != nil {
				return ChallengeResponse{}, err
			}
		}
		return ChallengeResponse{}, errInvalidCredentials
	}
	if err := query.Where(
		"cpo_id = ? AND user_id = ? AND status = ?",
		cpo.ID, user.ID, constants.CustomerStatusActive,
	).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChallengeResponse{}, errInvalidCredentials
		}
		return ChallengeResponse{}, fmt.Errorf("resolve customer login relationship: %w", err)
	}
	if err := query.Model(&models.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{
			"failed_login_attempts": 0,
			"locked_until":          nil,
			"updated_at":            now,
		}).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("reset customer failed-login state: %w", err)
	}
	return service.createAuthChallenge(
		ctx, user, cpo.ID, constants.ChallengeCustomerLogin,
		metadata, customerLoginMailTemplate,
	)
}

func (service *Service) VerifyLogin(
	ctx context.Context,
	appID string,
	request ChallengeRequest,
	metadata RequestMetadata,
) (TokenResponse, error) {
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
		challenge, user, err := service.lockAuthChallenge(
			tx, request.ChallengeID, constants.ChallengeCustomerLogin,
		)
		if err != nil {
			outcome = err
			return nil
		}
		context, err := service.loadCustomerContext(tx, user.ID, challenge.CPOID)
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
		if err := tx.Model(&models.AuthChallenge{}).
			Where("id = ? AND consumed_at IS NULL", challenge.ID).
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

func (service *Service) ResendLogin(
	ctx context.Context,
	appID string,
	request ResendRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	return service.resendAuthChallenge(
		ctx, appID, request, metadata,
		constants.ChallengeCustomerLogin, customerLoginMailTemplate,
	)
}

func (service *Service) Refresh(
	ctx context.Context,
	appID string,
	request RefreshRequest,
	metadata RequestMetadata,
) (TokenResponse, error) {
	if strings.TrimSpace(appID) == "" {
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
		var current models.AuthRefreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ?", hashRefreshToken(request.RefreshToken)).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				outcome = errInvalidRefresh
				return nil
			}
			return fmt.Errorf("lock customer refresh token: %w", err)
		}
		var session models.AuthSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&session, "id = ?", current.SessionID).Error; err != nil {
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
		if current.RevokedAt != nil || !now.Before(current.ExpiresAt) ||
			session.RevokedAt != nil || !now.Before(session.ExpiresAt) ||
			session.Scope != constants.AuthScopeCustomer ||
			session.CPOID == nil || session.CustomerID == nil {
			outcome = errInvalidRefresh
			return nil
		}
		customerContext, err := service.loadCustomerContext(tx, session.UserID, session.CPOID)
		if err != nil || customerContext.Customer.ID != *session.CustomerID {
			if err := revokeSessionTx(tx, session.ID, "CUSTOMER_AUTHORITY_CHANGED", now); err != nil {
				return err
			}
			outcome = errInvalidRefresh
			return nil
		}
		if customerContext.CPO.AppID != strings.TrimSpace(appID) {
			outcome = errInvalidRefresh
			return nil
		}
		replacementToken, err := security.RandomToken(32)
		if err != nil {
			return err
		}
		replacement := models.AuthRefreshToken{
			ID: uuid.New(), SessionID: session.ID,
			TokenHash: hashRefreshToken(replacementToken),
			ExpiresAt: session.ExpiresAt, CreatedAt: now,
		}
		if err := tx.Create(&replacement).Error; err != nil {
			return fmt.Errorf("create customer replacement refresh token: %w", err)
		}
		if err := tx.Model(&models.AuthRefreshToken{}).
			Where("id = ? AND used_at IS NULL", current.ID).
			Updates(map[string]any{
				"used_at": now, "replacement_id": replacement.ID,
			}).Error; err != nil {
			return fmt.Errorf("rotate customer refresh token: %w", err)
		}
		if err := tx.Model(&models.AuthSession{}).Where("id = ?", session.ID).
			Updates(map[string]any{
				"last_seen_at": now, "ip_address": metadata.IPAddress,
				"user_agent": boundedUserAgent(metadata.UserAgent),
			}).Error; err != nil {
			return fmt.Errorf("update customer refreshed session: %w", err)
		}
		accessToken, accessExpiry, err := service.tokens.Issue(
			now, session.UserID, session.ID, constants.AuthScopeCustomer,
			session.CPOID, nil, session.TokenVersion,
		)
		if err != nil {
			return err
		}
		response = tokenResponse(
			accessToken, accessExpiry, replacementToken, session.ExpiresAt,
			customerContext,
		)
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

func (service *Service) createCustomerSession(
	tx *gorm.DB,
	customerContext customerContext,
	metadata RequestMetadata,
	now time.Time,
) (TokenResponse, error) {
	customerID := customerContext.Customer.ID
	cpoID := customerContext.CPO.ID
	session := models.AuthSession{
		ID: uuid.New(), UserID: customerContext.User.ID,
		Scope: constants.AuthScopeCustomer, CPOID: &cpoID, CustomerID: &customerID,
		TokenVersion: 1, IPAddress: metadata.IPAddress,
		UserAgent: boundedUserAgent(metadata.UserAgent), CreatedAt: now,
		LastSeenAt: now, ExpiresAt: now.Add(service.config.SessionTTL),
	}
	if err := tx.Create(&session).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("create customer session: %w", err)
	}
	refreshToken, err := security.RandomToken(32)
	if err != nil {
		return TokenResponse{}, err
	}
	refresh := models.AuthRefreshToken{
		ID: uuid.New(), SessionID: session.ID, TokenHash: hashRefreshToken(refreshToken),
		ExpiresAt: session.ExpiresAt, CreatedAt: now,
	}
	if err := tx.Create(&refresh).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("store customer refresh token: %w", err)
	}
	accessToken, accessExpiry, err := service.tokens.Issue(
		now, customerContext.User.ID, session.ID, constants.AuthScopeCustomer,
		&cpoID, nil, session.TokenVersion,
	)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := tx.Model(&models.User{}).Where("id = ?", customerContext.User.ID).
		Updates(map[string]any{
			"last_login_at": now, "mfa_enabled": true,
			"is_verified": true, "updated_at": now,
		}).Error; err != nil {
		return TokenResponse{}, fmt.Errorf("record customer login: %w", err)
	}
	if err := createAudit(
		tx, customerContext.User.ID, cpoID, "CUSTOMER_LOGIN_SUCCEEDED",
		"AUTH_SESSION", session.ID, models.JSONB{}, now,
	); err != nil {
		return TokenResponse{}, err
	}
	return tokenResponse(
		accessToken, accessExpiry, refreshToken, session.ExpiresAt, customerContext,
	), nil
}

func tokenResponse(
	accessToken string,
	accessExpiry time.Time,
	refreshToken string,
	sessionExpiry time.Time,
	context customerContext,
) TokenResponse {
	return TokenResponse{
		AccessToken: accessToken, AccessTokenExpiresAt: accessExpiry,
		RefreshToken: refreshToken, SessionExpiresAt: sessionExpiry,
		TokenType: "Bearer", CustomerID: context.Customer.ID,
		CPOID: context.CPO.ID, CPOAppID: context.CPO.AppID,
	}
}

func (service *Service) createAuthChallenge(
	ctx context.Context,
	user models.User,
	cpoID uuid.UUID,
	purpose constants.AuthChallengePurpose,
	metadata RequestMetadata,
	template string,
) (ChallengeResponse, error) {
	var response ChallengeResponse
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		response, err = service.createAuthChallengeTx(
			tx, user, cpoID, purpose, metadata, template, service.now(),
		)
		return err
	})
	return response, err
}

func (service *Service) createAuthChallengeTx(
	tx *gorm.DB,
	user models.User,
	cpoID uuid.UUID,
	purpose constants.AuthChallengePurpose,
	metadata RequestMetadata,
	template string,
	now time.Time,
) (ChallengeResponse, error) {
	code, err := security.RandomDigits(6)
	if err != nil {
		return ChallengeResponse{}, err
	}
	scope := constants.AuthScopeCustomer
	challenge := models.AuthChallenge{
		ID: uuid.New(), UserID: user.ID, Purpose: purpose,
		Scope: &scope, CPOID: &cpoID,
		ExpiresAt: now.Add(service.config.OTPExpiry), MaxAttempts: 5,
		ResendAvailableAt: now.Add(service.config.OTPResendCooldown),
		RequestIP:         metadata.IPAddress, UserAgent: boundedUserAgent(metadata.UserAgent),
		CreatedAt: now,
	}
	challenge.CodeHash = service.authOTPHash(challenge.ID, purpose, code)
	if err := tx.Model(&models.AuthChallenge{}).
		Where(
			"user_id = ? AND cpo_id = ? AND purpose = ? AND consumed_at IS NULL AND invalidated_at IS NULL",
			user.ID, cpoID, purpose,
		).Update("invalidated_at", now).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("invalidate prior customer challenges: %w", err)
	}
	if err := tx.Create(&challenge).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("create customer authentication challenge: %w", err)
	}
	if err := service.outbox.EnqueueOTP(tx, user.Email, template, cmsmail.OTPPayload{
		RecipientName: user.FullName, Code: code, ExpiresAt: challenge.ExpiresAt,
	}); err != nil {
		return ChallengeResponse{}, err
	}
	return ChallengeResponse{
		ChallengeID: challenge.ID, ExpiresAt: challenge.ExpiresAt,
		ResendAvailableAt: challenge.ResendAvailableAt,
	}, nil
}

func (service *Service) resendAuthChallenge(
	ctx context.Context,
	appID string,
	request ResendRequest,
	metadata RequestMetadata,
	purpose constants.AuthChallengePurpose,
	template string,
) (ChallengeResponse, error) {
	if strings.TrimSpace(appID) == "" {
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
		challenge, user, err := service.lockAuthChallenge(tx, request.ChallengeID, purpose)
		if err != nil {
			outcome = err
			return nil
		}
		var context customerContext
		if purpose == constants.ChallengeCustomerReset {
			context, err = service.loadCustomerContextForRecovery(tx, user.ID, challenge.CPOID)
		} else {
			context, err = service.loadCustomerContext(tx, user.ID, challenge.CPOID)
		}
		now := service.now()
		if err != nil || context.CPO.AppID != strings.TrimSpace(appID) ||
			challenge.ConsumedAt != nil || challenge.InvalidatedAt != nil ||
			!now.Before(challenge.ExpiresAt) || now.Before(challenge.ResendAvailableAt) {
			outcome = errInvalidChallenge
			return nil
		}
		if err := tx.Model(&models.AuthChallenge{}).Where("id = ?", challenge.ID).
			Update("invalidated_at", now).Error; err != nil {
			return fmt.Errorf("invalidate customer challenge: %w", err)
		}
		response, err = service.createAuthChallengeTx(
			tx, user, context.CPO.ID, purpose, metadata, template, now,
		)
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

func (service *Service) lockAuthChallenge(
	tx *gorm.DB,
	id uuid.UUID,
	purpose constants.AuthChallengePurpose,
) (models.AuthChallenge, models.User, error) {
	var challenge models.AuthChallenge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND purpose = ?", id, purpose).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return challenge, models.User{}, errInvalidChallenge
		}
		return challenge, models.User{}, fmt.Errorf("lock customer authentication challenge: %w", err)
	}
	var user models.User
	if err := tx.First(&user, "id = ?", challenge.UserID).Error; err != nil {
		return challenge, user, fmt.Errorf("load challenged customer identity: %w", err)
	}
	return challenge, user, nil
}

func (service *Service) authOTPHash(
	id uuid.UUID,
	purpose constants.AuthChallengePurpose,
	code string,
) []byte {
	mac := hmac.New(sha256.New, service.config.OTPHMACKey)
	_, _ = mac.Write([]byte(id.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (service *Service) verifyAuthOTP(challenge models.AuthChallenge, code string) bool {
	return hmac.Equal(
		service.authOTPHash(challenge.ID, challenge.Purpose, code),
		challenge.CodeHash,
	)
}

func authChallengeUsable(challenge models.AuthChallenge, now time.Time) bool {
	return challenge.ConsumedAt == nil && challenge.InvalidatedAt == nil &&
		challenge.Attempts < challenge.MaxAttempts && now.Before(challenge.ExpiresAt)
}

func recordAuthChallengeFailure(
	tx *gorm.DB,
	challenge models.AuthChallenge,
	now time.Time,
) error {
	updates := map[string]any{"attempts": challenge.Attempts + 1}
	if challenge.Attempts+1 >= challenge.MaxAttempts {
		updates["invalidated_at"] = now
	}
	if err := tx.Model(&models.AuthChallenge{}).Where("id = ?", challenge.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("record customer challenge failure: %w", err)
	}
	return nil
}

func (service *Service) recordFailedLogin(
	ctx context.Context,
	userID uuid.UUID,
	now time.Time,
) error {
	result := service.database.WithContext(ctx).Exec(`
		UPDATE users
		SET failed_login_attempts = failed_login_attempts + 1,
		    locked_until = CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END,
		    updated_at = ?
		WHERE id = ?
	`, service.config.LoginMaxAttempts, now.Add(service.config.LoginLockDuration), now, userID)
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
	return &APIError{
		http.StatusBadRequest, "missing_cpo_app_id",
		"X-CPO-App-ID is required.",
	}
}
