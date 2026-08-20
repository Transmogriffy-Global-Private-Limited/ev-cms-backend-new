package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (service *Service) ForgotPassword(
	ctx context.Context,
	request ForgotPasswordRequest,
	metadata RequestMetadata,
) error {
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if !validEmail(email) {
		return nil
	}
	if !service.mailEnabled {
		return errMailUnavailable
	}
	if err := service.checkRateLimit(
		ctx,
		"PASSWORD_FORGOT",
		rateLimitAddress(metadata),
	); err != nil {
		return err
	}
	var user models.User
	if err := service.database.WithContext(ctx).
		Where("lower(btrim(email)) = ? AND is_active = true", email).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("find password-recovery identity: %w", err)
	}
	_, err := service.createChallenge(
		ctx,
		user,
		constants.ChallengePasswordReset,
		nil,
		nil,
		nil,
		metadata,
		passwordResetMailTemplate,
	)
	return err
}

func (service *Service) ResetPassword(
	ctx context.Context,
	request ResetPasswordRequest,
	metadata RequestMetadata,
) error {
	if request.ChallengeID == uuid.Nil || !validOTP(request.Code) {
		return errInvalidChallenge
	}
	if err := security.ValidatePassword(request.NewPassword); err != nil {
		return &APIError{
			Status: http.StatusBadRequest, Code: "invalid_password", Message: err.Error(),
		}
	}
	if err := service.checkRateLimit(
		ctx,
		"PASSWORD_RESET",
		rateLimitAddress(metadata),
	); err != nil {
		return err
	}

	passwordHash, err := security.HashPassword(request.NewPassword)
	if err != nil {
		return err
	}
	var outcome error
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, user, err := service.lockChallenge(
			tx,
			request.ChallengeID,
			constants.ChallengePasswordReset,
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
		if err := tx.Model(&models.AuthChallenge{}).
			Where("id = ? AND consumed_at IS NULL", challenge.ID).
			Update("consumed_at", now).Error; err != nil {
			return fmt.Errorf("consume password reset challenge: %w", err)
		}
		if err := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Updates(map[string]any{
				"password_hash":         passwordHash,
				"password_changed_at":   now,
				"failed_login_attempts": 0,
				"locked_until":          nil,
				"must_change_password":  false,
				"updated_at":            now,
			}).Error; err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		if err := service.revokeUserSessionsTx(
			tx,
			user.ID,
			"PASSWORD_RESET",
			now,
		); err != nil {
			return err
		}
		return service.audit(
			tx,
			&user.ID,
			nil,
			"AUTH_PASSWORD_RESET",
			"USER",
			&user.ID,
			models.JSONB{},
			now,
		)
	})
	if err != nil {
		return err
	}
	return outcome
}

func (service *Service) ChangePassword(
	ctx context.Context,
	principal Principal,
	request ChangePasswordRequest,
) error {
	if err := security.ValidatePassword(request.NewPassword); err != nil {
		return &APIError{
			Status: http.StatusBadRequest, Code: "invalid_password", Message: err.Error(),
		}
	}
	if request.CurrentPassword == request.NewPassword {
		return &APIError{
			Status: http.StatusBadRequest, Code: "password_reused",
			Message: "The new password must differ from the current password.",
		}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", principal.UserID).Error; err != nil {
			return fmt.Errorf("load identity for password change: %w", err)
		}
		matches, err := security.VerifyPassword(request.CurrentPassword, user.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify current password: %w", err)
		}
		if !matches {
			return &APIError{
				Status: http.StatusUnauthorized, Code: "invalid_current_password",
				Message: "The current password is incorrect.",
			}
		}
		passwordHash, err := security.HashPassword(request.NewPassword)
		if err != nil {
			return err
		}
		now := service.now()
		if err := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Updates(map[string]any{
				"password_hash":        passwordHash,
				"password_changed_at":  now,
				"must_change_password": false,
				"updated_at":           now,
			}).Error; err != nil {
			return fmt.Errorf("change password: %w", err)
		}
		if err := service.revokeUserSessionsTx(
			tx,
			user.ID,
			"PASSWORD_CHANGED",
			now,
		); err != nil {
			return err
		}
		return service.audit(
			tx,
			&user.ID,
			principal.CPOID,
			"AUTH_PASSWORD_CHANGED",
			"USER",
			&user.ID,
			models.JSONB{},
			now,
		)
	})
}
