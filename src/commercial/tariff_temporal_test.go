package commercial

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestResolveEnabledTariffUsesTemporalFallbackHierarchy(t *testing.T) {
	t.Parallel()
	root, baseline2, baseline3, festival, flash := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	aug1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sep1 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	aug10 := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	aug12 := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	aug15 := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	aug20 := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	tariffs := []TemporalTariff{
		{ID: root, IsActive: true},
		{ID: baseline2, IsActive: true, StartDate: &aug1},
		{ID: baseline3, IsActive: true, StartDate: &sep1},
		{ID: festival, IsActive: true, StartDate: &aug10, EndDate: &aug20},
		{ID: flash, IsActive: true, StartDate: &aug12, EndDate: &aug15},
	}
	for _, test := range []struct {
		at   time.Time
		want uuid.UUID
	}{
		{at: aug1.Add(-time.Nanosecond), want: root},
		{at: aug1, want: baseline2},
		{at: aug10.Add(time.Hour), want: festival},
		{at: aug12.Add(time.Hour), want: flash},
		{at: aug15, want: festival},
		{at: aug20, want: baseline2},
		{at: sep1, want: baseline3},
	} {
		got, ok, err := ResolveEnabledTariff(tariffs, test.at)
		if err != nil || !ok || got != test.want {
			t.Fatalf("resolve at %s = %s/%t/%v, want %s", test.at, got, ok, err, test.want)
		}
	}
}

func TestValidateEnabledTariffTopologyRejectsAmbiguity(t *testing.T) {
	t.Parallel()
	timeAt := func(day int) *time.Time { value := time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC); return &value }
	for _, tariffs := range [][]TemporalTariff{
		{{ID: uuid.New(), IsActive: true}, {ID: uuid.New(), IsActive: true}},
		{{ID: uuid.New(), IsActive: true, StartDate: timeAt(1)}, {ID: uuid.New(), IsActive: true, StartDate: timeAt(1)}},
		{{ID: uuid.New(), IsActive: true, StartDate: timeAt(1), EndDate: timeAt(10)}, {ID: uuid.New(), IsActive: true, StartDate: timeAt(5), EndDate: timeAt(15)}},
		{{ID: uuid.New(), IsActive: true, StartDate: timeAt(1), EndDate: timeAt(10)}, {ID: uuid.New(), IsActive: true, StartDate: timeAt(1), EndDate: timeAt(10)}},
	} {
		if err := ValidateEnabledTariffTopology(tariffs); !errors.Is(err, ErrTariffTemporalConflict) {
			t.Fatalf("topology error=%v, want conflict", err)
		}
	}
	if err := ValidateTariffDateShape(nil, timeAt(10)); !errors.Is(err, ErrInvalidTariffDateShape) {
		t.Fatalf("end-only shape error=%v, want invalid date shape", err)
	}
}
