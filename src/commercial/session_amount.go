package commercial

import (
	"errors"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/shopspring/decimal"
)

var ErrUnsupportedSessionPricing = errors.New("unsupported session pricing semantics")

type sessionSnapshotPricing struct {
	pricePerUnit decimal.Decimal
	tariffType   constants.TariffType
	priceType    constants.PriceType
	units        *constants.Unit
	legacyPerKWh bool
	legacyPerWh  bool
}

// SessionAmountFromSnapshots evaluates the immutable tariff/tax snapshot for a
// charging session at a concrete point in time.
//
// Both live monetary projections and final settlement use this function so
// their pricing semantics cannot drift apart.
func SessionAmountFromSnapshots(
	tariff models.JSONB,
	tax models.JSONB,
	consumedWh int64,
	startedAt time.Time,
	projectedAt time.Time,
) (decimal.Decimal, error) {
	pricing, err := sessionSnapshotPricingFromSnapshot(tariff)
	if err != nil {
		return decimal.Zero, err
	}

	baseAmount, err := pricing.baseAmount(consumedWh, startedAt, projectedAt)
	if err != nil {
		return decimal.Zero, err
	}

	sgst, sgstOK := sessionSnapshotDecimal(tax, "sgst_rate")
	cgst, cgstOK := sessionSnapshotDecimal(tax, "cgst_rate")
	igst, igstOK := sessionSnapshotDecimal(tax, "igst_rate")

	if !sgstOK || !cgstOK || !igstOK ||
		ValidateGSTComponents(&sgst, &cgst, &igst) != nil {
		return decimal.Zero, ErrUnsupportedSessionPricing
	}

	return baseAmount.Mul(GSTMultiplier(sgst, cgst, igst)).Round(2), nil
}

func sessionSnapshotPricingFromSnapshot(
	snapshot models.JSONB,
) (sessionSnapshotPricing, error) {
	if _, canonical := snapshot["price_per_unit"]; canonical {
		price, ok := sessionSnapshotDecimal(snapshot, "price_per_unit")
		if !ok {
			return sessionSnapshotPricing{}, ErrUnsupportedSessionPricing
		}

		tariffType := constants.TariffType(
			sessionSnapshotString(snapshot, "tariff_type"),
		)
		priceType := constants.PriceType(
			sessionSnapshotString(snapshot, "price_type"),
		)

		var units *constants.Unit
		if raw := sessionSnapshotString(snapshot, "units"); raw != "" {
			value := constants.Unit(raw)
			units = &value
		}

		// Preserve the short-lived historical per-Wh snapshot representation.
		if tariffType == constants.TariffTypeFixed &&
			priceType == constants.PriceTypeEnergy &&
			units != nil &&
			*units == constants.LegacyUnitWattHour {
			return sessionSnapshotPricing{
				pricePerUnit: price,
				tariffType:   tariffType,
				priceType:    priceType,
				units:        units,
				legacyPerWh:  true,
			}, nil
		}

		if !constants.SupportedChargingTariff(
			&tariffType,
			&priceType,
			units,
		) {
			return sessionSnapshotPricing{}, ErrUnsupportedSessionPricing
		}

		return sessionSnapshotPricing{
			pricePerUnit: price,
			tariffType:   tariffType,
			priceType:    priceType,
			units:        units,
		}, nil
	}

	// Historical snapshots used an explicit price_per_kwh property.
	if price, ok := sessionSnapshotDecimal(snapshot, "price_per_kwh"); ok {
		return sessionSnapshotPricing{
			pricePerUnit: price,
			tariffType:   constants.TariffTypeFixed,
			priceType:    constants.PriceTypeEnergy,
			legacyPerKWh: true,
		}, nil
	}

	return sessionSnapshotPricing{}, ErrUnsupportedSessionPricing
}

func (pricing sessionSnapshotPricing) baseAmount(
	consumedWh int64,
	startedAt time.Time,
	projectedAt time.Time,
) (decimal.Decimal, error) {
	if consumedWh < 0 || projectedAt.Before(startedAt) {
		return decimal.Zero, ErrUnsupportedSessionPricing
	}

	if pricing.legacyPerKWh {
		return pricing.pricePerUnit.
			Mul(decimal.NewFromInt(consumedWh)).
			Div(decimal.NewFromInt(1000)), nil
	}

	if pricing.legacyPerWh {
		return pricing.pricePerUnit.
			Mul(decimal.NewFromInt(consumedWh)), nil
	}

	switch pricing.priceType {
	case constants.PriceTypeEnergy:
		return pricing.pricePerUnit.
			Mul(decimal.NewFromInt(consumedWh)).
			Div(decimal.NewFromInt(1000)), nil

	case constants.PriceTypeTime:
		minutes := decimal.NewFromInt(
			projectedAt.Sub(startedAt).Nanoseconds(),
		).Div(decimal.NewFromInt(int64(time.Minute)))

		return pricing.pricePerUnit.Mul(minutes), nil

	case constants.PriceTypeSession:
		return pricing.pricePerUnit, nil

	default:
		return decimal.Zero, ErrUnsupportedSessionPricing
	}
}

func sessionSnapshotDecimal(
	snapshot models.JSONB,
	key string,
) (decimal.Decimal, bool) {
	value, ok := snapshot[key].(string)
	if !ok || value == "" {
		return decimal.Zero, false
	}

	parsed, err := decimal.NewFromString(value)
	return parsed, err == nil
}

func sessionSnapshotString(snapshot models.JSONB, key string) string {
	value, _ := snapshot[key].(string)
	return value
}
