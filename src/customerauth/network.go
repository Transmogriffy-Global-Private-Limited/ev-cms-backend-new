package customerauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	customerNetworkDefaultLimit = 25
	customerNetworkMaxLimit     = 100
	customerAvailabilityUnknown = "UNKNOWN"
)

var customerChargerIDPattern = regexp.MustCompile(`^[a-z0-9]{6}$`)

type CustomerHubListQuery struct {
	Before   *time.Time
	BeforeID *uuid.UUID
	Limit    int
	Search   string
}

type CustomerHubListResponse struct {
	Hubs         []CustomerHubSummary `json:"hubs"`
	NextBefore   *time.Time           `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID           `json:"next_before_id,omitempty"`
	HasMore      bool                 `json:"has_more"`
}

type CustomerHubSummary struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	Open24Hours     bool      `json:"open_24_hours"`
	CustomerVisible bool      `json:"customer_visible"`
	ChargerCount    int       `json:"charger_count"`
	IsFavorite      bool      `json:"is_favorite"`
}

type CustomerHubView struct {
	CustomerHubSummary
	Chargers []CustomerChargerView `json:"chargers"`
}

type CustomerChargerView struct {
	ID           uuid.UUID               `json:"id"`
	HubID        uuid.UUID               `json:"hub_id"`
	ChargerID    string                  `json:"charger_id"`
	Vendor       string                  `json:"vendor"`
	Model        string                  `json:"model"`
	MaxPowerKW   float64                 `json:"max_power_kw"`
	OCPPVersion  string                  `json:"ocpp_version"`
	Availability string                  `json:"availability"`
	IsFavorite   bool                    `json:"is_favorite"`
	Connectors   []CustomerConnectorView `json:"connectors"`
}

type CustomerConnectorView struct {
	ID              uuid.UUID `json:"id"`
	ConnectorNumber int       `json:"connector_number"`
	ConnectorType   string    `json:"connector_type"`
	MaxCurrent      float64   `json:"max_current"`
	MaxVoltage      float64   `json:"max_voltage"`
	Availability    string    `json:"availability"`
}

func (service *Service) ListCustomerHubs(ctx context.Context, principal Principal, query CustomerHubListQuery) (CustomerHubListResponse, error) {
	if err := validateCustomerHubListQuery(&query); err != nil {
		return CustomerHubListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).Where(
		"cpo_id = ? AND customer_visible = ?", principal.CPOID, true,
	)
	if query.Search != "" {
		pattern := "%" + query.Search + "%"
		databaseQuery = databaseQuery.Where("name ILIKE ? OR address ILIKE ?", pattern, pattern)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(created_at, id) < (?, ?)", *query.Before, *query.BeforeID)
	}
	var records []models.Hub
	if err := databaseQuery.
		Preload("Chargers", "cpo_id = ?", principal.CPOID).
		Order("created_at DESC, id DESC").
		Limit(query.Limit + 1).
		Find(&records).Error; err != nil {
		return CustomerHubListResponse{}, fmt.Errorf("list customer hubs: %w", err)
	}
	hasMore := len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	favoriteHubs, err := service.customerFavoriteHubIDs(ctx, principal, records)
	if err != nil {
		return CustomerHubListResponse{}, err
	}
	hubs := make([]CustomerHubSummary, 0, len(records))
	for _, record := range records {
		hubs = append(hubs, customerHubSummary(record, favoriteHubs[record.ID]))
	}
	response := CustomerHubListResponse{Hubs: hubs, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		response.NextBefore = &last.CreatedAt
		response.NextBeforeID = &last.ID
	}
	return response, nil
}

func (service *Service) GetCustomerHub(ctx context.Context, principal Principal, hubID uuid.UUID) (CustomerHubView, error) {
	var record models.Hub
	if err := service.database.WithContext(ctx).
		Preload("Chargers", "cpo_id = ?", principal.CPOID).
		Preload("Chargers.Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("cpo_id = ?", principal.CPOID).Order("connector_number ASC")
		}).
		First(&record, "id = ? AND cpo_id = ? AND customer_visible = ?", hubID, principal.CPOID, true).Error; err != nil {
		return CustomerHubView{}, customerNetworkNotFound(err, "hub")
	}
	favoriteHubs, err := service.customerFavoriteHubIDs(ctx, principal, []models.Hub{record})
	if err != nil {
		return CustomerHubView{}, err
	}
	favoriteChargers, err := service.customerFavoriteChargerIDs(ctx, principal, record.Chargers)
	if err != nil {
		return CustomerHubView{}, err
	}
	chargers := make([]CustomerChargerView, 0, len(record.Chargers))
	for _, charger := range record.Chargers {
		chargers = append(chargers, customerChargerView(charger, favoriteChargers[charger.ID]))
	}
	return CustomerHubView{CustomerHubSummary: customerHubSummary(record, favoriteHubs[record.ID]), Chargers: chargers}, nil
}

func (service *Service) GetCustomerCharger(ctx context.Context, principal Principal, publicChargerID string) (CustomerChargerView, error) {
	publicChargerID = strings.ToLower(strings.TrimSpace(publicChargerID))
	if !customerChargerIDPattern.MatchString(publicChargerID) {
		return CustomerChargerView{}, &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is invalid."}
	}
	var charger models.Charger
	if err := service.database.WithContext(ctx).
		Preload("Hub").
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("cpo_id = ?", principal.CPOID).Order("connector_number ASC")
		}).
		First(&charger, "cpo_id = ? AND charger_id = ?", principal.CPOID, publicChargerID).Error; err != nil {
		return CustomerChargerView{}, customerNetworkNotFound(err, "charger")
	}
	if charger.Hub == nil || charger.Hub.CPOID != principal.CPOID || !charger.Hub.CustomerVisible {
		return CustomerChargerView{}, customerNetworkNotFound(gorm.ErrRecordNotFound, "charger")
	}
	favoriteChargers, err := service.customerFavoriteChargerIDs(ctx, principal, []models.Charger{charger})
	if err != nil {
		return CustomerChargerView{}, err
	}
	return customerChargerView(charger, favoriteChargers[charger.ID]), nil
}

func validateCustomerHubListQuery(query *CustomerHubListQuery) error {
	if query.Limit == 0 {
		query.Limit = customerNetworkDefaultLimit
	}
	if query.Limit < 1 || query.Limit > customerNetworkMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	query.Search = strings.TrimSpace(query.Search)
	if len(query.Search) > 255 {
		return &APIError{http.StatusBadRequest, "invalid_search", "Search must not exceed 255 characters."}
	}
	if (query.Before == nil) != (query.BeforeID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Both before and before_id are required together."}
	}
	return nil
}

func (service *Service) customerFavoriteHubIDs(ctx context.Context, principal Principal, hubs []models.Hub) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool, len(hubs))
	if len(hubs) == 0 {
		return result, nil
	}
	ids := make([]uuid.UUID, 0, len(hubs))
	for _, hub := range hubs {
		ids = append(ids, hub.ID)
	}
	var favorites []models.CustomerFavoriteHub
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ? AND hub_id IN ?", principal.CPOID, principal.CustomerID, ids).Find(&favorites).Error; err != nil {
		return nil, fmt.Errorf("load customer hub favorites: %w", err)
	}
	for _, favorite := range favorites {
		result[favorite.HubID] = true
	}
	return result, nil
}

func (service *Service) customerFavoriteChargerIDs(ctx context.Context, principal Principal, chargers []models.Charger) (map[uuid.UUID]bool, error) {
	result := make(map[uuid.UUID]bool, len(chargers))
	if len(chargers) == 0 {
		return result, nil
	}
	ids := make([]uuid.UUID, 0, len(chargers))
	for _, charger := range chargers {
		ids = append(ids, charger.ID)
	}
	var favorites []models.CustomerFavoriteCharger
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ? AND charger_id IN ?", principal.CPOID, principal.CustomerID, ids).Find(&favorites).Error; err != nil {
		return nil, fmt.Errorf("load customer charger favorites: %w", err)
	}
	for _, favorite := range favorites {
		result[favorite.ChargerID] = true
	}
	return result, nil
}

func customerHubSummary(record models.Hub, favorite bool) CustomerHubSummary {
	return CustomerHubSummary{ID: record.ID, Name: record.Name, Address: record.Address, Latitude: record.Latitude, Longitude: record.Longitude, Open24Hours: record.Open24Hours, CustomerVisible: true, ChargerCount: len(record.Chargers), IsFavorite: favorite}
}

func customerChargerView(record models.Charger, favorite bool) CustomerChargerView {
	hubID := uuid.Nil
	if record.HubID != nil {
		hubID = *record.HubID
	}
	connectors := make([]CustomerConnectorView, 0, len(record.Connectors))
	for _, connector := range record.Connectors {
		connectors = append(connectors, CustomerConnectorView{ID: connector.ID, ConnectorNumber: connector.ConnectorNumber, ConnectorType: connector.ConnectorType, MaxCurrent: connector.MaxCurrent, MaxVoltage: connector.MaxVoltage, Availability: customerAvailabilityUnknown})
	}
	return CustomerChargerView{ID: record.ID, HubID: hubID, ChargerID: record.ChargerID, Vendor: record.Vendor, Model: record.Model, MaxPowerKW: record.MaxPowerKW, OCPPVersion: record.OCPPVersion, Availability: customerAvailabilityUnknown, IsFavorite: favorite, Connectors: connectors}
}

func customerNetworkNotFound(err error, resource string) error {
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load customer %s: %w", resource, err)
	}
	return &APIError{http.StatusNotFound, resource + "_not_found", "The requested charging resource was not found."}
}
