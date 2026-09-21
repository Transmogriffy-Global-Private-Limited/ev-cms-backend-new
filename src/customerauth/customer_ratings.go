package customerauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const customerRatingReasonMaxRunes = 1000

// CustomerSessionRatingRequest is the complete replaceable representation of
// one customer's feedback for one completed charging session.
type CustomerSessionRatingRequest struct {
	OverallRating int     `json:"overall_rating"`
	StationRating *int    `json:"station_rating,omitempty"`
	ChargerRating *int    `json:"charger_rating,omitempty"`
	Reason        *string `json:"reason,omitempty"`
}

// CustomerSessionRatingView intentionally exposes only the customer-owned
// resource fields, never its tenant, customer, charger, or hub identity.
type CustomerSessionRatingView struct {
	ID            uuid.UUID `json:"id"`
	SessionID     uuid.UUID `json:"session_id"`
	OverallRating int       `json:"overall_rating"`
	StationRating *int      `json:"station_rating,omitempty"`
	ChargerRating *int      `json:"charger_rating,omitempty"`
	Reason        *string   `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (handler *Handler) getChargingSessionRating(ctx *gin.Context) {
	principal, sessionID, ok := customerRatingPrincipalAndSessionID(ctx)
	if !ok {
		return
	}
	rating, err := handler.service.GetCustomerSessionRating(ctx.Request.Context(), principal, sessionID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, rating)
}

func (handler *Handler) putChargingSessionRating(ctx *gin.Context) {
	principal, sessionID, ok := customerRatingPrincipalAndSessionID(ctx)
	if !ok {
		return
	}
	var request CustomerSessionRatingRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	rating, created, err := handler.service.PutCustomerSessionRating(ctx.Request.Context(), principal, sessionID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if created {
		ctx.JSON(http.StatusCreated, rating)
		return
	}
	ctx.JSON(http.StatusOK, rating)
}

func customerRatingPrincipalAndSessionID(ctx *gin.Context) (Principal, uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return Principal{}, uuid.Nil, false
	}
	sessionID, err := uuid.Parse(ctx.Param("session_id"))
	if err != nil || sessionID == uuid.Nil {
		writeError(ctx, &APIError{Status: http.StatusBadRequest, Code: "invalid_session_id", Message: "The charging session ID is invalid."})
		return Principal{}, uuid.Nil, false
	}
	return principal, sessionID, true
}

func (service *Service) GetCustomerSessionRating(ctx context.Context, principal Principal, sessionID uuid.UUID) (CustomerSessionRatingView, error) {
	if sessionID == uuid.Nil {
		return CustomerSessionRatingView{}, invalidCustomerRatingSessionID()
	}
	if _, err := service.customerOwnedRatingSession(ctx, service.database, principal, sessionID, false); err != nil {
		return CustomerSessionRatingView{}, err
	}
	var rating models.CustomerRating
	if err := service.database.WithContext(ctx).First(&rating, "cpo_id = ? AND customer_id = ? AND session_id = ?", principal.CPOID, principal.CustomerID, sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CustomerSessionRatingView{}, &APIError{Status: http.StatusNotFound, Code: "rating_not_found", Message: "No rating has been submitted for this charging session."}
		}
		return CustomerSessionRatingView{}, fmt.Errorf("load customer session rating: %w", err)
	}
	return customerSessionRatingView(rating), nil
}

func (service *Service) PutCustomerSessionRating(ctx context.Context, principal Principal, sessionID uuid.UUID, request CustomerSessionRatingRequest) (CustomerSessionRatingView, bool, error) {
	if sessionID == uuid.Nil {
		return CustomerSessionRatingView{}, false, invalidCustomerRatingSessionID()
	}
	request, err := normalizeCustomerSessionRatingRequest(request)
	if err != nil {
		return CustomerSessionRatingView{}, false, err
	}

	var rating models.CustomerRating
	created := false
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, err := service.customerOwnedRatingSession(ctx, tx, principal, sessionID, true)
		if err != nil {
			return err
		}
		if session.Status != constants.SessionStatusCompleted {
			return &APIError{Status: http.StatusConflict, Code: "session_not_rateable", Message: "Only a completed charging session can be rated."}
		}

		var charger models.Charger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&charger, "id = ? AND cpo_id = ?", session.ChargerID, principal.CPOID).Error; err != nil {
			return fmt.Errorf("load charging-session charger for rating: %w", err)
		}
		now := service.now().UTC()
		candidate := models.CustomerRating{
			ID: uuid.New(), CPOID: principal.CPOID, CustomerID: principal.CustomerID,
			ChargerID: charger.ID, HubID: charger.HubID, SessionID: &session.ID,
			OverallRating: request.OverallRating, StationRating: request.StationRating,
			ChargerRating: request.ChargerRating, Reason: request.Reason,
			CreatedAt: now, UpdatedAt: now,
		}
		insert := tx.Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "cpo_id"}, {Name: "session_id"}, {Name: "customer_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "session_id IS NOT NULL"}}},
			DoNothing:   true,
		}).Create(&candidate)
		if insert.Error != nil {
			return fmt.Errorf("create customer session rating: %w", insert.Error)
		}
		if insert.RowsAffected == 1 {
			rating, created = candidate, true
			return nil
		}

		updates := map[string]any{
			"charger_id":     charger.ID,
			"hub_id":         charger.HubID,
			"overall_rating": request.OverallRating,
			"station_rating": request.StationRating,
			"charger_rating": request.ChargerRating,
			"reason":         request.Reason,
			"updated_at":     now,
		}
		if err := tx.Model(&models.CustomerRating{}).
			Where("cpo_id = ? AND customer_id = ? AND session_id = ?", principal.CPOID, principal.CustomerID, session.ID).
			Where("charger_id IS DISTINCT FROM ? OR hub_id IS DISTINCT FROM ? OR overall_rating IS DISTINCT FROM ? OR station_rating IS DISTINCT FROM ? OR charger_rating IS DISTINCT FROM ? OR reason IS DISTINCT FROM ?", charger.ID, charger.HubID, request.OverallRating, request.StationRating, request.ChargerRating, request.Reason).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update customer session rating: %w", err)
		}
		if err := tx.First(&rating, "cpo_id = ? AND customer_id = ? AND session_id = ?", principal.CPOID, principal.CustomerID, session.ID).Error; err != nil {
			return fmt.Errorf("reload customer session rating: %w", err)
		}
		return nil
	})
	if err != nil {
		return CustomerSessionRatingView{}, false, err
	}
	return customerSessionRatingView(rating), created, nil
}

func (service *Service) customerOwnedRatingSession(ctx context.Context, database *gorm.DB, principal Principal, sessionID uuid.UUID, lock bool) (models.ChargingSession, error) {
	query := database.WithContext(ctx)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var session models.ChargingSession
	if err := query.First(&session, "id = ? AND cpo_id = ? AND customer_id = ?", sessionID, principal.CPOID, principal.CustomerID).Error; err != nil {
		return models.ChargingSession{}, customerNetworkNotFound(err, "charging session")
	}
	return session, nil
}

func normalizeCustomerSessionRatingRequest(request CustomerSessionRatingRequest) (CustomerSessionRatingRequest, error) {
	if request.OverallRating < 1 || request.OverallRating > 5 {
		return CustomerSessionRatingRequest{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_overall_rating", Message: "overall_rating must be between 1 and 5."}
	}
	if request.StationRating != nil && (*request.StationRating < 1 || *request.StationRating > 5) {
		return CustomerSessionRatingRequest{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_station_rating", Message: "station_rating must be between 1 and 5."}
	}
	if request.ChargerRating != nil && (*request.ChargerRating < 1 || *request.ChargerRating > 5) {
		return CustomerSessionRatingRequest{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_charger_rating", Message: "charger_rating must be between 1 and 5."}
	}
	if request.Reason != nil {
		if strings.TrimSpace(*request.Reason) == "" {
			request.Reason = nil
		} else if utf8.RuneCountInString(*request.Reason) > customerRatingReasonMaxRunes {
			return CustomerSessionRatingRequest{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_reason", Message: "reason must be at most 1000 characters."}
		}
	}
	return request, nil
}

func invalidCustomerRatingSessionID() *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "invalid_session_id", Message: "The charging session ID is invalid."}
}

func customerSessionRatingView(rating models.CustomerRating) CustomerSessionRatingView {
	view := CustomerSessionRatingView{
		ID: rating.ID, OverallRating: rating.OverallRating, StationRating: rating.StationRating,
		ChargerRating: rating.ChargerRating, Reason: rating.Reason, CreatedAt: rating.CreatedAt, UpdatedAt: rating.UpdatedAt,
	}
	if rating.SessionID != nil {
		view.SessionID = *rating.SessionID
	}
	return view
}
