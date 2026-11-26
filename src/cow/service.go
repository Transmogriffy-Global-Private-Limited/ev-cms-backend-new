// Package cow implements the CPO-scoped Charger on Wheels module entitlement.
// The entitlement is independent of staff capabilities: BOTH gates must pass.
package cow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ModuleKey = "cow"

// EntitlementView represents the committed grant or default-off absence of a row.
// Nil changer fields on a version-zero view are intentional, not unknown grants.
type EntitlementView struct {
	CPOID           uuid.UUID  `json:"cpo_id"`
	Module          string     `json:"module"`
	Enabled         bool       `json:"enabled"`
	Version         int64      `json:"version"`
	Reason          string     `json:"reason,omitempty"`
	ChangedByUserID *uuid.UUID `json:"changed_by_user_id,omitempty"`
	ChangedAt       *time.Time `json:"changed_at,omitempty"`
}

type ChangeRequest struct {
	Enabled         *bool  `json:"enabled"`
	ExpectedVersion *int64 `json:"expected_version"`
	Reason          string `json:"reason"`
}

type AccessView struct {
	CPOID                uuid.UUID `json:"cpo_id"`
	Module               string    `json:"module"`
	Enabled              bool      `json:"enabled"`
	Eligible             bool      `json:"eligible"`
	EffectivePermissions []string  `json:"effective_permissions"`
}

type grantRow struct {
	CPOID           uuid.UUID `gorm:"column:cpo_id"`
	ModuleKey       string    `gorm:"column:module_key"`
	Enabled         bool      `gorm:"column:enabled"`
	Version         int64     `gorm:"column:version"`
	Reason          string    `gorm:"column:reason"`
	ChangedByUserID uuid.UUID `gorm:"column:changed_by_user_id"`
	ChangedAt       time.Time `gorm:"column:changed_at"`
}

func (grantRow) TableName() string { return "cpo_module_entitlements" }

func apiError(status int, code, message string) error {
	return &auth.APIError{Status: status, Code: code, Message: message}
}

func forbidden() error {
	return apiError(http.StatusForbidden, "forbidden", "You do not have access to this operation.")
}

func internal() error {
	return apiError(http.StatusInternalServerError, "internal_error", "The request could not be completed.")
}

func viewOf(cpoID uuid.UUID, row grantRow) EntitlementView {
	view := EntitlementView{CPOID: cpoID, Module: ModuleKey, Enabled: row.Enabled, Version: row.Version}
	if row.Version > 0 {
		actor := row.ChangedByUserID
		when := row.ChangedAt
		view.Reason, view.ChangedByUserID, view.ChangedAt = row.Reason, &actor, &when
	}
	return view
}

func getEntitlement(ctx context.Context, database *gorm.DB, cpoID uuid.UUID) (EntitlementView, error) {
	var row grantRow
	err := database.WithContext(ctx).Where("cpo_id = ? AND module_key = ?", cpoID, ModuleKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return viewOf(cpoID, grantRow{}), nil
	}
	if err != nil {
		return EntitlementView{}, err
	}
	return viewOf(cpoID, row), nil
}

func currentCPO(ctx context.Context, database *gorm.DB, cpoID uuid.UUID) (models.CPO, error) {
	var cpo models.CPO
	err := database.WithContext(ctx).Select("id", "status").Where("id = ?", cpoID).Take(&cpo).Error
	return cpo, err
}

func requirePlatform(ctx context.Context, db *gorm.DB, principal auth.Principal) error {
	if principal.Scope != constants.AuthScopePlatform || principal.UserID == uuid.Nil {
		return forbidden()
	}
	var count int64
	if err := db.WithContext(ctx).Table("platform_admins").Where("user_id = ? AND is_active = true", principal.UserID).Count(&count).Error; err != nil {
		return internal()
	}
	if count != 1 {
		return forbidden()
	}
	return nil
}

// GetPlatformEntitlement never infers a grant from a CPO's general status.
func GetPlatformEntitlement(ctx context.Context, database *gorm.DB, principal auth.Principal, cpoID uuid.UUID) (EntitlementView, error) {
	if database == nil {
		return EntitlementView{}, internal()
	}
	if err := requirePlatform(ctx, database, principal); err != nil {
		return EntitlementView{}, err
	}
	if cpoID == uuid.Nil {
		return EntitlementView{}, apiError(400, "invalid_cpo_id", "CPO ID is invalid.")
	}
	if _, err := currentCPO(ctx, database, cpoID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EntitlementView{}, apiError(404, "cpo_not_found", "CPO was not found.")
		}
		return EntitlementView{}, internal()
	}
	view, err := getEntitlement(ctx, database, cpoID)
	if err != nil {
		return EntitlementView{}, internal()
	}
	return view, nil
}

