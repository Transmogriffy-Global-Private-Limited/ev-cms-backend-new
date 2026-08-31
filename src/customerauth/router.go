package customerauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxRequestBytes int64 = 32 * 1024

type Handler struct {
	service  *Service
	charging chargingController
}

type chargingController interface {
	StartCharging(context.Context, Principal, ChargingStartRequest, string) (ChargingStartResponse, error)
	StopCharging(context.Context, Principal, uuid.UUID, ChargingStopRequest, string) error
}

// requestCorrelationID returns the canonical CMS request identity for a
// downstream HAL mutation. Client headers are tracing input, never the CMS
// protocol correlation authority.
func requestCorrelationID(ctx *gin.Context) string {
	if requestID, ok := cmsmiddleware.RequestID(ctx); ok && strings.TrimSpace(requestID) != "" {
		return requestID
	}
	return uuid.NewString()
}

func RegisterRoutes(group *gin.RouterGroup, service *Service) {
	handler := &Handler{service: service, charging: service}
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
	protected.GET("/chargers/locations", handler.listChargerLocations)
	protected.GET("/chargers/:charger_id/image", handler.chargerImage)
	protected.GET("/chargers/:charger_id", handler.getCharger)
	protected.GET("/chargers/:charger_id/price", handler.getChargerPrice)
	protected.GET("/wallet", handler.getWallet)
	protected.GET("/wallet/transactions", handler.listWalletTransactions)
	protected.POST("/wallet/recharge/orders", handler.createRechargeOrder)
	protected.POST("/wallet/recharge/verify", handler.verifyRecharge)
	protected.POST("/charging-sessions", handler.startCharging)
	protected.GET("/charging-sessions", handler.listChargingSessions)
	protected.GET("/charging-start-intents/:start_intent_id", handler.getChargingStartIntent)
	protected.GET("/charging-sessions/:session_id", handler.getChargingSession)
	protected.POST("/charging-sessions/:session_id/stop", handler.stopCharging)
	protected.GET("/operations/events", handler.operationalEvents)
	protected.GET("/operations/realtime/stream", handler.operationalStream)
	protected.GET("/operations/live-sessions", handler.liveChargingSessionsStream)
	protected.GET("/operations/live-sessions/snapshot", handler.liveChargingSessionsSnapshot)
	protected.GET("/operations/live-sessions/realtime/stream", handler.liveChargingSessionsStream)
	protected.GET("/operations/charger-availability", handler.chargerAvailabilityStream)
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

func (handler *Handler) startCharging(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request ChargingStartRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	response, err := handler.charging.StartCharging(ctx.Request.Context(), principal, request, requestCorrelationID(ctx))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, response)
}

