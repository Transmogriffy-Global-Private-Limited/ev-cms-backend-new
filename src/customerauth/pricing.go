package customerauth

import (
	"context"
	"net/http"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	customerPriceAvailable   = "AVAILABLE"
	customerPriceUnavailable = "UNAVAILABLE"
)

type CustomerGSTView struct {
	SGSTRate string `json:"sgst_rate"`
	CGSTRate string `json:"cgst_rate"`
	IGSTRate string `json:"igst_rate"`
}

type CustomerPriceResponse struct {
	Status            string           `json:"status"`
	EffectiveAt       time.Time        `json:"effective_at"`
	Currency          string           `json:"currency,omitempty"`
	PricePerKWh       string           `json:"price_per_kwh,omitempty"`
	IdleFeePerMinute  string           `json:"idle_fee_per_minute,omitempty"`
	GST               *CustomerGSTView `json:"gst,omitempty"`
	UnavailableReason string           `json:"unavailable_reason,omitempty"`
}

func (service *Service) GetCustomerHubPrice(ctx context.Context, principal Principal, hubID uuid.UUID) (CustomerPriceResponse, error) {
	if err := service.customerVisibleHubExists(ctx, principal, hubID); err != nil {
		return CustomerPriceResponse{}, err
	}
	return service.resolveCustomerPrice(ctx, principal, hubID, nil, service.now()), nil
}

func (service *Service) GetCustomerChargerPrice(ctx context.Context, principal Principal, publicChargerID string) (CustomerPriceResponse, error) {
	charger, err := service.loadPublishedCustomerCharger(ctx, principal, publicChargerID)
	if err != nil {
		return CustomerPriceResponse{}, err
	}
	return service.resolveCustomerPrice(ctx, principal, *charger.HubID, &charger.ID, service.now()), nil
}

func (service *Service) customerVisibleHubExists(ctx context.Context, principal Principal, hubID uuid.UUID) error {
	if hubID == uuid.Nil {
		return &APIError{http.StatusBadRequest, "invalid_hub_id", "The hub ID is invalid."}
	}
	var hub models.Hub
	if err := service.database.WithContext(ctx).First(&hub, "id = ? AND cpo_id = ? AND customer_visible = ?", hubID, principal.CPOID, true).Error; err != nil {
		return customerNetworkNotFound(err, "hub")
	}
	return nil
}

func (service *Service) loadPublishedCustomerCharger(ctx context.Context, principal Principal, publicChargerID string) (models.Charger, error) {
	publicChargerID = normalizeCustomerChargerID(publicChargerID)
	if !customerChargerIDPattern.MatchString(publicChargerID) {
		return models.Charger{}, &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is invalid."}
	}
	var charger models.Charger
	if err := service.database.WithContext(ctx).Preload("Hub").First(&charger, "cpo_id = ? AND charger_id = ?", principal.CPOID, publicChargerID).Error; err != nil {
		return models.Charger{}, customerNetworkNotFound(err, "charger")
	}
	if charger.Hub == nil || charger.Hub.CPOID != principal.CPOID || !charger.Hub.CustomerVisible {
		return models.Charger{}, customerNetworkNotFound(gorm.ErrRecordNotFound, "charger")
	}
	return charger, nil
}

func (service *Service) resolveCustomerPrice(ctx context.Context, principal Principal, hubID uuid.UUID, chargerID *uuid.UUID, effectiveAt time.Time) CustomerPriceResponse {
	response := CustomerPriceResponse{Status: customerPriceUnavailable, EffectiveAt: effectiveAt, UnavailableReason: "no_eligible_tariff"}
	query := service.database.WithContext(ctx).Where(
		"cpo_id = ? AND hub_id = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date > ?)",
		principal.CPOID, hubID, true, effectiveAt, effectiveAt,
	)
	if chargerID == nil {
		query = query.Where("charger_id IS NULL")
	} else {
		query = query.Where("charger_id IS NULL OR charger_id = ?", *chargerID)
	}
	if principal.Customer.UserGroupID == nil {
		query = query.Where("user_group_id IS NULL")
	} else {
		query = query.Where("user_group_id IS NULL OR user_group_id = ?", *principal.Customer.UserGroupID)
	}
	var tariffs []models.Tariff
	if err := query.Preload("GST", "cpo_id = ? AND is_active = ?", principal.CPOID, true).Find(&tariffs).Error; err != nil {
		return response
	}
	selected, ok := selectCustomerTariff(tariffs, chargerID, principal.Customer.UserGroupID)
	if !ok || (selected.GSTID != nil && selected.GST == nil) {
		return response
	}
	response.Status = customerPriceAvailable
	response.Currency = selected.Currency
	response.PricePerKWh = selected.PricePerKWh.StringFixed(4)
	response.IdleFeePerMinute = selected.IdleFeePerMin.StringFixed(4)
	response.UnavailableReason = ""
	if selected.GST != nil {
		response.GST = &CustomerGSTView{SGSTRate: selected.GST.SGSTRate.StringFixed(2), CGSTRate: selected.GST.CGSTRate.StringFixed(2), IGSTRate: selected.GST.IGSTRate.StringFixed(2)}
	}
	return response
}

func selectCustomerTariff(tariffs []models.Tariff, chargerID, userGroupID *uuid.UUID) (models.Tariff, bool) {
	bestRank := 99
	var selected models.Tariff
	for _, tariff := range tariffs {
		rank := customerTariffRank(tariff, chargerID, userGroupID)
		if rank < bestRank {
			bestRank = rank
			selected = tariff
		}
	}
	if bestRank == 99 {
		return models.Tariff{}, false
	}
	return selected, true
}

func customerTariffRank(tariff models.Tariff, chargerID, userGroupID *uuid.UUID) int {
	chargerMatch := tariff.ChargerID != nil && chargerID != nil && *tariff.ChargerID == *chargerID
	groupMatch := tariff.UserGroupID != nil && userGroupID != nil && *tariff.UserGroupID == *userGroupID
	if chargerMatch && groupMatch {
		return 1
	}
	if tariff.ChargerID == nil && groupMatch {
		return 2
	}
	if chargerMatch && tariff.UserGroupID == nil {
		return 3
	}
	if tariff.ChargerID == nil && tariff.UserGroupID == nil {
		return 4
	}
	return 99
}
