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

	auth := group.Group("/auth")
	auth.POST("/signup", handler.start)
	auth.POST("/signup/verify", handler.verify)
	auth.POST("/signup/resend", handler.resend)
	auth.POST("/login", handler.login)
	auth.POST("/login/verify", handler.verifyLogin)
	auth.POST("/login/resend", handler.resendLogin)
	auth.POST("/refresh", handler.refresh)
	auth.POST("/password/forgot", handler.forgotPassword)
	auth.POST("/password/reset/resend", handler.resendPasswordReset)
	auth.POST("/password/reset", handler.resetPassword)

	protected := group.Group("")
	protected.Use(service.Authenticate(), RequireAppID())
	protected.GET("/me", handler.me)
	protected.PATCH("/profile", handler.updateProfile)
	protected.GET("/hubs", handler.listHubs)
	protected.GET("/hubs/:hub_id", handler.getHub)
	protected.GET("/hubs/:hub_id/price", handler.getHubPrice)
	protected.GET("/chargers", handler.listChargers)
	protected.GET("/chargers/:charger_id", handler.getCharger)
	protected.GET("/chargers/:charger_id/price", handler.getChargerPrice)
	protected.GET("/wallet", handler.getWallet)
	protected.GET("/wallet/transactions", handler.listWalletTransactions)
	protected.POST("/wallet/recharge/orders", handler.createRechargeOrder)
	protected.POST("/wallet/recharge/verify", handler.verifyRecharge)
	protected.GET("/favorites", handler.listFavorites)
	protected.PUT("/favorite-hubs/:hub_id", handler.addFavoriteHub)
	protected.DELETE("/favorite-hubs/:hub_id", handler.removeFavoriteHub)
	protected.PUT("/favorite-chargers/:charger_id", handler.addFavoriteCharger)
	protected.DELETE("/favorite-chargers/:charger_id", handler.removeFavoriteCharger)

	protectedAuth := auth.Group("")
	protectedAuth.Use(service.Authenticate(), RequireAppID())
	protectedAuth.GET("/sessions", handler.sessions)
	protectedAuth.DELETE("/sessions/:session_id", handler.revokeSession)
	protectedAuth.POST("/logout", handler.logout)
	protectedAuth.POST("/logout-all", handler.logoutAll)
	protectedAuth.POST("/password/change", handler.changePassword)
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

func (handler *Handler) listChargers(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerChargerListQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerChargers(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getWallet(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	response, err := handler.service.GetCustomerWallet(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) listWalletTransactions(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerWalletTransactionQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerWalletTransactions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) createRechargeOrder(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request CustomerRechargeOrderRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.CreateWalletRechargeOrder(
		ctx.Request.Context(), principal, ctx.GetHeader(IdempotencyKeyHeader), request,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}

func (handler *Handler) verifyRecharge(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request CustomerRechargeVerifyRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.service.VerifyWalletRecharge(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getHubPrice(ctx *gin.Context) {
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
	response, err := handler.service.GetCustomerHubPrice(ctx.Request.Context(), principal, hubID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getChargerPrice(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	response, err := handler.service.GetCustomerChargerPrice(ctx.Request.Context(), principal, ctx.Param("charger_id"))
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

func customerChargerListQuery(ctx *gin.Context) (CustomerChargerListQuery, error) {
	query := CustomerChargerListQuery{
		Search:        ctx.Query("q"),
		ConnectorType: ctx.Query("connector_type"),
	}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	var err error
	query.Before, err = parseCustomerCursorTime(ctx.Query("before"))
	if err != nil {
		return CustomerChargerListQuery{}, err
	}
	query.BeforeID, err = parseCustomerCursorID(ctx.Query("before_id"))
	if err != nil {
		return CustomerChargerListQuery{}, err
	}
	if raw := strings.TrimSpace(ctx.Query("min_power_kw")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_min_power_kw", "Minimum power must be a number."}
		}
		query.MinPowerKW = &value
	}
	if raw := strings.TrimSpace(ctx.Query("max_power_kw")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_max_power_kw", "Maximum power must be a number."}
		}
		query.MaxPowerKW = &value
	}
	if raw := strings.TrimSpace(ctx.Query("lat")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_latitude", "Latitude must be a number."}
		}
		query.Latitude = &value
	}
	if raw := strings.TrimSpace(ctx.Query("lng")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_longitude", "Longitude must be a number."}
		}
		query.Longitude = &value
	}
	if raw := strings.TrimSpace(ctx.Query("radius_km")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_radius_km", "Radius must be a number."}
		}
		query.RadiusKM = &value
	}
	if raw := strings.TrimSpace(ctx.Query("open_24_hours")); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return CustomerChargerListQuery{}, &APIError{http.StatusBadRequest, "invalid_open_24_hours", "open_24_hours must be true or false."}
		}
		query.Open24Hours = &value
	}
	return query, validateCustomerChargerListQuery(&query)
}

func customerWalletTransactionQuery(ctx *gin.Context) (CustomerWalletTransactionQuery, error) {
	query := CustomerWalletTransactionQuery{}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return CustomerWalletTransactionQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	var err error
	query.Before, err = parseCustomerCursorTime(ctx.Query("before"))
	if err != nil {
		return CustomerWalletTransactionQuery{}, err
	}
	query.BeforeID, err = parseCustomerCursorID(ctx.Query("before_id"))
	if err != nil {
		return CustomerWalletTransactionQuery{}, err
	}
	return query, validateCustomerWalletTransactionQuery(&query)
}

func parseCustomerCursorTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, &APIError{http.StatusBadRequest, "invalid_cursor", "The before cursor timestamp is invalid."}
	}
	return &value, nil
}

