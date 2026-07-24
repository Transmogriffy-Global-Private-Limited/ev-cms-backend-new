package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const CPOAppIDHeader = "X-CPO-App-ID"

const principalContextKey = "ev_cms_principal"
const accessTokenContextKey = "ev_cms_access_token"

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
		ctx.Set(accessTokenContextKey, parts[1])
		ctx.Next()
	}
}

func CurrentAccessToken(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get(accessTokenContextKey)
	if !exists {
		return "", false
	}
	token, ok := value.(string)
	return token, ok && token != ""
}

func RequirePlatform() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		principal, ok := CurrentPrincipal(ctx)
		if !ok || principal.Scope != constants.AuthScopePlatform {
			writeError(ctx, errForbidden)
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func RequireCPORoles(allowed ...constants.CPORole) gin.HandlerFunc {
	roles := make(map[constants.CPORole]struct{}, len(allowed))
	for _, role := range allowed {
		roles[role] = struct{}{}
	}
	return func(ctx *gin.Context) {
		principal, ok := CurrentPrincipal(ctx)
		if !ok || principal.Scope != constants.AuthScopeCPO ||
			principal.CPOID == nil || principal.Role == nil {
			writeError(ctx, errForbidden)
			ctx.Abort()
			return
		}
		if _, permitted := roles[*principal.Role]; !permitted {
			writeError(ctx, errForbidden)
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func RequireCPOAppID() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		principal, ok := CurrentPrincipal(ctx)
		if !ok || principal.Scope != constants.AuthScopeCPO ||
			principal.CPOID == nil || principal.CPOAppID == nil {
			writeError(ctx, errForbidden)
			ctx.Abort()
			return
		}
		if principal.User.MustChangePassword {
			writeError(ctx, &APIError{
				Status:  http.StatusForbidden,
				Code:    "password_change_required",
				Message: "Change the temporary password before using CPO operations.",
			})
			ctx.Abort()
			return
		}
		supplied := strings.TrimSpace(ctx.GetHeader(CPOAppIDHeader))
		if supplied == "" {
			writeError(ctx, &APIError{
				Status:  http.StatusBadRequest,
				Code:    "missing_cpo_app_id",
				Message: "The X-CPO-App-ID header is required.",
			})
			ctx.Abort()
			return
		}
		expected := *principal.CPOAppID
		if len(supplied) != len(expected) ||
			subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) != 1 {
			writeError(ctx, &APIError{
				Status:  http.StatusForbidden,
				Code:    "cpo_app_id_mismatch",
				Message: "The CPO app ID does not match the authenticated CPO.",
			})
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

func CurrentUserID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return principal.UserID, true
}

func CurrentCPOID(ctx *gin.Context) (uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok || principal.CPOID == nil {
		return uuid.Nil, false
	}
	return *principal.CPOID, true
}

func CurrentCPOAppID(ctx *gin.Context) (string, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok || principal.CPOAppID == nil {
		return "", false
	}
	return *principal.CPOAppID, true
}

func writeError(ctx *gin.Context, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = &APIError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "The request could not be completed.",
		}
	}
	ctx.JSON(apiErr.Status, gin.H{
		"error": gin.H{
			"code":    apiErr.Code,
			"message": apiErr.Message,
		},
	})
}
