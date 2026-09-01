package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"gorm.io/gorm"
)

// ErrCPOAccessDenied identifies an expected authorization-domain miss, not an
// infrastructure failure. Callers must still fail closed, but must not report a
// database failure as if a valid authorization decision had denied access.
var ErrCPOAccessDenied = errors.New("CPO access denied")

// CPOAccess is a fresh projection of the membership that authorizes one
// request. It deliberately comes from the database rather than token role
// claims so suspension, role changes, and overrides take effect immediately.
type CPOAccess struct {
	Membership   models.CPOMembership
	RoleDefaults []string
	Allow        []string
	Deny         []string
	Effective    []string
}

func EvaluateCPOAccess(ctx context.Context, database *gorm.DB, principal Principal) (CPOAccess, error) {
	if database == nil {
		return CPOAccess{}, fmt.Errorf("evaluate CPO access: database is unavailable")
	}
	if principal.Scope != constants.AuthScopeCPO || principal.CPOID == nil {
		return CPOAccess{}, fmt.Errorf("%w: invalid CPO principal", ErrCPOAccessDenied)
	}
	var membership models.CPOMembership
	if err := database.WithContext(ctx).Where("cpo_id = ? AND user_id = ? AND status = ?", *principal.CPOID, principal.UserID, constants.MembershipStatusActive).First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CPOAccess{}, fmt.Errorf("%w: active CPO membership", ErrCPOAccessDenied)
		}
		return CPOAccess{}, fmt.Errorf("load active CPO membership: %w", err)
	}
	var overrides []models.CPOMembershipPermissionOverride
	if err := database.WithContext(ctx).Where("membership_id = ?", membership.ID).Find(&overrides).Error; err != nil {
		return CPOAccess{}, fmt.Errorf("load CPO permission overrides: %w", err)
	}
	access := CPOAccess{Membership: membership, RoleDefaults: cpopermissions.RoleDefaults(membership.Role)}
	for _, override := range overrides {
		if !cpopermissions.Known(override.Permission) {
			continue
		}
		switch override.Effect {
		case "DENY":
			access.Deny = append(access.Deny, override.Permission)
		case "ALLOW":
			access.Allow = append(access.Allow, override.Permission)
		}
	}
	sort.Strings(access.Allow)
	sort.Strings(access.Deny)
	access.Effective = cpopermissions.Effective(membership.Role, access.Allow, access.Deny)
	return access, nil
}

// CPOAuthorizationError converts a failed evaluator call into the stable,
// safe HTTP semantics shared by middleware and service defense-in-depth.
func CPOAuthorizationError(err error) *APIError {
	if errors.Is(err, ErrCPOAccessDenied) {
		return errForbidden
	}
	return &APIError{
		Status:  500,
		Code:    "internal_error",
		Message: "The request could not be completed.",
	}
}

func EvaluateCPOPermission(ctx context.Context, database *gorm.DB, principal Principal, permission string) (CPOAccess, bool, error) {
	if !cpopermissions.Known(permission) {
		return CPOAccess{}, false, fmt.Errorf("unknown CPO permission")
	}
	access, err := EvaluateCPOAccess(ctx, database, principal)
	if err != nil {
		return CPOAccess{}, false, err
	}
	for _, allowed := range access.Effective {
		if allowed == permission {
			return access, true, nil
		}
	}
	return access, false, nil
}