func parseCustomerCursorID(raw string) (*uuid.UUID, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := uuid.Parse(raw)
	if err != nil {
		return nil, &APIError{http.StatusBadRequest, "invalid_cursor", "The before cursor ID is invalid."}
	}
	return &value, nil
}

func (handler *Handler) listFavorites(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerFavoritesQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerFavorites(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) addFavoriteHub(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	hubID, err := uuid.Parse(ctx.Param("hub_id"))
	if err != nil {
		writeError(ctx, invalidFavoriteID("hub"))
		return
	}
	if err := handler.service.AddFavoriteHub(ctx.Request.Context(), principal, hubID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) removeFavoriteHub(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	hubID, err := uuid.Parse(ctx.Param("hub_id"))
	if err != nil {
		writeError(ctx, invalidFavoriteID("hub"))
		return
	}
	if err := handler.service.RemoveFavoriteHub(ctx.Request.Context(), principal, hubID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) addFavoriteCharger(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	if err := handler.service.AddFavoriteCharger(ctx.Request.Context(), principal, ctx.Param("charger_id")); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (handler *Handler) removeFavoriteCharger(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	if err := handler.service.RemoveFavoriteCharger(ctx.Request.Context(), principal, ctx.Param("charger_id")); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func customerFavoritesQuery(ctx *gin.Context) (CustomerFavoritesQuery, error) {
	query := CustomerFavoritesQuery{}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return CustomerFavoritesQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	var err error
	query.HubBefore, query.HubBeforeID, err = favoriteCursor(ctx, "hub_before", "hub_before_id")
	if err != nil {
		return CustomerFavoritesQuery{}, err
	}
	query.ChargerBefore, query.ChargerBeforeID, err = favoriteCursor(ctx, "charger_before", "charger_before_id")
	if err != nil {
		return CustomerFavoritesQuery{}, err
	}
	return query, nil
}

func favoriteCursor(ctx *gin.Context, timestampKey, idKey string) (*time.Time, *uuid.UUID, error) {
	timestamp := strings.TrimSpace(ctx.Query(timestampKey))
	id := strings.TrimSpace(ctx.Query(idKey))
	if timestamp == "" && id == "" {
		return nil, nil, nil
	}
	if timestamp == "" || id == "" {
		return nil, nil, &APIError{http.StatusBadRequest, "invalid_cursor", "Favorite cursors require both timestamp and ID."}
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return nil, nil, &APIError{http.StatusBadRequest, "invalid_cursor", "The favorite cursor timestamp is invalid."}
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, nil, &APIError{http.StatusBadRequest, "invalid_cursor", "The favorite cursor ID is invalid."}
	}
	return &parsedTime, &parsedID, nil
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
