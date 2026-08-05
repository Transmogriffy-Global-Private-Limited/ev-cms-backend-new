package customerauth

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestSelectCustomerTariffPrefersUserGroupThenChargerThenHub(t *testing.T) {
	t.Parallel()

	hubID := uuid.New()
	chargerID := uuid.New()
	groupID := uuid.New()

	userCharger := models.Tariff{ID: uuid.New(), HubID: hubID, ChargerID: &chargerID, UserGroupID: &groupID}
	userHub := models.Tariff{ID: uuid.New(), HubID: hubID, UserGroupID: &groupID}
	genericCharger := models.Tariff{ID: uuid.New(), HubID: hubID, ChargerID: &chargerID}
	genericHub := models.Tariff{ID: uuid.New(), HubID: hubID}

	selected, ok := selectCustomerTariff(
		[]models.Tariff{genericHub, genericCharger, userHub},
		&chargerID,
		&groupID,
	)
	if !ok || selected.ID != userHub.ID {
		t.Fatalf("selected tariff=%s, want UserGroup tariff %s over generic charger", selected.ID, userHub.ID)
	}

	selected, ok = selectCustomerTariff(
		[]models.Tariff{genericHub, userHub, userCharger},
		&chargerID,
		&groupID,
	)
	if !ok || selected.ID != userCharger.ID {
		t.Fatalf("selected tariff=%s, want more-specific UserGroup charger tariff %s", selected.ID, userCharger.ID)
	}

	selected, ok = selectCustomerTariff(
		[]models.Tariff{genericHub, genericCharger},
		&chargerID,
		nil,
	)
	if !ok || selected.ID != genericCharger.ID {
		t.Fatalf("selected tariff=%s, want generic charger tariff %s", selected.ID, genericCharger.ID)
	}

	selected, ok = selectCustomerTariff([]models.Tariff{genericHub}, &chargerID, nil)
	if !ok || selected.ID != genericHub.ID {
		t.Fatalf("selected tariff=%s, want generic hub tariff %s", selected.ID, genericHub.ID)
	}
}
