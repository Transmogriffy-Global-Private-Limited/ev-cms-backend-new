package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func SeedSuperadmin(
	ctx context.Context,
	database *gorm.DB,
	seed config.Superadmin,
) error {
	email := strings.ToLower(strings.TrimSpace(seed.Email))
	if email == "" || seed.Password == "" {
		return errors.New("superadmin bootstrap credentials are required")
	}

	return database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT pg_advisory_xact_lock(hashtext(?))",
			"ev_cms_superadmin_bootstrap",
		).Error; err != nil {
			return fmt.Errorf("lock superadmin bootstrap: %w", err)
		}
		var user models.User
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lower(btrim(email)) = ?", email).
			First(&user)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find bootstrap superadmin: %w", result.Error)
		}

		created := false
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			passwordHash, err := security.HashPassword(seed.Password)
			if err != nil {
				return fmt.Errorf("hash bootstrap superadmin password: %w", err)
			}
			now := time.Now().UTC()
			user = models.User{
				ID:                uuid.New(),
				Email:             email,
				PasswordHash:      passwordHash,
				FullName:          strings.TrimSpace(seed.FullName),
				IsActive:          true,
				IsVerified:        true,
				MFAEnabled:        true,
				PasswordChangedAt: now,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			if user.FullName == "" {
				user.FullName = "Platform Superadmin"
			}
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("create bootstrap superadmin identity: %w", err)
			}
			created = true
		}

		admin := models.PlatformAdmin{
			UserID:    user.ID,
			CreatedAt: time.Now().UTC(),
		}
		result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&admin)
		if result.Error != nil {
			return fmt.Errorf("grant bootstrap platform authority: %w", result.Error)
		}
		if created || result.RowsAffected > 0 {
			action := "PLATFORM_SUPERADMIN_BOOTSTRAPPED"
			details := models.JSONB{"identity_created": created}
			audit := models.AuditLog{
				ID:        uuid.New(),
				UserID:    &user.ID,
				Action:    action,
				Entity:    "USER",
				EntityID:  &user.ID,
				Details:   details,
				CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&audit).Error; err != nil {
				return fmt.Errorf("audit bootstrap superadmin: %w", err)
			}
		}
		return nil
	})
}
