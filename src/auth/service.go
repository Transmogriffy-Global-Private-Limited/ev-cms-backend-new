package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	loginMailTemplate         = "LOGIN_OTP"
	passwordResetMailTemplate = "PASSWORD_RESET_OTP"
)

var (
	errInvalidCredentials = &APIError{
		Status: http.StatusUnauthorized, Code: "invalid_credentials",
		Message: "The supplied credentials or scope are invalid.",
	}
	errInvalidChallenge = &APIError{
		Status: http.StatusUnauthorized, Code: "invalid_challenge",
		Message: "The verification challenge or code is invalid.",
	}
	errInvalidRefresh = &APIError{
		Status: http.StatusUnauthorized, Code: "invalid_refresh_token",
		Message: "The refresh token is invalid.",
	}
	errUnauthorized = &APIError{
		Status: http.StatusUnauthorized, Code: "unauthorized",
		Message: "Authentication is required.",
	}
	errForbidden = &APIError{
		Status: http.StatusForbidden, Code: "forbidden",
		Message: "You do not have access to this operation.",
	}
	errMailUnavailable = &APIError{
		Status: http.StatusServiceUnavailable, Code: "mail_unavailable",
		Message: "Email verification is temporarily unavailable.",
	}
	errRateLimited = &APIError{
		Status: http.StatusTooManyRequests, Code: "rate_limited",
		Message: "Too many attempts. Try again later.",
	}
)

type Service struct {
	database    *gorm.DB
	config      config.Auth
	mailEnabled bool
	outbox      *cmsmail.Outbox
	tokens      *security.TokenManager
	dummyHash   string
	now         func() time.Time
}

func NewService(
	database *gorm.DB,
	cfg config.Auth,
	mailEnabled bool,
	outbox *cmsmail.Outbox,
	tokens *security.TokenManager,
) (*Service, error) {
	dummyHash, err := security.HashPassword("invalid-login-password")
	if err != nil {
		return nil, fmt.Errorf("initialize password verifier: %w", err)
	}
	return &Service{
		database:    database,
		config:      cfg,
		mailEnabled: mailEnabled,
		outbox:      outbox,
		tokens:      tokens,
		dummyHash:   dummyHash,
		now:         func() time.Time { return time.Now().UTC() },
	}, nil
}

func (service *Service) Login(
	ctx context.Context,
	request LoginRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if !validEmail(email) || request.Password == "" || !request.Scope.Valid() {
		return ChallengeResponse{}, errInvalidCredentials
	}
	if request.Scope == constants.AuthScopePlatform && request.CPOID != nil {
		return ChallengeResponse{}, errInvalidCredentials
	}
	if request.Scope == constants.AuthScopeCPO &&
		(request.CPOID == nil || *request.CPOID == uuid.Nil) {
		return ChallengeResponse{}, errInvalidCredentials
	}
	if !service.mailEnabled {
		return ChallengeResponse{}, errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "LOGIN", rateLimitAddress(metadata)); err != nil {
		return ChallengeResponse{}, err
	}

	var user models.User
	query := service.database.WithContext(ctx).
		Where("lower(btrim(email)) = ?", email).
		First(&user)
	hash := service.dummyHash
	if query.Error == nil {
		hash = user.PasswordHash
	} else if !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return ChallengeResponse{}, fmt.Errorf("find login identity: %w", query.Error)
	}
	passwordMatches, verifyErr := security.VerifyPassword(request.Password, hash)
	if verifyErr != nil {
		return ChallengeResponse{}, fmt.Errorf("verify login password: %w", verifyErr)
	}
	now := service.now()
	if query.Error != nil || !passwordMatches || !user.IsActive ||
		(user.LockedUntil != nil && user.LockedUntil.After(now)) {
		if query.Error == nil && !passwordMatches {
			if err := service.recordFailedLogin(ctx, user.ID, now); err != nil {
				return ChallengeResponse{}, err
			}
		}
		return ChallengeResponse{}, errInvalidCredentials
	}

	role, err := service.resolveLoginScope(ctx, user.ID, request.Scope, request.CPOID)
	if err != nil {
		return ChallengeResponse{}, err
	}
	if err := service.database.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"failed_login_attempts": 0,
			"locked_until":          nil,
			"updated_at":            now,
		}).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("reset failed login state: %w", err)
	}

	return service.createChallenge(
		ctx,
		user,
		constants.ChallengeLogin2FA,
		&request.Scope,
		request.CPOID,
		role,
		metadata,
		loginMailTemplate,
	)
}