// SetPlatformEntitlement serializes grant changes on the CPO row. A version
// precondition prevents stale administrative panels from undoing newer grants.
// The event and platform audit are committed in the SAME database transaction.
func SetPlatformEntitlement(ctx context.Context, database *gorm.DB, principal auth.Principal, cpoID uuid.UUID, change ChangeRequest) (EntitlementView, error) {
	if database == nil {
		return EntitlementView{}, internal()
	}
	if err := requirePlatform(ctx, database, principal); err != nil {
		return EntitlementView{}, err
	}
	if cpoID == uuid.Nil {
		return EntitlementView{}, apiError(400, "invalid_cpo_id", "CPO ID is invalid.")
	}
	if change.Enabled == nil || change.ExpectedVersion == nil || *change.ExpectedVersion < 0 {
		return EntitlementView{}, apiError(400, "invalid_request", "enabled and a nonnegative expected_version are required.")
	}
	reason := strings.TrimSpace(change.Reason)
	if len(reason) < 3 || len(reason) > 500 {
		return EntitlementView{}, apiError(400, "invalid_reason", "Reason must contain 3 to 500 UTF-8 bytes.")
	}

	var result EntitlementView
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cpo models.CPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", cpoID).Take(&cpo).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apiError(404, "cpo_not_found", "CPO was not found.")
			}
			return err
		}
		before, err := getEntitlement(ctx, tx, cpoID)
		if err != nil {
			return err
		}
		if before.Version != *change.ExpectedVersion {
			return apiError(409, "entitlement_version_conflict", "Entitlement version changed; read again before updating.")
		}
		if before.Enabled == *change.Enabled {
			result = before // Idempotent no-op; no misleading audit event.
			return nil
		}
		if before.Version == int64(^uint64(0)>>1) {
			return apiError(409, "entitlement_version_exhausted", "Entitlement version cannot be advanced.")
		}
		now := time.Now().UTC()
		row := grantRow{
			CPOID: cpoID, ModuleKey: ModuleKey, Enabled: *change.Enabled,
			Version: before.Version + 1, Reason: reason,
			ChangedByUserID: principal.UserID, ChangedAt: now,
		}
		if before.Version == 0 {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			res := tx.Model(&grantRow{}).Where("cpo_id = ? AND module_key = ? AND version = ?", cpoID, ModuleKey, before.Version).Updates(map[string]any{
				"enabled": row.Enabled, "version": row.Version, "reason": row.Reason,
				"changed_by_user_id": row.ChangedByUserID, "changed_at": row.ChangedAt,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return apiError(409, "entitlement_version_conflict", "Entitlement version changed; read again before updating.")
			}
		}
		if err := tx.Exec(`INSERT INTO cpo_module_entitlement_events
			(id, cpo_id, module_key, version, old_enabled, new_enabled, changed_by_user_id, reason, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New(), cpoID, ModuleKey, row.Version, before.Enabled, row.Enabled, principal.UserID, reason, now).Error; err != nil {
			return fmt.Errorf("record CoW entitlement change: %w", err)
		}
		actorID := principal.UserID
		log := models.AuditLog{
			ID: uuid.New(), CPOID: &cpoID, UserID: &actorID,
			Action: "CPO_MODULE_COW_CHANGED", Entity: "cpo", EntityID: &cpoID,
			Details:   models.JSONB{"module": ModuleKey, "old_enabled": before.Enabled, "new_enabled": row.Enabled, "version": row.Version, "reason": reason},
			CreatedAt: now,
		}
		if err := tx.Create(&log).Error; err != nil {
			return fmt.Errorf("audit CoW entitlement change: %w", err)
		}
		result = viewOf(cpoID, row)
		return nil
	})
	if err != nil {
		var ae *auth.APIError
		if errors.As(err, &ae) {
			return EntitlementView{}, ae
		}
		return EntitlementView{}, internal()
	}
	return result, nil
}

// GetCPOAccess is intentionally discoverable even when CoW is disabled, but
// only to an active tenant member. It never exposes another tenant's grant.
func GetCPOAccess(ctx context.Context, database *gorm.DB, principal auth.Principal) (AccessView, error) {
	if database == nil || principal.CPOID == nil || principal.Scope != constants.AuthScopeCPO || *principal.CPOID == uuid.Nil {
		return AccessView{}, forbidden()
	}
	access, err := auth.EvaluateCPOAccess(ctx, database, principal)
	if err != nil {
		if errors.Is(err, auth.ErrCPOAccessDenied) {
			return AccessView{}, forbidden()
		}
		return AccessView{}, internal()
	}
	cpo, err := currentCPO(ctx, database, *principal.CPOID)
	if err != nil {
		return AccessView{}, internal()
	}
	grant, err := getEntitlement(ctx, database, *principal.CPOID)
	if err != nil {
		return AccessView{}, internal()
	}
	active := cpo.Status == constants.CPOStatusActive
	view := AccessView{
		CPOID: *principal.CPOID, Module: ModuleKey,
		Enabled: grant.Enabled, Eligible: grant.Enabled && active,
		EffectivePermissions: []string{},
	}
	if view.Eligible {
		for _, permission := range access.Effective {
			if strings.HasPrefix(permission, "cow.") {
				view.EffectivePermissions = append(view.EffectivePermissions, permission)
			}
		}
	}
	return view, nil
}

// Authorize is the reusable fail-closed service guard. Future CoW operations
// MUST call it, not rely solely on route registration or UI role indicators.
// Mutations needing ordering against revocation must check again inside the
// mutation transaction while holding a compatible entitlement/CPO lock.
func Authorize(ctx context.Context, database *gorm.DB, principal auth.Principal, permission string) error {
	if database == nil || !strings.HasPrefix(permission, "cow.") || !cpopermissions.Known(permission) {
		return internal()
	}
	if principal.Scope != constants.AuthScopeCPO || principal.CPOID == nil || *principal.CPOID == uuid.Nil {
		return forbidden()
	}
	_, allowed, err := auth.EvaluateCPOPermission(ctx, database, principal, permission)
	if err != nil {
		if errors.Is(err, auth.ErrCPOAccessDenied) {
			return forbidden()
		}
		return internal()
	}
	if !allowed {
		return forbidden()
	}
	cpo, err := currentCPO(ctx, database, *principal.CPOID)
	if err != nil {
		return internal()
	}
	if cpo.Status != constants.CPOStatusActive {
		return forbidden()
	}
	grant, err := getEntitlement(ctx, database, *principal.CPOID)
	if err != nil {
		return internal()
	}
	if !grant.Enabled {
		return forbidden()
	}
	return nil
}
