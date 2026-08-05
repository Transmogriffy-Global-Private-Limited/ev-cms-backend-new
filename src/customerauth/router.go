package customerauth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxRequestBytes int64 = 32 * 1024

type Handler struct {
	service *Service
}

func RegisterRoutes(group *gin.RouterGroup, service *Service) {
	handler := &Handler{service: service}
	group.Use(noStore)
	group.POST("/signup", handler.start)
	group.POST("/signup/verify", handler.verify)
	group.POST("/signup/resend", handler.resend)
	group.POST("/login", handler.login)
	group.POST("/login/verify", handler.verifyLogin)
	group.POST("/login/resend", handler.resendLogin)
	group.POST("/refresh", handler.refresh)
	group.POST("/password/forgot", handler.forgotPassword)
	group.POST("/password/reset/resend", handler.resendPasswordReset)
	group.POST("/password/reset", handler.resetPassword)

	protected := group.Group("")
	protected.Use(service.Authenticate(), RequireAppID())
	protected.GET("/me", handler.me)
	protected.PATCH("/profile", handler.updateProfile)
	protected.GET("/hubs", handler.listHubs)
	protected.GET("/hubs/:hub_id", handler.getHub)
	protected.GET("/chargers/:charger_id", handler.getCharger)
	protected.GET("/sessions", handler.sessions)
	protected.DELETE("/sessions/:session_id", handler.revokeSession)
	protected.POST("/logout", handler.logout)
	protected.POST("/logout-all", handler.logoutAll)
	protected.POST("/password/change", handler.changePassword)
}

func (handler *Handler) start(ctx *gin.Context) {
	var request SignupRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Start(
		ctx.Request.Context(), ctx.GetHeader("X-CPO-App-ID"), request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) verify(ctx *gin.Context) {
	var request ChallengeRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Verify(
		ctx.Request.Context(), ctx.GetHeader("X-CPO-App-ID"), request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}

func (handler *Handler) resend(ctx *gin.Context) {
	var request ResendRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Resend(
		ctx.Request.Context(), ctx.GetHeader("X-CPO-App-ID"), request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) login(ctx *gin.Context) {
	var request LoginRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.Login(
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) verifyLogin(ctx *gin.Context) {
	var request ChallengeRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.VerifyLogin(
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) resendLogin(ctx *gin.Context) {
	var request ResendRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.ResendLogin(
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
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
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
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
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{
		"message": "If the customer account is eligible, a password reset code will be sent.",
	})
}

func (handler *Handler) resendPasswordReset(ctx *gin.Context) {
	var request ResendRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.ResendPasswordReset(
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) resetPassword(ctx *gin.Context) {
	var request ResetPasswordRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	if err := handler.service.ResetPassword(
		ctx.Request.Context(), ctx.GetHeader(CPOAppIDHeader),
		request, requestMetadata(ctx),
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "Password reset. Sign in again."})
}

func (handler *Handler) me(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	ctx.JSON(http.StatusOK, handler.service.Me(principal))
}

func (handler *Handler) updateProfile(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request UpdateProfileRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	profile, err := handler.service.UpdateProfile(
		ctx.Request.Context(), principal, request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, profile)
}

func (handler *Handler) listHubs(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerHubListQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerHubs(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getHub(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	hubID, err := uuid.Parse(ctx.Param("hub_id"))
	if err != nil {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_hub_id", "The hub ID is invalid."})
		return
	}
	response, err := handler.service.GetCustomerHub(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getCharger(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	response, err := handler.service.GetCustomerCharger(ctx.Request.Context(), principal, ctx.Param("charger_id"))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func customerHubListQuery(ctx *gin.Context) (CustomerHubListQuery, error) {
	query := CustomerHubListQuery{Search: ctx.Query("q")}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return CustomerHubListQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	if raw := strings.TrimSpace(ctx.Query("before")); raw != "" {
		before, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return CustomerHubListQuery{}, &APIError{http.StatusBadRequest, "invalid_cursor", "The before cursor timestamp is invalid."}
		}
		query.Before = &before
	}
	if raw := strings.TrimSpace(ctx.Query("before_id")); raw != "" {
		beforeID, err := uuid.Parse(raw)
		if err != nil {
			return CustomerHubListQuery{}, &APIError{http.StatusBadRequest, "invalid_cursor", "The before cursor ID is invalid."}
		}
		query.BeforeID = &beforeID
	}
	return query, nil
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
		writeError(ctx, &APIError{
			http.StatusBadRequest, "invalid_session_id", "The session ID is invalid.",
		})
		return
	}
	if err := handler.service.RevokeSession(
		ctx.Request.Context(), principal, sessionID,
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
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
		ctx.Request.Context(), principal, request,
	); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Password changed. All sessions were revoked; sign in again.",
	})
}

func decodeJSON(ctx *gin.Context, destination any) error {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxRequestBytes)
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
	return &APIError{http.StatusBadRequest, "invalid_request", message}
}

func requestMetadata(ctx *gin.Context) RequestMetadata {
	address := strings.TrimSpace(ctx.ClientIP())
	var pointer *string
	if address != "" {
		pointer = &address
	}
	return RequestMetadata{IPAddress: pointer, UserAgent: ctx.Request.UserAgent()}
}

func writeError(ctx *gin.Context, err error) {
	var apiError *APIError
	if errors.As(err, &apiError) {
		cmsmiddleware.LogHandledError(
			ctx, "customer_auth", apiError.Code, apiError.Status, err,
		)
		ctx.JSON(apiError.Status, gin.H{"error": gin.H{
			"code": apiError.Code, "message": apiError.Message,
		}})
		return
	}
	cmsmiddleware.LogHandledError(
		ctx, "customer_auth", "internal_error", http.StatusInternalServerError, err,
	)
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
		"code": "internal_error", "message": "The request could not be completed.",
	}})
}

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