func (service *Service) VerifyLoginChallenge(
	ctx context.Context,
	request ChallengeRequest,
	metadata RequestMetadata,
) (TokenResponse, error) {
	if request.ChallengeID == uuid.Nil || !validOTP(request.Code) {
		return TokenResponse{}, errInvalidChallenge
	}
	if err := service.checkRateLimit(
		ctx,
		"VERIFY_LOGIN_OTP",
		rateLimitAddress(metadata),
	); err != nil {
		return TokenResponse{}, err
	}

	var response TokenResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, user, err := service.lockChallenge(
			tx,
			request.ChallengeID,
			constants.ChallengeLogin2FA,
		)
		if err != nil {
			outcome = err
			return nil
		}
		now := service.now()
		if !service.challengeUsable(challenge, now) {
			outcome = errInvalidChallenge
			return nil
		}
		if !service.verifyOTP(challenge, request.Code) {
			if err := service.recordChallengeFailure(tx, challenge, now); err != nil {
				return err
			}
			outcome = errInvalidChallenge
			return nil
		}
		if challenge.Scope == nil {
			outcome = errInvalidChallenge
			return nil
		}
		role, err := service.resolveLoginScopeTx(
			tx,
			user.ID,
			*challenge.Scope,
			challenge.CPOID,
		)
		if err != nil {
			outcome = err
			return nil
		}
		if err := tx.Model(&models.AuthChallenge{}).
			Where("id = ? AND consumed_at IS NULL", challenge.ID).
			Update("consumed_at", now).Error; err != nil {
			return fmt.Errorf("consume login challenge: %w", err)
		}
		response, err = service.createSession(
			tx,
			user,
			*challenge.Scope,
			challenge.CPOID,
			role,
			metadata,
			now,
		)
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

func (service *Service) ResendLoginChallenge(
	ctx context.Context,
	request ResendRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	if request.ChallengeID == uuid.Nil || !service.mailEnabled {
		if !service.mailEnabled {
			return ChallengeResponse{}, errMailUnavailable
		}
		return ChallengeResponse{}, errInvalidChallenge
	}
	if err := service.checkRateLimit(
		ctx,
		"RESEND_LOGIN_OTP",
		rateLimitAddress(metadata),
	); err != nil {
		return ChallengeResponse{}, err
	}

	var response ChallengeResponse
	var outcome error
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, user, err := service.lockChallenge(
			tx,
			request.ChallengeID,
			constants.ChallengeLogin2FA,
		)
		if err != nil {
			outcome = err
			return nil
		}
		now := service.now()
		if challenge.ConsumedAt != nil || challenge.InvalidatedAt != nil ||
			!now.Before(challenge.ExpiresAt) ||
			now.Before(challenge.ResendAvailableAt) {
			outcome = errInvalidChallenge
			return nil
		}
		if challenge.Scope == nil {
			outcome = errInvalidChallenge
			return nil
		}
		if _, err := service.resolveLoginScopeTx(
			tx,
			user.ID,
			*challenge.Scope,
			challenge.CPOID,
		); err != nil {
			outcome = err
			return nil
		}
		if err := tx.Model(&models.AuthChallenge{}).
			Where("id = ?", challenge.ID).
			Update("invalidated_at", now).Error; err != nil {
			return fmt.Errorf("invalidate previous login challenge: %w", err)
		}
		response, err = service.createChallengeTx(
			tx,
			user,
			constants.ChallengeLogin2FA,
			challenge.Scope,
			challenge.CPOID,
			metadata,
			loginMailTemplate,
			now,
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

func (service *Service) createChallenge(
	ctx context.Context,
	user models.User,
	purpose constants.AuthChallengePurpose,
	scope *constants.AuthScope,
	cpoID *uuid.UUID,
	_ *constants.CPORole,
	metadata RequestMetadata,
	template string,
) (ChallengeResponse, error) {
	var response ChallengeResponse
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		response, err = service.createChallengeTx(
			tx,
			user,
			purpose,
			scope,
			cpoID,
			metadata,
			template,
			service.now(),
		)
		return err
	})
	return response, err
}

func (service *Service) createChallengeTx(
	tx *gorm.DB,
	user models.User,
	purpose constants.AuthChallengePurpose,
	scope *constants.AuthScope,
	cpoID *uuid.UUID,
	metadata RequestMetadata,
	template string,
	now time.Time,
) (ChallengeResponse, error) {
	code, err := security.RandomDigits(6)
	if err != nil {
		return ChallengeResponse{}, err
	}
	challenge := models.AuthChallenge{
		ID:                uuid.New(),
		UserID:            user.ID,
		Purpose:           purpose,
		Scope:             scope,
		CPOID:             cpoID,
		ExpiresAt:         now.Add(service.config.OTPExpiry),
		MaxAttempts:       5,
		ResendAvailableAt: now.Add(service.config.OTPResendCooldown),
		RequestIP:         metadata.IPAddress,
		UserAgent:         boundedUserAgent(metadata.UserAgent),
		CreatedAt:         now,
	}
	challenge.CodeHash = service.otpHash(challenge.ID, purpose, code)
	if err := tx.Model(&models.AuthChallenge{}).
		Where(
			"user_id = ? AND purpose = ? AND consumed_at IS NULL AND invalidated_at IS NULL",
			user.ID,
			purpose,
		).
		Update("invalidated_at", now).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("invalidate prior authentication challenges: %w", err)
	}
	if err := tx.Create(&challenge).Error; err != nil {
		return ChallengeResponse{}, fmt.Errorf("create authentication challenge: %w", err)
	}
	payload := challengeOTPPayload(user, challenge, code)
	if err := service.outbox.EnqueueOTP(
		tx,
		user.Email,
		template,
		payload,
	); err != nil {
		return ChallengeResponse{}, err
	}
	return ChallengeResponse{
		ChallengeID:       challenge.ID,
		ExpiresAt:         challenge.ExpiresAt,
		ResendAvailableAt: challenge.ResendAvailableAt,
	}, nil
}

