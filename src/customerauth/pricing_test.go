package customerauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEffectiveTariffTargetsUseFixedPrecedence(t *testing.T) {
	t.Parallel()

	hubID := uuid.New()
	chargerID := uuid.New()
	groupID := uuid.New()
	targets := effectiveTariffTargets(&groupID, &chargerID, &hubID)
	if len(targets) != 3 {
		t.Fatalf("target count=%d, want 3", len(targets))
	}
	for index, want := range []struct {
		assignment constants.TariffAssignmentType
		id         uuid.UUID
	}{
		{constants.TariffAssignedUserGroup, groupID},
		{constants.TariffAssignedCharger, chargerID},
		{constants.TariffAssignedHub, hubID},
	} {
		if targets[index].assignment != want.assignment || targets[index].id != want.id {
			t.Fatalf("target[%d]=%+v, want assignment=%q id=%s", index, targets[index], want.assignment, want.id)
		}
	}
}

func TestEffectiveTariffTargetsNeverUseChargerWithoutChargerContext(t *testing.T) {
	t.Parallel()

	hubID := uuid.New()
	groupID := uuid.New()
	targets := effectiveTariffTargets(&groupID, nil, &hubID)
	if len(targets) != 2 || targets[0].assignment != constants.TariffAssignedUserGroup || targets[1].assignment != constants.TariffAssignedHub {
		t.Fatalf("hub-only targets=%+v, want usergroup then hub", targets)
	}
}

func TestResolveEffectiveTariffPreservesTopologyAndInfrastructureErrors(t *testing.T) {
	t.Parallel()

	if !isTariffTopologyError(commercial.ErrTariffTemporalConflict) || !isTariffTopologyError(commercial.ErrInvalidTariffDateShape) {
		t.Fatal("semantic tariff topology errors were not recognized")
	}
	infrastructureErr := errors.New("database unavailable")
	if isTariffTopologyError(infrastructureErr) {
		t.Fatal("infrastructure error was misclassified as tariff topology")
	}
	database, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=unused dbname=unused"}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("construct no-connect test database handle: %v", err)
	}
	database.AddError(infrastructureErr)
	hubID := uuid.New()
	_, found, err := resolveEffectiveTariff(database, uuid.New(), nil, nil, &hubID, time.Now().UTC())
	if found || !errors.Is(err, infrastructureErr) {
		t.Fatalf("resolver error=%v found=%t, want preserved infrastructure error", err, found)
	}
	service := Service{database: database}
	response, err := service.resolveCustomerPrice(context.Background(), Principal{CPOID: uuid.New()}, hubID, nil, time.Now().UTC())
	if !errors.Is(err, infrastructureErr) || response.Status != "" {
		t.Fatalf("customer price response=%+v error=%v, want propagated infrastructure error", response, err)
	}
}

func TestCustomerPriceTreatsNoTariffAsCommercialUnavailability(t *testing.T) {
	t.Parallel()

	database, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=unused dbname=unused"}), &gorm.Config{DisableAutomaticPing: true, DryRun: true})
	if err != nil {
		t.Fatalf("construct no-connect dry-run database handle: %v", err)
	}
	hubID := uuid.New()
	response, err := (&Service{database: database}).resolveCustomerPrice(context.Background(), Principal{CPOID: uuid.New()}, hubID, nil, time.Now().UTC())
	if err != nil || response.Status != customerPriceUnavailable || response.UnavailableReason != "no_eligible_tariff" {
		t.Fatalf("no tariff response=%+v error=%v, want unavailable/no_eligible_tariff", response, err)
	}
}
