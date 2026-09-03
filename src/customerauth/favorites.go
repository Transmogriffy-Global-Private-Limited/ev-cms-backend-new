package customerauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerFavoritesQuery struct {
	Limit           int
	HubBefore       *time.Time
	HubBeforeID     *uuid.UUID
	ChargerBefore   *time.Time
	ChargerBeforeID *uuid.UUID
}

type CustomerFavoritesResponse struct {
	Hubs                []CustomerHubSummary  `json:"hubs"`
	Chargers            []CustomerChargerView `json:"chargers"`
	NextHubBefore       *time.Time            `json:"next_hub_before,omitempty"`
	NextHubBeforeID     *uuid.UUID            `json:"next_hub_before_id,omitempty"`
	HasMoreHubs         bool                  `json:"has_more_hubs"`
	NextChargerBefore   *time.Time            `json:"next_charger_before,omitempty"`
	NextChargerBeforeID *uuid.UUID            `json:"next_charger_before_id,omitempty"`
	HasMoreChargers     bool                  `json:"has_more_chargers"`
}

func (service *Service) AddFavoriteHub(ctx context.Context, principal Principal, hubID uuid.UUID) error {
	if hubID == uuid.Nil {
		return invalidFavoriteID("hub")
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hub models.Hub
		if err := tx.First(&hub, "id = ? AND cpo_id = ? AND customer_visible = ?", hubID, principal.CPOID, true).Error; err != nil {
			return customerNetworkNotFound(err, "hub")
		}
		favorite := models.CustomerFavoriteHub{CPOID: principal.CPOID, CustomerID: principal.CustomerID, HubID: hub.ID, CreatedAt: service.now()}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&favorite)
		if result.Error != nil {
			return fmt.Errorf("add customer hub favorite: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_FAVORITE_HUB_ADDED", "HUB", hub.ID, models.JSONB{}, favorite.CreatedAt)
	})
}

func (service *Service) RemoveFavoriteHub(ctx context.Context, principal Principal, hubID uuid.UUID) error {
	if hubID == uuid.Nil {
		return invalidFavoriteID("hub")
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("cpo_id = ? AND customer_id = ? AND hub_id = ?", principal.CPOID, principal.CustomerID, hubID).Delete(&models.CustomerFavoriteHub{})
		if result.Error != nil {
			return fmt.Errorf("remove customer hub favorite: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_FAVORITE_HUB_REMOVED", "HUB", hubID, models.JSONB{}, service.now())
	})
}

func (service *Service) AddFavoriteCharger(ctx context.Context, principal Principal, publicChargerID string) error {
	publicChargerID = normalizeCustomerChargerID(publicChargerID)
	if !customerChargerIDPattern.MatchString(publicChargerID) {
		return &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is invalid."}
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		charger, err := loadPublishedCustomerCharger(tx, principal.CPOID, publicChargerID)
		if err != nil {
			return err
		}
		favorite := models.CustomerFavoriteCharger{CPOID: principal.CPOID, CustomerID: principal.CustomerID, ChargerID: charger.ID, CreatedAt: service.now()}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&favorite)
		if result.Error != nil {
			return fmt.Errorf("add customer charger favorite: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_FAVORITE_CHARGER_ADDED", "CHARGER", charger.ID, models.JSONB{}, favorite.CreatedAt)
	})
}

func (service *Service) RemoveFavoriteCharger(ctx context.Context, principal Principal, publicChargerID string) error {
	publicChargerID = normalizeCustomerChargerID(publicChargerID)
	if !customerChargerIDPattern.MatchString(publicChargerID) {
		return invalidFavoriteID("charger")
	}
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var charger models.Charger
		if err := tx.First(&charger, "cpo_id = ? AND charger_id = ?", principal.CPOID, publicChargerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("load customer charger favorite: %w", err)
		}
		result := tx.Where("cpo_id = ? AND customer_id = ? AND charger_id = ?", principal.CPOID, principal.CustomerID, charger.ID).Delete(&models.CustomerFavoriteCharger{})
		if result.Error != nil {
			return fmt.Errorf("remove customer charger favorite: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return createCustomerAudit(tx, principal.CustomerID, principal.CPOID, "CUSTOMER_FAVORITE_CHARGER_REMOVED", "CHARGER", charger.ID, models.JSONB{}, service.now())
	})
}

