package customerauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	vehicleDefaultLimit = 50
	vehicleMaxLimit     = 100
)

type CreateVehicleRequest struct {
	VehicleNumber string  `json:"vehicle_number"`
	VehicleType   *string `json:"vehicle_type,omitempty"`
	VehicleMake   *string `json:"vehicle_make,omitempty"`
	VehicleModel  *string `json:"vehicle_model,omitempty"`
}

// UpdateVehicleRequest records field presence so PATCH can distinguish omitted
// values from explicit nulls, which clear optional vehicle metadata.
type UpdateVehicleRequest struct {
	VehicleNumber *string `json:"vehicle_number,omitempty"`
	VehicleType   *string `json:"vehicle_type,omitempty"`
	VehicleMake   *string `json:"vehicle_make,omitempty"`
	VehicleModel  *string `json:"vehicle_model,omitempty"`

	vehicleNumberSet bool
	vehicleTypeSet   bool
	vehicleMakeSet   bool
	vehicleModelSet  bool
}

func (request *UpdateVehicleRequest) UnmarshalJSON(data []byte) error {
	type wireRequest struct {
		VehicleNumber *string `json:"vehicle_number"`
		VehicleType   *string `json:"vehicle_type"`
		VehicleMake   *string `json:"vehicle_make"`
		VehicleModel  *string `json:"vehicle_model"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireRequest
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	request.VehicleNumber = wire.VehicleNumber
	request.VehicleType = wire.VehicleType
	request.VehicleMake = wire.VehicleMake
	request.VehicleModel = wire.VehicleModel
	_, request.vehicleNumberSet = fields["vehicle_number"]
	_, request.vehicleTypeSet = fields["vehicle_type"]
	_, request.vehicleMakeSet = fields["vehicle_make"]
	_, request.vehicleModelSet = fields["vehicle_model"]
	return nil
}

type CustomerVehicleView struct {
	ID            uuid.UUID  `json:"id"`
	VehicleNumber string     `json:"vehicle_number"`
	VehicleType   *string    `json:"vehicle_type,omitempty"`
	VehicleMake   *string    `json:"vehicle_make,omitempty"`
	VehicleModel  *string    `json:"vehicle_model,omitempty"`
	LastCharged   *time.Time `json:"last_charged,omitempty"`
	DateAdded     time.Time  `json:"date_added"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CustomerVehicleListQuery struct {
	Limit        int
	Before       *time.Time
	BeforeID     *uuid.UUID
	Search       string
	VehicleType  string
	VehicleMake  string
	VehicleModel string
}

type CustomerVehicleListResponse struct {
	Vehicles     []CustomerVehicleView `json:"vehicles"`
	HasMore      bool                  `json:"has_more"`
	NextBefore   *time.Time            `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID            `json:"next_before_id,omitempty"`
}

func (handler *Handler) createVehicle(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	var request CreateVehicleRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	vehicle, err := handler.service.CreateVehicle(ctx.Request.Context(), principal, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, vehicle)
}

func (handler *Handler) listVehicles(ctx *gin.Context) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return
	}
	query, err := customerVehicleListQuery(ctx)
	if err != nil {
		writeError(ctx, err)
		return
	}
	response, err := handler.service.ListVehicles(ctx.Request.Context(), principal, query)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (handler *Handler) getVehicle(ctx *gin.Context) {
	principal, vehicleID, ok := vehiclePrincipalAndID(ctx)
	if !ok {
		return
	}
	vehicle, err := handler.service.GetVehicle(ctx.Request.Context(), principal, vehicleID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, vehicle)
}

func (handler *Handler) updateVehicle(ctx *gin.Context) {
	principal, vehicleID, ok := vehiclePrincipalAndID(ctx)
	if !ok {
		return
	}
	var request UpdateVehicleRequest
	if err := decodeJSON(ctx, &request); err != nil {
		writeError(ctx, invalidRequest(err))
		return
	}
	vehicle, err := handler.service.UpdateVehicle(ctx.Request.Context(), principal, vehicleID, request)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, vehicle)
}

func (handler *Handler) deleteVehicle(ctx *gin.Context) {
	principal, vehicleID, ok := vehiclePrincipalAndID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteVehicle(ctx.Request.Context(), principal, vehicleID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func vehiclePrincipalAndID(ctx *gin.Context) (Principal, uuid.UUID, bool) {
	principal, ok := CurrentPrincipal(ctx)
	if !ok {
		writeError(ctx, errUnauthorized)
		return Principal{}, uuid.Nil, false
	}
	vehicleID, err := uuid.Parse(ctx.Param("vehicle_id"))
	if err != nil {
		writeError(ctx, invalidVehicleID())
		return Principal{}, uuid.Nil, false
	}
	return principal, vehicleID, true
}

func (service *Service) CreateVehicle(ctx context.Context, principal Principal, request CreateVehicleRequest) (CustomerVehicleView, error) {
	vehicleNumber, err := normalizeVehicleNumber(request.VehicleNumber)
	if err != nil {
		return CustomerVehicleView{}, err
	}
	vehicleType, err := normalizeVehicleOptional(request.VehicleType, 50, "vehicle_type")
	if err != nil {
		return CustomerVehicleView{}, err
	}
	vehicleMake, err := normalizeVehicleOptional(request.VehicleMake, 100, "vehicle_make")
	if err != nil {
		return CustomerVehicleView{}, err
	}
	vehicleModel, err := normalizeVehicleOptional(request.VehicleModel, 100, "vehicle_model")
	if err != nil {
		return CustomerVehicleView{}, err
	}
	now := service.now()
	vehicle := models.Vehicle{ID: uuid.New(), CPOID: principal.CPOID, CustomerID: principal.CustomerID, VehicleNumber: vehicleNumber, Type: vehicleType, Make: vehicleMake, Model: vehicleModel, DateAdded: now, CreatedAt: now, UpdatedAt: now}
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&vehicle).Error; err != nil {
			return fmt.Errorf("create customer vehicle: %w", err)
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_VEHICLE_CREATED", "VEHICLE", vehicle.ID, models.JSONB{}, now)
	}); err != nil {
		return CustomerVehicleView{}, err
	}
	return customerVehicleView(vehicle), nil
}

func (service *Service) GetVehicle(ctx context.Context, principal Principal, vehicleID uuid.UUID) (CustomerVehicleView, error) {
	if vehicleID == uuid.Nil {
		return CustomerVehicleView{}, invalidVehicleID()
	}
	vehicle, err := service.customerVehicle(ctx, service.database, principal, vehicleID)
	if err != nil {
		return CustomerVehicleView{}, err
	}
	return customerVehicleView(vehicle), nil
}

func (service *Service) ListVehicles(ctx context.Context, principal Principal, query CustomerVehicleListQuery) (CustomerVehicleListResponse, error) {
	if err := validateCustomerVehicleListQuery(&query); err != nil {
		return CustomerVehicleListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).Model(&models.Vehicle{}).Where("cpo_id = ? AND customer_id = ?", principal.CPOID, principal.CustomerID)
	if query.Search != "" {
		pattern := "%" + query.Search + "%"
		databaseQuery = databaseQuery.Where("vehicle_number ILIKE ? OR type ILIKE ? OR make ILIKE ? OR model ILIKE ?", pattern, pattern, pattern, pattern)
	}
	if query.VehicleType != "" {
		databaseQuery = databaseQuery.Where("lower(btrim(type)) = lower(?)", query.VehicleType)
	}
	if query.VehicleMake != "" {
		databaseQuery = databaseQuery.Where("lower(btrim(make)) = lower(?)", query.VehicleMake)
	}
	if query.VehicleModel != "" {
		databaseQuery = databaseQuery.Where("lower(btrim(model)) = lower(?)", query.VehicleModel)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(date_added, id) < (?, ?)", *query.Before, *query.BeforeID)
	}
	var vehicles []models.Vehicle
	if err := databaseQuery.Order("date_added DESC, id DESC").Limit(query.Limit + 1).Find(&vehicles).Error; err != nil {
		return CustomerVehicleListResponse{}, fmt.Errorf("list customer vehicles: %w", err)
	}
	response := CustomerVehicleListResponse{HasMore: len(vehicles) > query.Limit}
	if response.HasMore {
		vehicles = vehicles[:query.Limit]
	}
	response.Vehicles = make([]CustomerVehicleView, 0, len(vehicles))
	for _, vehicle := range vehicles {
		response.Vehicles = append(response.Vehicles, customerVehicleView(vehicle))
	}
	if response.HasMore && len(vehicles) > 0 {
		last := vehicles[len(vehicles)-1]
		response.NextBefore = &last.DateAdded
		response.NextBeforeID = &last.ID
	}
	return response, nil
}

func (service *Service) UpdateVehicle(ctx context.Context, principal Principal, vehicleID uuid.UUID, request UpdateVehicleRequest) (CustomerVehicleView, error) {
	if vehicleID == uuid.Nil {
		return CustomerVehicleView{}, invalidVehicleID()
	}
	if !request.vehicleNumberSet && !request.vehicleTypeSet && !request.vehicleMakeSet && !request.vehicleModelSet {
		return CustomerVehicleView{}, &APIError{http.StatusBadRequest, "invalid_vehicle_update", "At least one editable vehicle field is required."}
	}
	returnValue := CustomerVehicleView{}
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		vehicle, err := service.customerVehicle(ctx, tx, principal, vehicleID)
		if err != nil {
			return err
		}
		updates := map[string]any{}
		changedFields := make([]string, 0, 4)
		if request.vehicleNumberSet {
			if request.VehicleNumber == nil {
				return invalidVehicleNumber()
			}
			value, err := normalizeVehicleNumber(*request.VehicleNumber)
			if err != nil {
				return err
			}
			if value != vehicle.VehicleNumber {
				updates["vehicle_number"] = value
				changedFields = append(changedFields, "vehicle_number")
			}
		}
		if request.vehicleTypeSet {
			value, err := normalizeVehicleOptional(request.VehicleType, 50, "vehicle_type")
			if err != nil {
				return err
			}
			if !sameVehicleOptional(value, vehicle.Type) {
				updates["type"] = value
				changedFields = append(changedFields, "vehicle_type")
			}
		}
		if request.vehicleMakeSet {
			value, err := normalizeVehicleOptional(request.VehicleMake, 100, "vehicle_make")
			if err != nil {
				return err
			}
			if !sameVehicleOptional(value, vehicle.Make) {
				updates["make"] = value
				changedFields = append(changedFields, "vehicle_make")
			}
		}
		if request.vehicleModelSet {
			value, err := normalizeVehicleOptional(request.VehicleModel, 100, "vehicle_model")
			if err != nil {
				return err
			}
			if !sameVehicleOptional(value, vehicle.Model) {
				updates["model"] = value
				changedFields = append(changedFields, "vehicle_model")
			}
		}
		if len(updates) == 0 {
			returnValue = customerVehicleView(vehicle)
			return nil
		}
		now := service.now()
		updates["updated_at"] = now
		if err := tx.Model(&models.Vehicle{}).Where("id = ? AND cpo_id = ? AND customer_id = ?", vehicleID, principal.CPOID, principal.CustomerID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update customer vehicle: %w", err)
		}
		if err := createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_VEHICLE_UPDATED", "VEHICLE", vehicleID, models.JSONB{"changed_fields": changedFields}, now); err != nil {
			return err
		}
		updated, err := service.customerVehicle(ctx, tx, principal, vehicleID)
		if err != nil {
			return err
		}
		returnValue = customerVehicleView(updated)
		return nil
	})
	return returnValue, err
}

func (service *Service) DeleteVehicle(ctx context.Context, principal Principal, vehicleID uuid.UUID) error {
	if vehicleID == uuid.Nil {
		return invalidVehicleID()
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND cpo_id = ? AND customer_id = ?", vehicleID, principal.CPOID, principal.CustomerID).Delete(&models.Vehicle{})
		if result.Error != nil {
			return fmt.Errorf("delete customer vehicle: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return vehicleNotFound()
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_VEHICLE_DELETED", "VEHICLE", vehicleID, models.JSONB{}, service.now())
	})
}

func (service *Service) customerVehicle(ctx context.Context, database *gorm.DB, principal Principal, vehicleID uuid.UUID) (models.Vehicle, error) {
	var vehicle models.Vehicle
	if err := database.WithContext(ctx).First(&vehicle, "id = ? AND cpo_id = ? AND customer_id = ?", vehicleID, principal.CPOID, principal.CustomerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Vehicle{}, vehicleNotFound()
		}
		return models.Vehicle{}, fmt.Errorf("load customer vehicle: %w", err)
	}
	return vehicle, nil
}

func customerVehicleListQuery(ctx *gin.Context) (CustomerVehicleListQuery, error) {
	query := CustomerVehicleListQuery{Search: ctx.Query("search"), VehicleType: ctx.Query("vehicle_type"), VehicleMake: ctx.Query("vehicle_make"), VehicleModel: ctx.Query("vehicle_model")}
	if raw := strings.TrimSpace(ctx.Query("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return CustomerVehicleListQuery{}, &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be a number between 1 and 100."}
		}
		query.Limit = limit
	}
	var err error
	query.Before, err = parseCustomerCursorTime(ctx.Query("before"))
	if err != nil {
		return CustomerVehicleListQuery{}, err
	}
	query.BeforeID, err = parseCustomerCursorID(ctx.Query("before_id"))
	if err != nil {
		return CustomerVehicleListQuery{}, err
	}
	return query, validateCustomerVehicleListQuery(&query)
}

func validateCustomerVehicleListQuery(query *CustomerVehicleListQuery) error {
	if query.Limit == 0 {
		query.Limit = vehicleDefaultLimit
	}
	if query.Limit < 1 || query.Limit > vehicleMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	query.Search = strings.TrimSpace(query.Search)
	query.VehicleType = strings.TrimSpace(query.VehicleType)
	query.VehicleMake = strings.TrimSpace(query.VehicleMake)
	query.VehicleModel = strings.TrimSpace(query.VehicleModel)
	if len(query.Search) > 100 {
		return &APIError{http.StatusBadRequest, "invalid_search", "Search must not exceed 100 characters."}
	}
	if len(query.VehicleType) > 50 {
		return &APIError{http.StatusBadRequest, "invalid_vehicle_type", "Vehicle type must not exceed 50 characters."}
	}
	if len(query.VehicleMake) > 100 {
		return &APIError{http.StatusBadRequest, "invalid_vehicle_make", "Vehicle make must not exceed 100 characters."}
	}
	if len(query.VehicleModel) > 100 {
		return &APIError{http.StatusBadRequest, "invalid_vehicle_model", "Vehicle model must not exceed 100 characters."}
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Both before and before_id are required together."}
	}
	return nil
}

func normalizeVehicleNumber(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 50 {
		return "", invalidVehicleNumber()
	}
	return value, nil
}

func normalizeVehicleOptional(value *string, maximum int, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, nil
	}
	if len(normalized) > maximum {
		return nil, invalidVehicleField(field)
	}
	return &normalized, nil
}

func sameVehicleOptional(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func customerVehicleView(vehicle models.Vehicle) CustomerVehicleView {
	return CustomerVehicleView{ID: vehicle.ID, VehicleNumber: vehicle.VehicleNumber, VehicleType: vehicle.Type, VehicleMake: vehicle.Make, VehicleModel: vehicle.Model, LastCharged: vehicle.LastCharged, DateAdded: vehicle.DateAdded, CreatedAt: vehicle.CreatedAt, UpdatedAt: vehicle.UpdatedAt}
}

func invalidVehicleID() *APIError {
	return &APIError{http.StatusBadRequest, "invalid_vehicle_id", "The vehicle ID is invalid."}
}

func invalidVehicleNumber() *APIError {
	return &APIError{http.StatusBadRequest, "invalid_vehicle_number", "Vehicle number must contain 1 to 50 characters."}
}

func invalidVehicleField(field string) *APIError {
	labels := map[string]string{"vehicle_type": "Vehicle type", "vehicle_make": "Vehicle make", "vehicle_model": "Vehicle model"}
	return &APIError{http.StatusBadRequest, "invalid_" + field, labels[field] + " is too long."}
}

func vehicleNotFound() *APIError {
	return &APIError{http.StatusNotFound, "vehicle_not_found", "The vehicle was not found."}
}