func (handler *Handler) getChargingStartIntent(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	id, err := uuid.Parse(ctx.Param("start_intent_id"))
	if err != nil {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_start_intent_id", "The charging start intent ID is invalid."})
		return
	}
	response, err := handler.service.GetChargingStartIntent(ctx.Request.Context(), principal, id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) listChargingSessions(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := chargingSessionHistoryQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerChargingSessions(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getChargingSession(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	id, err := uuid.Parse(ctx.Param("session_id"))
	if err != nil {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_session_id", "The charging session ID is invalid."})
		return
	}
	response, err := handler.service.GetChargingSessionWithFinancialProjection(ctx.Request.Context(), principal, id)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) operationalEvents(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListOperationalEvents(ctx.Request.Context(), principal, after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (handler *Handler) operationalStream(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	parts := strings.Fields(strings.TrimSpace(ctx.GetHeader("Authorization")))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeError(ctx, errUnauthorized)
		return
	}
	after, limit, ok := parseOperationalEventQuery(ctx)
	if !ok {
		return
	}
	page, err := handler.service.ListOperationalEvents(ctx.Request.Context(), principal, after, limit)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{})
	cursor := page.NextCursor
	if err := operationalrealtime.WriteSSE(ctx.Writer, page.Events); err != nil {
		return
	}
	ctx.Writer.Flush()
	poll, heartbeat, batchSize := handler.service.OperationalStreamTiming()
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			page, err := handler.service.ListOperationalEvents(ctx.Request.Context(), principal, cursor, batchSize)
			if err != nil || len(page.Events) == 0 {
				continue
			}
			if err := operationalrealtime.WriteSSE(ctx.Writer, page.Events); err != nil {
				return
			}
			cursor = page.NextCursor
			ctx.Writer.Flush()
		case <-heartbeatTicker.C:
			refreshed, err := handler.service.ValidateAccess(ctx.Request.Context(), parts[1])
			if err != nil || refreshed.CPOID != principal.CPOID || refreshed.CustomerID != principal.CustomerID {
				return
			}
			if _, err := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

// liveChargingSessionsSnapshot is the REST recovery/read surface for the same
// full projection emitted by the live-session SSE stream.
func (handler *Handler) liveChargingSessionsSnapshot(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	snapshot, err := handler.service.ListCustomerLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, snapshot)
}

// liveChargingSessionsStream delivers complete replacement state, never event
// envelopes or resource-ID invalidations. The durable event table is only the
// ordered committed wake-up source.
func (handler *Handler) liveChargingSessionsStream(ctx *gin.Context) {
	principal, token, ok := customerStreamPrincipal(ctx)
	if !ok {
		return
	}
	if handler.service.operationalEvents == nil {
		writeError(ctx, &APIError{http.StatusServiceUnavailable, "realtime_unavailable", "Realtime charging updates are temporarily unavailable."})
		return
	}

	// Watermark first: a state change after this point may cause a redundant
	// refresh, but cannot be skipped between snapshot and event consumption.
	cursor, err := handler.service.operationalEvents.LatestCustomerLiveProjectionEventID(ctx.Request.Context(), principal.CPOID, principal.CustomerID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	snapshot, err := handler.service.ListCustomerLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal)
	if err != nil {
		writeError(ctx, err)
		return
	}
	lastFingerprint, err := customerLiveSessionFingerprint(snapshot)
	if err != nil {
		writeError(ctx, err)
		return
	}
	prepareSSE(ctx)
	if err := writeProjectionSSE(ctx.Writer, "snapshot", cursor, snapshot); err != nil {
		return
	}
	ctx.Writer.Flush()

	poll, heartbeat, batchSize := handler.service.OperationalStreamTiming()
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			chargerIDs, connectorIDs := customerLiveProjectionResourceIDs(snapshot)
			page, err := handler.service.operationalEvents.ListCustomerLiveProjectionEvents(ctx.Request.Context(), principal.CPOID, principal.CustomerID, cursor, batchSize, chargerIDs, connectorIDs)
			if err != nil || len(page.Events) == 0 {
				continue
			}
			cursor = page.NextCursor
			snapshot, lastFingerprint, err = handler.refreshCustomerLiveSessions(ctx, principal, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
		case <-heartbeatTicker.C:
			if !handler.customerStreamStillAuthorized(ctx.Request.Context(), token, principal, ctx.GetHeader(CPOAppIDHeader)) {
				return
			}
			var err error
			snapshot, lastFingerprint, err = handler.refreshCustomerLiveSessions(ctx, principal, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func (handler *Handler) refreshCustomerLiveSessions(ctx *gin.Context, principal Principal, cursor int64, previous CustomerLiveChargingSessionListResponse, previousFingerprint []byte) (CustomerLiveChargingSessionListResponse, []byte, error) {
	next, err := handler.service.ListCustomerLiveChargingSessionsWithFinancialProjection(ctx.Request.Context(), principal)
	if err != nil {
		return previous, previousFingerprint, err
	}
	fingerprint, err := customerLiveSessionFingerprint(next)
	if err != nil {
		return previous, previousFingerprint, err
	}
	if bytes.Equal(previousFingerprint, fingerprint) {
		return next, fingerprint, nil
	}
	if err := writeProjectionSSE(ctx.Writer, "live_sessions", cursor, next); err != nil {
		return previous, previousFingerprint, err
	}
	ctx.Writer.Flush()
	return next, fingerprint, nil
}

// chargerAvailabilityStream emits the exact customer-safe charger projection
// used by GET /chargers/{charger_id}. It accepts the public charger ID, never
// a CMS UUID, and reprojects on authorized heartbeats for freshness aging.
func (handler *Handler) chargerAvailabilityStream(ctx *gin.Context) {
	principal, token, ok := customerStreamPrincipal(ctx)
	if !ok {
		return
	}
	publicChargerID := strings.TrimSpace(ctx.Query("charger_id"))
	if publicChargerID == "" {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is required."})
		return
	}
	if handler.service.operationalEvents == nil {
		writeError(ctx, &APIError{http.StatusServiceUnavailable, "realtime_unavailable", "Realtime charger updates are temporarily unavailable."})
		return
	}
	cursor, err := handler.service.operationalEvents.LatestCPOChargerAvailabilityEventID(ctx.Request.Context(), principal.CPOID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	snapshot, err := handler.service.GetCustomerCharger(ctx.Request.Context(), principal, publicChargerID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	lastFingerprint, err := json.Marshal(snapshot)
	if err != nil {
		writeError(ctx, err)
		return
	}
	prepareSSE(ctx)
	if err := writeProjectionSSE(ctx.Writer, "snapshot", cursor, snapshot); err != nil {
		return
	}
	ctx.Writer.Flush()

	poll, heartbeat, batchSize := handler.service.OperationalStreamTiming()
	pollTicker, heartbeatTicker := time.NewTicker(poll), time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-pollTicker.C:
			page, err := handler.service.operationalEvents.ListCPOChargerAvailabilityEvents(ctx.Request.Context(), principal.CPOID, snapshot.ID, customerChargerConnectorIDs(snapshot), cursor, batchSize)
			if err != nil || len(page.Events) == 0 {
				continue
			}
			cursor = page.NextCursor
			snapshot, lastFingerprint, err = handler.refreshCustomerCharger(ctx, principal, publicChargerID, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
		case <-heartbeatTicker.C:
			if !handler.customerStreamStillAuthorized(ctx.Request.Context(), token, principal, ctx.GetHeader(CPOAppIDHeader)) {
				return
			}
			var err error
			snapshot, lastFingerprint, err = handler.refreshCustomerCharger(ctx, principal, publicChargerID, cursor, snapshot, lastFingerprint)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(ctx.Writer, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339)); err != nil {
				return
			}
			ctx.Writer.Flush()
		}
	}
}

func (handler *Handler) refreshCustomerCharger(ctx *gin.Context, principal Principal, publicChargerID string, cursor int64, previous CustomerChargerView, previousFingerprint []byte) (CustomerChargerView, []byte, error) {
	next, err := handler.service.GetCustomerCharger(ctx.Request.Context(), principal, publicChargerID)
	if err != nil {
		return previous, previousFingerprint, err
	}
	fingerprint, err := json.Marshal(next)
	if err != nil {
		return previous, previousFingerprint, err
	}
	if bytes.Equal(previousFingerprint, fingerprint) {
		return next, fingerprint, nil
	}
	if err := writeProjectionSSE(ctx.Writer, "charger_availability", cursor, next); err != nil {
		return previous, previousFingerprint, err
	}
	ctx.Writer.Flush()
	return next, fingerprint, nil
}

func customerStreamPrincipal(ctx *gin.Context) (Principal, string, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return Principal{}, "", false
	}
	parts := strings.Fields(strings.TrimSpace(ctx.GetHeader("Authorization")))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeError(ctx, errUnauthorized)
		return Principal{}, "", false
	}
	return principal, parts[1], true
}

func (handler *Handler) customerStreamStillAuthorized(ctx context.Context, token string, principal Principal, appID string) bool {
	refreshed, err := handler.service.ValidateAccess(ctx, token)
	return err == nil && refreshed.CPOID == principal.CPOID && refreshed.CustomerID == principal.CustomerID && refreshed.CPOAppID == principal.CPOAppID && refreshed.CPOAppID == appID
}

func prepareSSE(ctx *gin.Context) {
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{})
}

func writeProjectionSSE(writer io.Writer, eventType string, eventID int64, projection any) error {
	payload, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", eventID, eventType, payload)
	return err
}

func customerLiveSessionFingerprint(snapshot CustomerLiveChargingSessionListResponse) ([]byte, error) {
	snapshot.AsOf = time.Time{}
	return json.Marshal(snapshot)
}

func customerChargerConnectorIDs(snapshot CustomerChargerView) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(snapshot.Connectors))
	for _, connector := range snapshot.Connectors {
		ids = append(ids, connector.ID)
	}
	return ids
}

func parseOperationalEventQuery(ctx *gin.Context) (int64, int, bool) {
	after := ctx.Query("after_id")
	if strings.TrimSpace(after) == "" {
		after = ctx.GetHeader("Last-Event-ID")
	}
	parsedAfter, limit, err := operationalrealtime.ParseCursor(after, ctx.Query("limit"))
	if err != nil {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_event_query", "The event cursor or limit is invalid."})
		return 0, 0, false
	}
	return parsedAfter, limit, true
}

func (handler *Handler) stopCharging(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	id, err := uuid.Parse(ctx.Param("session_id"))
	if err != nil {
		writeError(ctx, &APIError{http.StatusBadRequest, "invalid_session_id", "The charging session ID is invalid."})
		return
	}
	var request ChargingStopRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	if err := handler.charging.StopCharging(ctx.Request.Context(), principal, id, request, requestCorrelationID(ctx)); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusAccepted)
}

func (handler *Handler) chargerImage(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}

	image, err := handler.service.OpenCustomerChargerImage(
		ctx.Request.Context(), principal, ctx.Param("charger_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	defer image.File.Close()

	ctx.Header("Content-Type", image.ContentType)
	ctx.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(ctx.Writer, ctx.Request, image.Name, image.ModifiedAt, image.File)
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

func (handler *Handler) listChargerLocations(ctx *gin.Context) {
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
	response, err := handler.service.ListCustomerChargerLocations(ctx.Request.Context(), principal, query)
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

func chargingSessionHistoryQuery(ctx *gin.Context) (ChargingSessionHistoryQuery, error) {
	query := ChargingSessionHistoryQuery{}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return ChargingSessionHistoryQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	var err error
	query.Before, err = parseCustomerCursorTime(ctx.Query("before"))
	if err != nil {
		return ChargingSessionHistoryQuery{}, err
	}
	query.BeforeID, err = parseCustomerCursorID(ctx.Query("before_id"))
	if err != nil {
		return ChargingSessionHistoryQuery{}, err
	}
	return query, validateChargingSessionHistoryQuery(&query)
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
