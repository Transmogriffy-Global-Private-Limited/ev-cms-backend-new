package customerauth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
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
	PricePerUnit      string           `json:"price_per_unit,omitempty"`
	TariffType        string           `json:"tariff_type,omitempty"`
	PriceType         string           `json:"price_type,omitempty"`
	Units             *string          `json:"units,omitempty"`
	GST               *CustomerGSTView `json:"gst,omitempty"`
	UnavailableReason string           `json:"unavailable_reason,omitempty"`
}

func (service *Service) GetCustomerHubPrice(ctx context.Context, principal Principal, hubID uuid.UUID) (CustomerPriceResponse, error) {
	if err := service.customerVisibleHubExists(ctx, principal, hubID); err != nil {
		return CustomerPriceResponse{}, err
	}
	return service.resolveCustomerPrice(ctx, principal, hubID, nil, service.now())
}

func (service *Service) GetCustomerChargerPrice(ctx context.Context, principal Principal, publicChargerID string) (CustomerPriceResponse, error) {
	charger, err := service.loadPublishedCustomerCharger(ctx, principal, publicChargerID)
	if err != nil {
		return CustomerPriceResponse{}, err
	}
	return service.resolveCustomerPrice(ctx, principal, *charger.HubID, &charger.ID, service.now())
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
	return loadPublishedCustomerCharger(service.database.WithContext(ctx), principal.CPOID, publicChargerID)
}

func loadPublishedCustomerCharger(database *gorm.DB, cpoID uuid.UUID, publicChargerID string) (models.Charger, error) {
	var charger models.Charger
	if err := database.Preload("Hub").First(&charger, "cpo_id = ? AND charger_id = ?", cpoID, publicChargerID).Error; err != nil {
		return models.Charger{}, customerNetworkNotFound(err, "charger")
	}
	if charger.Hub == nil || charger.Hub.CPOID != cpoID || !charger.CustomerVisibility || !charger.Hub.CustomerVisible {
		return models.Charger{}, customerNetworkNotFound(gorm.ErrRecordNotFound, "charger")
	}
	return charger, nil
}

func (service *Service) resolveCustomerPrice(ctx context.Context, principal Principal, hubID uuid.UUID, chargerID *uuid.UUID, effectiveAt time.Time) (CustomerPriceResponse, error) {
	response := CustomerPriceResponse{Status: customerPriceUnavailable, EffectiveAt: effectiveAt, UnavailableReason: "no_eligible_tariff"}
	selected, ok, err := resolveEffectiveTariff(service.database.WithContext(ctx), principal.CPOID, principal.Customer.UserGroupID, chargerID, &hubID, effectiveAt)
	if err != nil {
		if isTariffTopologyError(err) {
			return response, nil
		}
		return CustomerPriceResponse{}, err
	}
	if !ok {
		return response, nil
	}
	pricing, err := tariffPricingFromTariff(selected)
	if err != nil {
		response.UnavailableReason = "unsupported_tariff_pricing"
		return response, nil
	}
	gst, ok, err := resolveActiveHubGST(service.database.WithContext(ctx), principal.CPOID, hubID)
	if err != nil {
		return CustomerPriceResponse{}, err
	}
	if !ok {
		response.UnavailableReason = "hub_gst_unavailable"
		return response, nil
	}
	response.Status = customerPriceAvailable
	response.Currency = selected.Currency
	response.PricePerUnit = selected.PricePerUnit.StringFixed(4)
	response.TariffType = string(pricing.tariffType)
	response.PriceType = string(pricing.priceType)
	if pricing.units != nil {
		units := string(*pricing.units)
		response.Units = &units
	}
	response.UnavailableReason = ""
	response.GST = &CustomerGSTView{SGSTRate: gst.SGSTRate.StringFixed(2), CGSTRate: gst.CGSTRate.StringFixed(2), IGSTRate: gst.IGSTRate.StringFixed(2)}
	return response, nil
}

type effectiveTariffTarget struct {
	assignment constants.TariffAssignmentType
	column     string
	id         uuid.UUID
}

func effectiveTariffTargets(userGroupID, chargerID, hubID *uuid.UUID) []effectiveTariffTarget {
	targets := make([]effectiveTariffTarget, 0, 3)
	if userGroupID != nil {
		targets = append(targets, effectiveTariffTarget{assignment: constants.TariffAssignedUserGroup, column: "user_group_id", id: *userGroupID})
	}
	if chargerID != nil {
		targets = append(targets, effectiveTariffTarget{assignment: constants.TariffAssignedCharger, column: "charger_id", id: *chargerID})
	}
	if hubID != nil {
		targets = append(targets, effectiveTariffTarget{assignment: constants.TariffAssignedHub, column: "hub_id", id: *hubID})
	}
	return targets
}

// resolveEffectiveTariff selects the independent commercial tariff. Tax is
// intentionally resolved from the charger's hub by resolveActiveHubGST.
func resolveEffectiveTariff(database *gorm.DB, cpoID uuid.UUID, userGroupID, chargerID, hubID *uuid.UUID, effectiveAt time.Time) (models.Tariff, bool, error) {
	for _, target := range effectiveTariffTargets(userGroupID, chargerID, hubID) {
		var tariffs []models.Tariff
		if err := database.Where(
			"cpo_id = ? AND assigned_to = ? AND "+target.column+" = ? AND is_active = ?",
			cpoID, target.assignment, target.id, true,
		).Find(&tariffs).Error; err != nil {
			return models.Tariff{}, false, err
		}
		projection := make([]commercial.TemporalTariff, 0, len(tariffs))
		for _, tariff := range tariffs {
			projection = append(projection, commercial.TemporalTariff{ID: tariff.ID, IsActive: tariff.IsActive, StartDate: tariff.StartDate, EndDate: tariff.EndDate})
		}
		selectedID, ok, err := commercial.ResolveEnabledTariff(projection, effectiveAt)
		if err != nil {
			return models.Tariff{}, false, err
		}
		if !ok {
			continue
		}
		for _, tariff := range tariffs {
			if tariff.ID == selectedID {
				return tariff, true, nil
			}
		}
		return models.Tariff{}, false, errors.New("selected tariff missing from temporal policy set")
	}
	return models.Tariff{}, false, nil
}

func isTariffTopologyError(err error) bool {
	return errors.Is(err, commercial.ErrTariffTemporalConflict) ||
		errors.Is(err, commercial.ErrInvalidTariffDateShape)
}

// resolveActiveHubGST is the only customer-commercial tax lookup. A missing,
// cross-tenant, inactive, or structurally incomplete hub GST is unavailable;
// it is never interpreted as a zero-rate tax configuration.
func resolveActiveHubGST(database *gorm.DB, cpoID, hubID uuid.UUID) (models.GST, bool, error) {
	var hub models.Hub
	if err := database.First(&hub, "id = ? AND cpo_id = ?", hubID, cpoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.GST{}, false, nil
		}
		return models.GST{}, false, err
	}
	if hub.GSTID == nil {
		return models.GST{}, false, nil
	}
	var gst models.GST
	if err := database.First(&gst, "id = ? AND cpo_id = ? AND is_active = ?", *hub.GSTID, cpoID, true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.GST{}, false, nil
		}
		return models.GST{}, false, err
	}
	if commercial.ValidateHubGST(hub.State, gst.State, gst.SGSTRate, gst.CGSTRate, gst.IGSTRate) != nil {
		return models.GST{}, false, nil
	}
	return gst, true, nil
}