func challengeOTPPayload(
	user models.User,
	challenge models.AuthChallenge,
	code string,
) cmsmail.OTPPayload {
	payload := cmsmail.OTPPayload{
		RecipientName: user.FullName,
		Code:          code,
		ExpiresAt:     challenge.ExpiresAt,
	}
	if challenge.Purpose == constants.ChallengePasswordReset {
		payload.ChallengeID = challenge.ID.String()
	}
	return payload
}

func (service *Service) lockChallenge(
	tx *gorm.DB,
	id uuid.UUID,
	purpose constants.AuthChallengePurpose,
) (models.AuthChallenge, models.User, error) {
	var challenge models.AuthChallenge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND purpose = ?", id, purpose).
		First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.AuthChallenge{}, models.User{}, errInvalidChallenge
		}
		return models.AuthChallenge{}, models.User{}, fmt.Errorf("lock authentication challenge: %w", err)
	}
	var user models.User
	if err := tx.First(&user, "id = ?", challenge.UserID).Error; err != nil {
		return models.AuthChallenge{}, models.User{}, fmt.Errorf("load challenged identity: %w", err)
	}
	return challenge, user, nil
}

func (service *Service) challengeUsable(challenge models.AuthChallenge, now time.Time) bool {
	return challenge.ConsumedAt == nil &&
		challenge.InvalidatedAt == nil &&
		challenge.Attempts < challenge.MaxAttempts &&
		now.Before(challenge.ExpiresAt)
}

func (service *Service) recordChallengeFailure(
	tx *gorm.DB,
	challenge models.AuthChallenge,
	now time.Time,
) error {
	updates := map[string]any{"attempts": challenge.Attempts + 1}
	if challenge.Attempts+1 >= challenge.MaxAttempts {
		updates["invalidated_at"] = now
	}
	if err := tx.Model(&models.AuthChallenge{}).
		Where("id = ?", challenge.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("record challenge failure: %w", err)
	}
	return nil
}

