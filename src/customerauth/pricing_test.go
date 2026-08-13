package customerauth

import (
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
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
