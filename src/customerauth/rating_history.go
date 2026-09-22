package customerauth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CustomerRatingHistoryQuery struct {
	Limit                  int
	MinOverall, MaxOverall *int
	HasReview              *bool
	ChargerID              *string
	HubID                  *uuid.UUID
	SortBy, SortOrder      string
	Before                 *time.Time
	BeforeID               *uuid.UUID
	CursorValue            *string
	CursorID               *uuid.UUID
}

type CustomerRatingHistoryItem struct {
	Rating  CustomerSessionRatingView  `json:"rating"`
	Session ChargingSessionHistoryView `json:"session"`
}

type CustomerRatingHistoryResponse struct {
	Ratings         []CustomerRatingHistoryItem `json:"ratings"`
	NextBefore      *time.Time                  `json:"next_before,omitempty"`
	NextBeforeID    *uuid.UUID                  `json:"next_before_id,omitempty"`
	NextCursorValue *string                     `json:"next_cursor_value,omitempty"`
	NextCursorID    *uuid.UUID                  `json:"next_cursor_id,omitempty"`
	HasMore         bool                        `json:"has_more"`
}

func (handler *Handler) listChargingSessionRatings(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerRatingHistoryQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListCustomerRatingHistory(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func customerRatingHistoryQuery(ctx *gin.Context) (CustomerRatingHistoryQuery, error) {
	query := CustomerRatingHistoryQuery{SortBy: ctx.Query("sort_by"), SortOrder: ctx.Query("sort_order")}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return query, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = value
	}
	for _, item := range []struct {
		key    string
		target **int
	}{{"min_overall_rating", &query.MinOverall}, {"max_overall_rating", &query.MaxOverall}} {
		if raw := strings.TrimSpace(ctx.Query(item.key)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				return query, &APIError{http.StatusBadRequest, "invalid_rating", item.key + " must be an integer."}
			}
			*item.target = &value
		}
	}
	if raw := strings.TrimSpace(ctx.Query("has_review")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return query, &APIError{http.StatusBadRequest, "invalid_has_review", "has_review must be true or false."}
		}
		query.HasReview = &value
	}
	if raw := strings.TrimSpace(ctx.Query("charger_id")); raw != "" {
		value := strings.ToLower(raw)
		if !customerChargerIDPattern.MatchString(value) {
			return query, &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is invalid."}
		}
		query.ChargerID = &value
	}
	if raw := strings.TrimSpace(ctx.Query("hub_id")); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			return query, &APIError{http.StatusBadRequest, "invalid_hub_id", "The hub ID is invalid."}
		}
		query.HubID = &value
	}
	var err error
	query.Before, err = parseCustomerCursorTime(ctx.Query("before"))
	if err != nil {
		return query, err
	}
	query.BeforeID, err = parseCustomerCursorID(ctx.Query("before_id"))
	if err != nil {
		return query, err
	}
	if raw := strings.TrimSpace(ctx.Query("cursor_value")); raw != "" {
		query.CursorValue = &raw
	}
	if raw := strings.TrimSpace(ctx.Query("cursor_id")); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			return query, &APIError{http.StatusBadRequest, "invalid_cursor", "cursor_id must be a non-zero UUID."}
		}
		query.CursorID = &value
	}
	return query, validateCustomerRatingHistoryQuery(&query)
}

func validateCustomerRatingHistoryQuery(query *CustomerRatingHistoryQuery) error {
	if query.Limit == 0 {
		query.Limit = customerNetworkDefaultLimit
	}
	if query.Limit < 1 || query.Limit > customerNetworkMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	if query.MinOverall != nil && (*query.MinOverall < 1 || *query.MinOverall > 5) {
		return &APIError{http.StatusBadRequest, "invalid_min_overall_rating", "min_overall_rating must be between 1 and 5."}
	}
	if query.MaxOverall != nil && (*query.MaxOverall < 1 || *query.MaxOverall > 5) {
		return &APIError{http.StatusBadRequest, "invalid_max_overall_rating", "max_overall_rating must be between 1 and 5."}
	}
	if query.MinOverall != nil && query.MaxOverall != nil && *query.MinOverall > *query.MaxOverall {
		return &APIError{http.StatusBadRequest, "invalid_overall_rating_range", "min_overall_rating must not exceed max_overall_rating."}
	}
	query.SortBy = strings.ToLower(strings.TrimSpace(query.SortBy))
	query.SortOrder = strings.ToLower(strings.TrimSpace(query.SortOrder))
	if query.SortBy == "" {
		query.SortBy = "created_at"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
	if query.SortBy != "created_at" && query.SortBy != "updated_at" && query.SortBy != "overall_rating" && query.SortBy != "session_start_time" {
		return &APIError{http.StatusBadRequest, "invalid_sort_by", "sort_by must be created_at, updated_at, overall_rating, or session_start_time."}
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		return &APIError{http.StatusBadRequest, "invalid_sort_order", "sort_order must be asc or desc."}
	}
	if (query.Before == nil) != (query.BeforeID == nil) || (query.CursorValue == nil) != (query.CursorID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Cursor values must be supplied as pairs."}
	}
	if query.Before != nil && query.SortBy != "created_at" {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "before cursors only apply to created_at sorting."}
	}
	if query.CursorValue != nil && query.SortBy == "created_at" {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "created_at sorting uses before and before_id."}
	}
	if query.CursorValue != nil {
		switch query.SortBy {
		case "overall_rating":
			value, err := strconv.Atoi(*query.CursorValue)
			if err != nil || value < 1 || value > 5 {
				return &APIError{http.StatusBadRequest, "invalid_cursor", "cursor_value must be an overall rating between 1 and 5."}
			}
		case "updated_at", "session_start_time":
			if _, err := time.Parse(time.RFC3339Nano, *query.CursorValue); err != nil {
				return &APIError{http.StatusBadRequest, "invalid_cursor", "cursor_value must be an RFC3339 timestamp."}
			}
		}
	}
	return nil
}

