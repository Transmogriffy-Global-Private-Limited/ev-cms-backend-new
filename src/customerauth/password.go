package customerauth

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
)

func (service *Service) ForgotPassword(
	ctx context.Context,
	appID string,
	request ForgotPasswordRequest,
	metadata RequestMetadata,
) error {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return missingAppIDError()
	}
	cpo, err := activeCPO(service.database.WithContext(ctx), appID)
	if err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if !validEmail(email) {
		return nil
	}
	if !service.mailEnabled {
		return errMailUnavailable
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_PASSWORD_FORGOT", rateLimitAddress(metadata)); err != nil {
		return err
	}
	var user models.User
	if err := service.database.WithContext(ctx).Where(
		"lower(btrim(email)) = ? AND is_active = true", email,
	).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("find customer recovery identity: %w", err)
	}
	var customerCount int64
	if err := service.database.WithContext(ctx).Model(&models.Customer{}).Where(
		"cpo_id = ? AND user_id = ? AND status = ?",
		cpo.ID, user.ID, constants.CustomerStatusActive,
	).Count(&customerCount).Error; err != nil {
		return fmt.Errorf("check customer recovery relationship: %w", err)
	}
	if customerCount != 1 {
		return nil
	}
	_, err = service.createAuthChallenge(
		ctx, user, cpo.ID, constants.ChallengeCustomerReset,
		metadata, customerResetMailTemplate,
	)
	return err
}

func (service *Service) ResendPasswordReset(
	ctx context.Context,
	appID string,
	request ResendRequest,
	metadata RequestMetadata,
) (ChallengeResponse, error) {
	return service.resendAuthChallenge(
		ctx, appID, request, metadata,
		constants.ChallengeCustomerReset, customerResetMailTemplate,
	)
}

func (service *Service) ResetPassword(
	ctx context.Context,
	appID string,
	request ResetPasswordRequest,
	metadata RequestMetadata,
) error {
	if strings.TrimSpace(appID) == "" {
		return missingAppIDError()
	}
	if request.ChallengeID == uuid.Nil || !validOTP(request.Code) {
		return errInvalidChallenge
	}
	if err := security.ValidatePassword(request.NewPassword); err != nil {
		return &APIError{http.StatusBadRequest, "invalid_password", err.Error()}
	}
	if err := service.checkRateLimit(ctx, "CUSTOMER_PASSWORD_RESET", rateLimitAddress(metadata)); err != nil {
		return err
	}
	passwordHash, err := security.HashPassword(request.NewPassword)
	if err != nil {
		return err
	}
	var outcome error
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		challenge, user, err := service.lockAuthChallenge(
			tx, request.ChallengeID, constants.ChallengeCustomerReset,
		)
		if err != nil {
			outcome = err
			return nil
		}
		context, err := service.loadCustomerContextForRecovery(tx, user.ID, challenge.CPOID)
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
			return fmt.Errorf("consume customer password-reset challenge: %w", err)
		}
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).
			Updates(map[string]any{
				"password_hash": passwordHash, "password_changed_at": now,
				"failed_login_attempts": 0, "locked_until": nil,
				"must_change_password": false, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("reset customer identity password: %w", err)
		}
		if err := revokeUserSessionsTx(tx, user.ID, "PASSWORD_RESET", now); err != nil {
			return err
		}
		return createAudit(
			tx, user.ID, context.CPO.ID, "CUSTOMER_PASSWORD_RESET",
			"USER", user.ID, models.JSONB{}, now,
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
		return &APIError{http.StatusBadRequest, "invalid_password", err.Error()}
	}
	if request.CurrentPassword == request.NewPassword {
		return &APIError{
			http.StatusBadRequest, "password_reused",
			"The new password must differ from the current password.",
		}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, "id = ?", principal.UserID).Error; err != nil {
			return fmt.Errorf("load customer identity for password change: %w", err)
		}
		matches, err := security.VerifyPassword(request.CurrentPassword, user.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify customer current password: %w", err)
		}
		if !matches {
			return &APIError{
				http.StatusUnauthorized, "invalid_current_password",
				"The current password is incorrect.",
			}
		}
		passwordHash, err := security.HashPassword(request.NewPassword)
		if err != nil {
			return err
		}
		now := service.now()
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).
			Updates(map[string]any{
				"password_hash": passwordHash, "password_changed_at": now,
				"must_change_password": false, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("change customer identity password: %w", err)
		}
		if err := revokeUserSessionsTx(tx, user.ID, "PASSWORD_CHANGED", now); err != nil {
			return err
		}
		return createAudit(
			tx, user.ID, principal.CPOID, "CUSTOMER_PASSWORD_CHANGED",
			"USER", user.ID, models.JSONB{}, now,
		)
	})
}