func (service *Service) otpHash(
	challengeID uuid.UUID,
	purpose constants.AuthChallengePurpose,
	code string,
) []byte {
	mac := hmac.New(sha256.New, service.config.OTPHMACKey)
	_, _ = mac.Write([]byte(challengeID.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (service *Service) verifyOTP(challenge models.AuthChallenge, code string) bool {
	actual := service.otpHash(challenge.ID, challenge.Purpose, code)
	return hmac.Equal(actual, challenge.CodeHash)
}

func (service *Service) resolveLoginScope(
	ctx context.Context,
	userID uuid.UUID,
	scope constants.AuthScope,
	cpoID *uuid.UUID,
) (*constants.CPORole, error) {
	return service.resolveLoginScopeTx(service.database.WithContext(ctx), userID, scope, cpoID)
}

func (service *Service) resolveLoginScopeTx(
	tx *gorm.DB,
	userID uuid.UUID,
	scope constants.AuthScope,
	cpoID *uuid.UUID,
) (*constants.CPORole, error) {
	if scope == constants.AuthScopePlatform {
		var count int64
		if err := tx.Model(&models.PlatformAdmin{}).
			Where("user_id = ?", userID).
			Count(&count).Error; err != nil {
			return nil, fmt.Errorf("check platform authority: %w", err)
		}
		if count != 1 {
			return nil, errInvalidCredentials
		}
		return nil, nil
	}
	if scope != constants.AuthScopeCPO || cpoID == nil {
		return nil, errInvalidCredentials
	}
	var membership models.CPOMembership
	if err := tx.
		Joins("JOIN cpos ON cpos.id = cpo_memberships.cpo_id").
		Where(
			"cpo_memberships.user_id = ? AND cpo_memberships.cpo_id = ? AND cpo_memberships.role = ? AND cpo_memberships.status = ? AND cpos.status = ?",
			userID,
			*cpoID,
			constants.CPORoleAdmin,
			constants.MembershipStatusActive,
			constants.CPOStatusActive,
		).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errInvalidCredentials
		}
		return nil, fmt.Errorf("check CPO authority: %w", err)
	}
	role := membership.Role
	return &role, nil
}

func (service *Service) recordFailedLogin(
	ctx context.Context,
	userID uuid.UUID,
	now time.Time,
) error {
	result := service.database.WithContext(ctx).Exec(`
		UPDATE users
		SET failed_login_attempts = failed_login_attempts + 1,
		    locked_until = CASE
		        WHEN failed_login_attempts + 1 >= ? THEN ?
		        ELSE locked_until
		    END,
		    updated_at = ?
		WHERE id = ?
	`, service.config.LoginMaxAttempts, now.Add(service.config.LoginLockDuration), now, userID)
	if result.Error != nil {
		return fmt.Errorf("record failed login: %w", result.Error)
	}
	return nil
}

func (service *Service) checkRateLimit(
	ctx context.Context,
	action string,
	material string,
) error {
	sum := sha256.Sum256([]byte(material))
	scopeKey := hex.EncodeToString(sum[:])
	now := service.now()
	var blockedUntil sql.NullTime
	result := service.database.WithContext(ctx).Raw(`
		INSERT INTO auth_rate_limits (
		    scope_key, action, window_started_at, attempt_count, blocked_until, updated_at
		)
		VALUES (?, ?, ?, 1, NULL, ?)
		ON CONFLICT (scope_key, action) DO UPDATE
		SET window_started_at = CASE
		        WHEN auth_rate_limits.window_started_at <= ? THEN excluded.window_started_at
		        ELSE auth_rate_limits.window_started_at
		    END,
		    attempt_count = CASE
		        WHEN auth_rate_limits.window_started_at <= ? THEN 1
		        ELSE auth_rate_limits.attempt_count + 1
		    END,
		    blocked_until = CASE
		        WHEN auth_rate_limits.blocked_until > ? THEN auth_rate_limits.blocked_until
		        WHEN auth_rate_limits.window_started_at > ?
		             AND auth_rate_limits.attempt_count + 1 > ?
		            THEN ?
		        ELSE NULL
		    END,
		    updated_at = excluded.updated_at
		RETURNING blocked_until
	`,
		scopeKey,
		action,
		now,
		now,
		now.Add(-service.config.RateLimitWindow),
		now.Add(-service.config.RateLimitWindow),
		now,
		now.Add(-service.config.RateLimitWindow),
		service.config.RateLimitMax,
		now.Add(service.config.RateLimitWindow),
	).Scan(&blockedUntil)
	if result.Error != nil {
		return fmt.Errorf("apply authentication rate limit: %w", result.Error)
	}
	if err := service.database.WithContext(ctx).
		Where(
			"updated_at < ? AND (blocked_until IS NULL OR blocked_until < ?)",
			now.Add(-7*24*time.Hour),
			now,
		).
		Delete(&models.AuthRateLimit{}).Error; err != nil {
		return fmt.Errorf("prune authentication rate limits: %w", err)
	}
	if blockedUntil.Valid && blockedUntil.Time.After(now) {
		return errRateLimited
	}
	return nil
}

func validOTP(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validEmail(value string) bool {
	if value == "" || len(value) > 320 {
		return false
	}
	address, err := netmail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func rateLimitAddress(metadata RequestMetadata) string {
	address := "unknown"
	if metadata.IPAddress != nil {
		address = *metadata.IPAddress
	}
	return address
}

func boundedUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
