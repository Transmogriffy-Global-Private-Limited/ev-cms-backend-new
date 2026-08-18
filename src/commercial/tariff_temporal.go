package commercial

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTariffDateShape = errors.New("invalid tariff date shape")
	ErrTariffTemporalConflict = errors.New("ambiguous tariff temporal topology")
)

// TemporalTariff is the minimum immutable-policy projection needed to reason
// about one exact target's enabled temporal hierarchy.
type TemporalTariff struct {
	ID        uuid.UUID
	IsActive  bool
	StartDate *time.Time
	EndDate   *time.Time
}

func ValidateTariffDateShape(startDate, endDate *time.Time) error {
	if startDate == nil && endDate != nil {
		return ErrInvalidTariffDateShape
	}
	if startDate != nil && endDate != nil && !startDate.Before(*endDate) {
		return ErrInvalidTariffDateShape
	}
	return nil
}

// ValidateEnabledTariffTopology rejects ambiguity among enabled rows for one
// exact target. Root and open-ended fallbacks can coexist with bounded
// overrides; bounded overlaps must be strict containment, never crossing or
// identical intervals.
func ValidateEnabledTariffTopology(tariffs []TemporalTariff) error {
	var roots []TemporalTariff
	openStarts := make([]time.Time, 0, len(tariffs))
	bounded := make([]TemporalTariff, 0, len(tariffs))
	for _, tariff := range tariffs {
		if !tariff.IsActive {
			continue
		}
		if err := ValidateTariffDateShape(tariff.StartDate, tariff.EndDate); err != nil {
			return err
		}
		switch {
		case tariff.StartDate == nil:
			roots = append(roots, tariff)
		case tariff.EndDate == nil:
			for _, existingStart := range openStarts {
				if tariff.StartDate.Equal(existingStart) {
					return ErrTariffTemporalConflict
				}
			}
			openStarts = append(openStarts, *tariff.StartDate)
		default:
			bounded = append(bounded, tariff)
		}
	}
	if len(roots) > 1 {
		return ErrTariffTemporalConflict
	}
	for index, first := range bounded {
		for _, second := range bounded[index+1:] {
			if !boundedOverlap(first, second) {
				continue
			}
			if !strictlyContains(first, second) && !strictlyContains(second, first) {
				return ErrTariffTemporalConflict
			}
		}
	}
	return nil
}

// ResolveEnabledTariff applies one exact target's temporal precedence at an
// instant: deepest bounded override, newest eligible open-ended fallback, then
// root. Callers must query only one exact target and compose scope precedence.
func ResolveEnabledTariff(tariffs []TemporalTariff, at time.Time) (uuid.UUID, bool, error) {
	if err := ValidateEnabledTariffTopology(tariffs); err != nil {
		return uuid.Nil, false, err
	}
	var bounded *TemporalTariff
	var open *TemporalTariff
	var root *TemporalTariff
	for _, tariff := range tariffs {
		if !tariff.IsActive {
			continue
		}
		switch {
		case tariff.StartDate != nil && tariff.EndDate != nil && !at.Before(*tariff.StartDate) && at.Before(*tariff.EndDate):
			if bounded == nil || tariff.StartDate.After(*bounded.StartDate) || (tariff.StartDate.Equal(*bounded.StartDate) && tariff.EndDate.Before(*bounded.EndDate)) {
				candidate := tariff
				bounded = &candidate
			}
		case tariff.StartDate != nil && tariff.EndDate == nil && !at.Before(*tariff.StartDate):
			if open == nil || tariff.StartDate.After(*open.StartDate) {
				candidate := tariff
				open = &candidate
			}
		case tariff.StartDate == nil && tariff.EndDate == nil:
			candidate := tariff
			root = &candidate
		}
	}
	if bounded != nil {
		return bounded.ID, true, nil
	}
	if open != nil {
		return open.ID, true, nil
	}
	if root != nil {
		return root.ID, true, nil
	}
	return uuid.Nil, false, nil
}

func boundedOverlap(first, second TemporalTariff) bool {
	return first.StartDate.Before(*second.EndDate) && second.StartDate.Before(*first.EndDate)
}

func strictlyContains(outer, inner TemporalTariff) bool {
	return !outer.StartDate.After(*inner.StartDate) && !outer.EndDate.Before(*inner.EndDate) &&
		(outer.StartDate.Before(*inner.StartDate) || inner.EndDate.Before(*outer.EndDate))
}
