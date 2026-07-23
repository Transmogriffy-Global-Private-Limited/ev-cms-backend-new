package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxAuthRequestBytes int64 = 32 * 1024

type Handler struct {
	service *Service
}

func RegisterRoutes(group *gin.RouterGroup, service *Service) {
	handler := &Handler{service: service}
	group.Use(noStore)

	group.POST("/login", handler.login)
	group.POST("/2fa/verify", handler.verify2FA)
	group.POST("/2fa/resend", handler.resend2FA)
	group.POST("/refresh", handler.refresh)
	group.POST("/password/forgot", handler.forgotPassword)
	group.POST("/password/reset", handler.resetPassword)

	protected := group.Group("")
	protected.Use(service.Authenticate())
	protected.GET("/me", handler.me)
	protected.POST("/logout", handler.logout)
	protected.POST("/logout-all", handler.logoutAll)
	protected.GET("/sessions", handler.sessions)
	protected.DELETE("/sessions/:session_id", handler.revokeSession)
	protected.POST("/password/change", handler.changePassword)
}

func (handler *Handler) login(ctx *gin.Context) {
	var request LoginRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Login(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) verify2FA(ctx *gin.Context) {
	var request ChallengeRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.VerifyLoginChallenge(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) resend2FA(ctx *gin.Context) {
	var request ResendRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.ResendLoginChallenge(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) refresh(ctx *gin.Context) {
	var request RefreshRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Refresh(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) forgotPassword(ctx *gin.Context) {
	var request ForgotPasswordRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	if err := handler.service.ForgotPassword(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{
		"message": "If the account is eligible, a password reset code will be sent.",
	})
}

func (handler *Handler) resetPassword(ctx *gin.Context) {
	var request ResetPasswordRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	if err := handler.service.ResetPassword(
		ctx.Request.Context(),
		request,
		requestMetadata(ctx),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Password reset. Sign in again.",
	})
}

func (handler *Handler) me(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	ctx.JSON(http.StatusOK, handler.service.Me(principal))
}

func (handler *Handler) logout(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	if err := handler.service.Logout(ctx.Request.Context(), principal); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) logoutAll(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	if err := handler.service.LogoutAll(ctx.Request.Context(), principal); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) sessions(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	sessions, err := handler.service.ListSessions(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (handler *Handler) revokeSession(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	sessionID, err := uuid.Parse(ctx.Param("session_id"))
	if err != nil {
		writeError(ctx, invalidRequest(errors.New("invalid session ID")))
		return
	}
	if err := handler.service.RevokeSession(
		ctx.Request.Context(),
		principal,
		sessionID,
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) changePassword(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request ChangePasswordRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	if err := handler.service.ChangePassword(
		ctx.Request.Context(),
		principal,
		request,
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Password changed. All sessions were revoked; sign in again.",
	})
}

func decodeJSON(ctx *gin.Context, destination any) error {
	ctx.Request.Body = http.MaxBytesReader(
		ctx.Writer,
		ctx.Request.Body,
		maxAuthRequestBytes,
	)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func invalidRequest(err error) *APIError {
	message := "The request body is invalid."
	if strings.Contains(err.Error(), "http: request body too large") {
		message = "The request body is too large."
	}
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "invalid_request",
		Message: message,
	}
}

func requestMetadata(ctx *gin.Context) RequestMetadata {
	address := strings.TrimSpace(ctx.ClientIP())
	var addressPointer *string
	if address != "" {
		addressPointer = &address
	}
	return RequestMetadata{
		IPAddress: addressPointer,
		UserAgent: ctx.Request.UserAgent(),
	}
}

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
