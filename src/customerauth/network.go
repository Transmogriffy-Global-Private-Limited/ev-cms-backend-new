package customerauth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
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

type CustomerChargerListQuery struct {
	Before        *time.Time
	BeforeID      *uuid.UUID
	Limit         int
	Search        string
	ConnectorType string
	MinPowerKW    *float64
	MaxPowerKW    *float64
	Latitude      *float64
	Longitude     *float64
	RadiusKM      *float64
	Open24Hours   *bool `json:"twenty_four_seven_open_status"`
}

type CustomerChargerListResponse struct {
	Chargers     []CustomerChargerView `json:"chargers"`
	NextBefore   *time.Time            `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID            `json:"next_before_id,omitempty"`
	HasMore      bool                  `json:"has_more"`
}

type CustomerHubSummary struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	Open24Hours     bool      `json:"twenty_four_seven_open_status"`
	CustomerVisible bool      `json:"customer_visible"`
	ChargerCount    int       `json:"charger_count"`
	IsFavorite      bool      `json:"is_favorite"`
}

type CustomerHubView struct {
	CustomerHubSummary
	Chargers []CustomerChargerView `json:"chargers"`
}

type CustomerChargerView struct {
	ID              uuid.UUID               `json:"id"`
	HubID           uuid.UUID               `json:"hub_id"`
	ChargerID       string                  `json:"charger_id"`
	ChargerName     string                  `json:"charger_name,omitempty"`
	Vendor          *string                 `json:"vendor,omitempty"`
	Model           *string                 `json:"model,omitempty"`
	MaxPowerKW      float64                 `json:"max_power_kw"`
	OCPPVersion     string                  `json:"ocpp_version"`
	Status          constants.ChargerStatus `json:"status"`
	ChargerImageURL *string                 `json:"charger_image_url,omitempty"`
	ChargerType     string                  `json:"charger_type,omitempty"`
	Segment         string                  `json:"segment,omitempty"`
	SubSegment      string                  `json:"sub_segment,omitempty"`
	ChargerUseType  string                  `json:"charger_use_type,omitempty"`
	Parking         string                  `json:"parking,omitempty"`
	HubName         string                  `json:"hub_name,omitempty"`
	HubAddress      string                  `json:"hub_address,omitempty"`
	HubLatitude     *float64                `json:"hub_latitude,omitempty"`
	HubLongitude    *float64                `json:"hub_longitude,omitempty"`
	Open24Hours     *bool                   `json:"twenty_four_seven_open_status,omitempty"`
	DistanceKM      *float64                `json:"distance_km,omitempty"`
	Availability    string                  `json:"availability"`
	IsFavorite      bool                    `json:"is_favorite"`
	Connectors      []CustomerConnectorView `json:"connectors"`
}

const customerChargerSearchRadiusKM = 10.0

func (service *Service) ListCustomerChargers(ctx context.Context, principal Principal, query CustomerChargerListQuery) (CustomerChargerListResponse, error) {
	if err := validateCustomerChargerListQuery(&query); err != nil {
		return CustomerChargerListResponse{}, err
	}
	databaseQuery := service.database.WithContext(ctx).Model(&models.Charger{}).
		Joins("JOIN hubs ON hubs.id = chargers.hub_id AND hubs.cpo_id = chargers.cpo_id").
		Where("chargers.cpo_id = ? AND hubs.customer_visible = ?", principal.CPOID, true)
	if query.Search != "" {
		pattern := "%" + query.Search + "%"
		databaseQuery = databaseQuery.Where(
			"chargers.charger_id ILIKE ? OR chargers.vendor ILIKE ? OR chargers.model ILIKE ? OR hubs.name ILIKE ? OR hubs.address ILIKE ?",
			pattern, pattern, pattern, pattern, pattern,
		)
	}
	if query.ConnectorType != "" {
		pattern := "%" + query.ConnectorType + "%"
		databaseQuery = databaseQuery.Where(
			"EXISTS (SELECT 1 FROM connectors WHERE connectors.cpo_id = chargers.cpo_id AND connectors.charger_id = chargers.id AND connectors.connector_type ILIKE ?)",
			pattern,
		)
	}
	if query.MinPowerKW != nil {
		databaseQuery = databaseQuery.Where("chargers.max_power_kw >= ?", *query.MinPowerKW)
	}
	if query.MaxPowerKW != nil {
		databaseQuery = databaseQuery.Where("chargers.max_power_kw <= ?", *query.MaxPowerKW)
	}
	if query.Open24Hours != nil {
		databaseQuery = databaseQuery.Where("hubs.open_24_hours = ?", *query.Open24Hours)
	}
	if query.Before != nil {
		databaseQuery = databaseQuery.Where("(chargers.created_at, chargers.id) < (?, ?)", *query.Before, *query.BeforeID)
	}
	if query.Latitude != nil {
		distanceExpression := customerChargerDistanceExpression()
		databaseQuery = databaseQuery.
			Select("chargers.*, "+distanceExpression+" AS customer_distance_km", *query.Latitude, *query.Longitude, *query.Latitude).
			Where(distanceExpression+" <= ?", *query.Latitude, *query.Longitude, *query.Latitude, *query.RadiusKM).
			Order("customer_distance_km ASC, chargers.created_at DESC, chargers.id DESC")
	} else {
		databaseQuery = databaseQuery.Order("chargers.created_at DESC, chargers.id DESC")
	}
	var records []models.Charger
	limit := query.Limit
	if query.Latitude == nil {
		limit++
	}
	if err := databaseQuery.
		Preload("Hub").
		Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("cpo_id = ?", principal.CPOID).Order("connector_number ASC")
		}).
		Limit(limit).
		Find(&records).Error; err != nil {
		return CustomerChargerListResponse{}, fmt.Errorf("list customer chargers: %w", err)
	}
	hasMore := false
	if query.Latitude == nil && len(records) > query.Limit {
		hasMore = true
		records = records[:query.Limit]
	}
	favorites, err := service.customerFavoriteChargerIDs(ctx, principal, records)
	if err != nil {
		return CustomerChargerListResponse{}, err
	}
	chargers := make([]CustomerChargerView, 0, len(records))
	for _, record := range records {
		view := customerChargerView(record, favorites[record.ID])
		if query.Latitude != nil && record.Hub != nil {
			distance := haversineDistanceKM(*query.Latitude, *query.Longitude, record.Hub.Latitude, record.Hub.Longitude)
			view.DistanceKM = &distance
		}
		chargers = append(chargers, view)
	}
	response := CustomerChargerListResponse{Chargers: chargers, HasMore: hasMore}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		response.NextBefore = &last.CreatedAt
		response.NextBeforeID = &last.ID
	}
	return response, nil
}

type CustomerConnectorView struct {
	ID                     uuid.UUID               `json:"id"`
	ConnectorNumber        int                     `json:"connector_number"`
	ConnectorType          string                  `json:"connector_type"`
	ConnectorTotalCapacity float64                 `json:"connector_total_capacity"`
	Status                 constants.ChargerStatus `json:"status"`
	Availability           string                  `json:"availability"`
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

func validateCustomerChargerListQuery(query *CustomerChargerListQuery) error {
	if query.Limit == 0 {
		query.Limit = customerNetworkDefaultLimit
	}
	if query.Limit < 1 || query.Limit > customerNetworkMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	query.Search = strings.TrimSpace(query.Search)
	query.ConnectorType = strings.TrimSpace(query.ConnectorType)
	if len(query.Search) > 255 {
		return &APIError{http.StatusBadRequest, "invalid_search", "Search must not exceed 255 characters."}
	}
	if len(query.ConnectorType) > 50 {
		return &APIError{http.StatusBadRequest, "invalid_connector_type", "Connector type must not exceed 50 characters."}
	}
	if query.MinPowerKW != nil && *query.MinPowerKW < 0 {
		return &APIError{http.StatusBadRequest, "invalid_min_power_kw", "Minimum power cannot be negative."}
	}
	if query.MaxPowerKW != nil && *query.MaxPowerKW < 0 {
		return &APIError{http.StatusBadRequest, "invalid_max_power_kw", "Maximum power cannot be negative."}
	}
	if query.MinPowerKW != nil && query.MaxPowerKW != nil && *query.MinPowerKW > *query.MaxPowerKW {
		return &APIError{http.StatusBadRequest, "invalid_power_range", "Minimum power cannot exceed maximum power."}
	}
	locationSupplied := query.Latitude != nil || query.Longitude != nil
	if locationSupplied && (query.Latitude == nil || query.Longitude == nil) {
		return &APIError{http.StatusBadRequest, "invalid_location", "Latitude and longitude are required together."}
	}
	if query.Latitude != nil && (*query.Latitude < -90 || *query.Latitude > 90) {
		return &APIError{http.StatusBadRequest, "invalid_latitude", "Latitude must be between -90 and 90."}
	}
	if query.Longitude != nil && (*query.Longitude < -180 || *query.Longitude > 180) {
		return &APIError{http.StatusBadRequest, "invalid_longitude", "Longitude must be between -180 and 180."}
	}
	if locationSupplied {
		if query.RadiusKM == nil {
			query.RadiusKM = func() *float64 { value := customerChargerSearchRadiusKM; return &value }()
		}
		if *query.RadiusKM <= 0 || *query.RadiusKM > 100 {
			return &APIError{http.StatusBadRequest, "invalid_radius_km", "Radius must be greater than 0 and no more than 100 km."}
		}
		if query.Before != nil || query.BeforeID != nil {
			return &APIError{http.StatusBadRequest, "invalid_cursor", "Location searches do not support before cursors."}
		}
	} else if query.RadiusKM != nil {
		return &APIError{http.StatusBadRequest, "invalid_location", "Radius requires latitude and longitude."}
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
		connectors = append(connectors, CustomerConnectorView{
			ID:                     connector.ID,
			ConnectorNumber:        connector.ConnectorNumber,
			ConnectorType:          connector.ConnectorType,
			ConnectorTotalCapacity: connector.ConnectorTotalCapacity,
			Status:                 connector.Status,
			Availability:           customerAvailabilityUnknown,
		})
	}
	view := CustomerChargerView{ID: record.ID, HubID: hubID, ChargerID: record.ChargerID, ChargerName: record.ChargerName, Vendor: record.Vendor, Model: record.Model, MaxPowerKW: record.MaxPowerKW, OCPPVersion: record.OCPPVersion, Status: record.Status, ChargerImageURL: customerChargerImageURL(record), ChargerType: record.ChargerType, Segment: record.Segment, SubSegment: record.SubSegment, ChargerUseType: record.ChargerUseType, Parking: record.Parking, Availability: customerAvailabilityUnknown, IsFavorite: favorite, Connectors: connectors}
	if record.Hub != nil {
		open24Hours := record.Hub.Open24Hours
		view.HubName = record.Hub.Name
		view.HubAddress = record.Hub.Address
		view.HubLatitude = &record.Hub.Latitude
		view.HubLongitude = &record.Hub.Longitude
		view.Open24Hours = &open24Hours
	}
	return view
}

func customerChargerImageURL(record models.Charger) *string {
	if strings.TrimSpace(record.ChargerImage) == "" {
		return nil
	}
	value := "/api/v1/app/chargers/" + record.ChargerID + "/image"
	return &value
}

func customerChargerDistanceExpression() string {
	return "6371.0 * acos(LEAST(1.0, GREATEST(-1.0, cos(radians(?)) * cos(radians(hubs.latitude)) * cos(radians(hubs.longitude) - radians(?)) + sin(radians(?)) * sin(radians(hubs.latitude)))))"
}

func haversineDistanceKM(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	const earthRadiusKM = 6371.0
	latitudeDelta := (latitudeB - latitudeA) * (3.141592653589793 / 180)
	longitudeDelta := (longitudeB - longitudeA) * (3.141592653589793 / 180)
	latitudeA *= 3.141592653589793 / 180
	latitudeB *= 3.141592653589793 / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) + math.Cos(latitudeA)*math.Cos(latitudeB)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	return math.Round(earthRadiusKM*2*math.Asin(math.Sqrt(a))*100) / 100
}

func customerNetworkNotFound(err error, resource string) error {
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load customer %s: %w", resource, err)
	}
	return &APIError{http.StatusNotFound, resource + "_not_found", "The requested charging resource was not found."}
}