func (service *Service) ListCustomerFavorites(ctx context.Context, principal Principal, query CustomerFavoritesQuery) (CustomerFavoritesResponse, error) {
	if err := validateCustomerFavoritesQuery(&query); err != nil {
		return CustomerFavoritesResponse{}, err
	}
	response := CustomerFavoritesResponse{}
	var hubFavorites []models.CustomerFavoriteHub
	hubQuery := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ?", principal.CPOID, principal.CustomerID)
	if query.HubBefore != nil {
		hubQuery = hubQuery.Where("(created_at, hub_id) < (?, ?)", *query.HubBefore, *query.HubBeforeID)
	}
	if err := hubQuery.Order("created_at DESC, hub_id DESC").Limit(query.Limit + 1).Find(&hubFavorites).Error; err != nil {
		return CustomerFavoritesResponse{}, fmt.Errorf("list customer hub favorites: %w", err)
	}
	response.HasMoreHubs = len(hubFavorites) > query.Limit
	if response.HasMoreHubs {
		hubFavorites = hubFavorites[:query.Limit]
	}
	if len(hubFavorites) > 0 {
		ids := make([]uuid.UUID, 0, len(hubFavorites))
		for _, favorite := range hubFavorites {
			ids = append(ids, favorite.HubID)
		}
		var hubs []models.Hub
		if err := service.database.WithContext(ctx).Preload("Chargers", "cpo_id = ? AND customer_visibility = ?", principal.CPOID, true).Where("cpo_id = ? AND customer_visible = ? AND id IN ?", principal.CPOID, true, ids).Find(&hubs).Error; err != nil {
			return CustomerFavoritesResponse{}, fmt.Errorf("load favorite customer hubs: %w", err)
		}
		byID := make(map[uuid.UUID]models.Hub, len(hubs))
		for _, hub := range hubs {
			byID[hub.ID] = hub
		}
		for _, favorite := range hubFavorites {
			if hub, ok := byID[favorite.HubID]; ok {
				response.Hubs = append(response.Hubs, customerHubSummary(hub, true))
			}
		}
	}
	if response.HasMoreHubs && len(hubFavorites) > 0 {
		last := hubFavorites[len(hubFavorites)-1]
		response.NextHubBefore = &last.CreatedAt
		response.NextHubBeforeID = &last.HubID
	}

	var chargerFavorites []models.CustomerFavoriteCharger
	chargerQuery := service.database.WithContext(ctx).Where("cpo_id = ? AND customer_id = ?", principal.CPOID, principal.CustomerID)
	if query.ChargerBefore != nil {
		chargerQuery = chargerQuery.Where("(created_at, charger_id) < (?, ?)", *query.ChargerBefore, *query.ChargerBeforeID)
	}
	if err := chargerQuery.Order("created_at DESC, charger_id DESC").Limit(query.Limit + 1).Find(&chargerFavorites).Error; err != nil {
		return CustomerFavoritesResponse{}, fmt.Errorf("list customer charger favorites: %w", err)
	}
	response.HasMoreChargers = len(chargerFavorites) > query.Limit
	if response.HasMoreChargers {
		chargerFavorites = chargerFavorites[:query.Limit]
	}
	if len(chargerFavorites) > 0 {
		ids := make([]uuid.UUID, 0, len(chargerFavorites))
		for _, favorite := range chargerFavorites {
			ids = append(ids, favorite.ChargerID)
		}
		var chargers []models.Charger
		if err := service.database.WithContext(ctx).Preload("Hub").Preload("Connectors", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("cpo_id = ?", principal.CPOID).Order("connector_number ASC")
		}).Where("cpo_id = ? AND customer_visibility = ? AND id IN ?", principal.CPOID, true, ids).Find(&chargers).Error; err != nil {
			return CustomerFavoritesResponse{}, fmt.Errorf("load favorite customer chargers: %w", err)
		}
		byID := make(map[uuid.UUID]models.Charger, len(chargers))
		for _, charger := range chargers {
			if charger.Hub != nil && charger.Hub.CPOID == principal.CPOID && charger.CustomerVisibility && charger.Hub.CustomerVisible {
				byID[charger.ID] = charger
			}
		}
		for _, favorite := range chargerFavorites {
			if charger, ok := byID[favorite.ChargerID]; ok {
				response.Chargers = append(response.Chargers, customerChargerView(charger, true))
			}
		}
		if err := service.enrichCustomerChargerChargeability(ctx, principal, response.Chargers); err != nil {
			return CustomerFavoritesResponse{}, err
		}
	}
	if response.HasMoreChargers && len(chargerFavorites) > 0 {
		last := chargerFavorites[len(chargerFavorites)-1]
		response.NextChargerBefore = &last.CreatedAt
		response.NextChargerBeforeID = &last.ChargerID
	}
	return response, nil
}

func validateCustomerFavoritesQuery(query *CustomerFavoritesQuery) error {
	if query.Limit == 0 {
		query.Limit = customerNetworkDefaultLimit
	}
	if query.Limit < 1 || query.Limit > customerNetworkMaxLimit {
		return &APIError{http.StatusBadRequest, "invalid_limit", "Limit must be between 1 and 100."}
	}
	if (query.HubBefore == nil) != (query.HubBeforeID == nil) || (query.ChargerBefore == nil) != (query.ChargerBeforeID == nil) {
		return &APIError{http.StatusBadRequest, "invalid_cursor", "Favorite cursors require both timestamp and ID."}
	}
	return nil
}

func invalidFavoriteID(resource string) error {
	return &APIError{http.StatusBadRequest, "invalid_" + resource + "_id", "The " + resource + " ID is invalid."}
}

func normalizeCustomerChargerID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
