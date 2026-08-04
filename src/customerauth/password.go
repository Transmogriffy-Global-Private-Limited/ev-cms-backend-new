package customerauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (service *Service) ForgotPassword(ctx context.Context, appID string, request ForgotPasswordRequest, metadata RequestMetadata) error {
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
	var customer models.Customer
	if err := service.database.WithContext(ctx).Where(
		"cpo_id = ? AND lower(btrim(email)) = ? AND status = ?", cpo.ID, email, constants.CustomerStatusActive,
	).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("find customer recovery account: %w", err)
	}
	_, err = service.createAuthChallenge(ctx, customer, constants.ChallengeCustomerReset, metadata, customerResetMailTemplate)
	return err
}

func (service *Service) ResendPasswordReset(ctx context.Context, appID string, request ResendRequest, metadata RequestMetadata) (ChallengeResponse, error) {
	return service.resendAuthChallenge(ctx, appID, request, metadata, constants.ChallengeCustomerReset, customerResetMailTemplate)
}

func (service *Service) ResetPassword(ctx context.Context, appID string, request ResetPasswordRequest, metadata RequestMetadata) error {
	appID = strings.TrimSpace(appID)
	if appID == "" {
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
		challenge, customer, err := service.lockAuthChallenge(tx, request.ChallengeID, constants.ChallengeCustomerReset)
		if err != nil {
			outcome = err
			return nil
		}
		context, err := service.loadCustomerContext(tx, customer.ID, customer.CPOID, true)
		if err != nil || context.CPO.AppID != appID {
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
		if err := tx.Model(&models.CustomerAuthChallenge{}).Where("id = ? AND consumed_at IS NULL", challenge.ID).Update("consumed_at", now).Error; err != nil {
			return fmt.Errorf("consume customer password-reset challenge: %w", err)
		}
		if err := tx.Model(&models.Customer{}).Where("id = ? AND cpo_id = ?", customer.ID, customer.CPOID).Updates(map[string]any{
			"password_hash": passwordHash, "password_changed_at": now, "failed_login_attempts": 0, "locked_until": nil, "updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("reset customer password: %w", err)
		}
		if err := revokeCustomerSessionsTx(tx, customer.CPOID, customer.ID, "PASSWORD_RESET", now); err != nil {
			return err
		}
		if err := invalidateCustomerChallengesTx(tx, customer.CPOID, customer.ID, now); err != nil {
			return err
		}
		return createCustomerAudit(tx, customer.ID, customer.CPOID, "CUSTOMER_PASSWORD_RESET", "CUSTOMER", customer.ID, models.JSONB{}, now)
	})
	if err != nil {
		return err
	}
	return outcome
}

func (service *Service) ChangePassword(ctx context.Context, principal Principal, request ChangePasswordRequest) error {
	if err := security.ValidatePassword(request.NewPassword); err != nil {
		return &APIError{http.StatusBadRequest, "invalid_password", err.Error()}
	}
	if request.CurrentPassword == request.NewPassword {
		return &APIError{http.StatusBadRequest, "password_reused", "The new password must differ from the current password."}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customer models.Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND cpo_id = ?", principal.CustomerID, principal.CPOID).First(&customer).Error; err != nil {
			return fmt.Errorf("load customer account for password change: %w", err)
		}
		matches, err := security.VerifyPassword(request.CurrentPassword, customer.PasswordHash)
		if err != nil {
			return fmt.Errorf("verify customer current password: %w", err)
		}
		if !matches {
			return &APIError{http.StatusUnauthorized, "invalid_current_password", "The current password is incorrect."}
		}
		passwordHash, err := security.HashPassword(request.NewPassword)
		if err != nil {
			return err
		}
		now := service.now()
		if err := tx.Model(&models.Customer{}).Where("id = ? AND cpo_id = ?", customer.ID, customer.CPOID).
			Updates(map[string]any{"password_hash": passwordHash, "password_changed_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("change customer password: %w", err)
		}
		if err := revokeCustomerSessionsTx(tx, customer.CPOID, customer.ID, "PASSWORD_CHANGED", now); err != nil {
			return err
		}
		if err := invalidateCustomerChallengesTx(tx, customer.CPOID, customer.ID, now); err != nil {
			return err
		}
		return createCustomerAudit(tx, customer.ID, customer.CPOID, "CUSTOMER_PASSWORD_CHANGED", "CUSTOMER", customer.ID, models.JSONB{}, now)
	})
}

func invalidateCustomerChallengesTx(tx *gorm.DB, cpoID, customerID uuid.UUID, now time.Time) error {
	if err := tx.Model(&models.CustomerAuthChallenge{}).Where(
		"cpo_id = ? AND customer_id = ? AND consumed_at IS NULL AND invalidated_at IS NULL",
		cpoID, customerID,
	).Update("invalidated_at", now).Error; err != nil {
		return fmt.Errorf("invalidate customer authentication challenges: %w", err)
	}
	return nil
}