func (service *Service) ListCustomerRatingHistory(ctx context.Context, principal Principal, query CustomerRatingHistoryQuery) (CustomerRatingHistoryResponse, error) {
	if err := validateCustomerRatingHistoryQuery(&query); err != nil {
		return CustomerRatingHistoryResponse{}, err
	}
	db := service.database.WithContext(ctx).Model(&models.CustomerRating{}).
		Joins("JOIN charging_sessions ON charging_sessions.id = customer_ratings.session_id AND charging_sessions.cpo_id = customer_ratings.cpo_id AND charging_sessions.customer_id = customer_ratings.customer_id").
		Preload("Session", "cpo_id = ? AND customer_id = ?", principal.CPOID, principal.CustomerID).
		Preload("Session.Charger", "cpo_id = ?", principal.CPOID).Preload("Session.Charger.Hub", "cpo_id = ?", principal.CPOID).Preload("Session.Connector", "cpo_id = ?", principal.CPOID).
		Where("customer_ratings.cpo_id = ? AND customer_ratings.customer_id = ? AND customer_ratings.session_id IS NOT NULL", principal.CPOID, principal.CustomerID)
	if query.ChargerID != nil {
		db = db.Joins("JOIN chargers ON chargers.id = customer_ratings.charger_id AND chargers.cpo_id = customer_ratings.cpo_id").Where("chargers.charger_id = ?", *query.ChargerID)
	}
	if query.HubID != nil {
		db = db.Where("customer_ratings.hub_id = ?", *query.HubID)
	}
	if query.MinOverall != nil {
		db = db.Where("customer_ratings.overall_rating >= ?", *query.MinOverall)
	}
	if query.MaxOverall != nil {
		db = db.Where("customer_ratings.overall_rating <= ?", *query.MaxOverall)
	}
	if query.HasReview != nil {
		if *query.HasReview {
			db = db.Where("NULLIF(BTRIM(customer_ratings.reason), '') IS NOT NULL")
		} else {
			db = db.Where("NULLIF(BTRIM(customer_ratings.reason), '') IS NULL")
		}
	}
	column := "customer_ratings." + query.SortBy
	if query.SortBy == "session_start_time" {
		column = "charging_sessions.start_time"
	}
	if query.Before != nil {
		operator := "<"
		if query.SortOrder == "asc" {
			operator = ">"
		}
		db = db.Where("(customer_ratings.created_at, customer_ratings.id) "+operator+" (?, ?)", *query.Before, *query.BeforeID)
	}
	if query.CursorValue != nil {
		operator := "<"
		if query.SortOrder == "asc" {
			operator = ">"
		}
		var cursor any = *query.CursorValue
		if query.SortBy == "overall_rating" {
			value, _ := strconv.Atoi(*query.CursorValue) // validated above
			cursor = value
		} else {
			value, _ := time.Parse(time.RFC3339Nano, *query.CursorValue) // validated above
			cursor = value
		}
		db = db.Where("("+column+", customer_ratings.id) "+operator+" (?, ?)", cursor, *query.CursorID)
	}
	direction := strings.ToUpper(query.SortOrder)
	var rows []models.CustomerRating
	if err := db.Order(column + " " + direction + ", customer_ratings.id " + direction).Limit(query.Limit + 1).Find(&rows).Error; err != nil {
		return CustomerRatingHistoryResponse{}, fmt.Errorf("list customer rating history: %w", err)
	}
	hasMore := len(rows) > query.Limit
	if hasMore {
		rows = rows[:query.Limit]
	}
	response := CustomerRatingHistoryResponse{Ratings: make([]CustomerRatingHistoryItem, 0, len(rows)), HasMore: hasMore}
	for _, row := range rows {
		if row.Session == nil {
			continue
		}
		response.Ratings = append(response.Ratings, CustomerRatingHistoryItem{Rating: customerSessionRatingView(row), Session: customerChargingSessionHistoryView(*row.Session)})
	}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		if query.SortBy == "created_at" {
			response.NextBefore, response.NextBeforeID = &last.CreatedAt, &last.ID
		} else {
			var value string
			if query.SortBy == "overall_rating" {
				value = strconv.Itoa(last.OverallRating)
			} else if query.SortBy == "updated_at" {
				value = last.UpdatedAt.Format(time.RFC3339Nano)
			} else {
				value = last.Session.StartTime.Format(time.RFC3339Nano)
			}
			response.NextCursorValue, response.NextCursorID = &value, &last.ID
		}
	}
	return response, nil
}
