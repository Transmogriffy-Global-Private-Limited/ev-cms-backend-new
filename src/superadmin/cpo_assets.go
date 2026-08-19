
package superadmin

import (
	"context"
	"fmt"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func (service *Service) CPOAssets(ctx context.Context, principal auth.Principal) (CPOAssetsOverview, error) {
	if err := requirePlatform(principal); err != nil {
		return CPOAssetsOverview{}, err
	}

	var cpos []models.CPO
	if err := service.database.WithContext(ctx).Find(&cpos).Error; err != nil {
		return CPOAssetsOverview{}, fmt.Errorf("failed to fetch CPOs: %w", err)
	}

	cpoAssets := make([]CPOWithAssets, 0, len(cpos))
	for _, cpo := range cpos {
		var hubs []models.Hub
		if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpo.ID).Find(&hubs).Error; err != nil {
			return CPOAssetsOverview{}, fmt.Errorf("failed to fetch hubs for CPO %s: %w", cpo.ID, err)
		}

		hubInfos := make([]HubInfo, 0, len(hubs))
		for _, hub := range hubs {
			hubInfos = append(hubInfos, HubInfo{
				ID:        hub.ID,
				Name:      hub.Name,
				Status:    "ACTIVE", // Placeholder, as Hub doesn't have a status field
				CreatedAt: hub.CreatedAt,
			})
		}

		var chargers []models.Charger
		if err := service.database.WithContext(ctx).Where("cpo_id = ?", cpo.ID).Find(&chargers).Error; err != nil {
			return CPOAssetsOverview{}, fmt.Errorf("failed to fetch chargers for CPO %s: %w", cpo.ID, err)
		}

		chargerInfos := make([]ChargerInfo, 0, len(chargers))
		for _, charger := range chargers {
			chargerInfos = append(chargerInfos, ChargerInfo{
				ID:        charger.ID,
				ChargerID: charger.ChargerID,
				Status:    string(charger.Status),
				CreatedAt: charger.CreatedAt,
			})
		}

		cpoAssets = append(cpoAssets, CPOWithAssets{
			ID:           cpo.ID,
			BusinessName: cpo.BusinessName,
			Hubs:         hubInfos,
			Chargers:     chargerInfos,
		})
	}

	return CPOAssetsOverview{CPOs: cpoAssets}, nil
}
