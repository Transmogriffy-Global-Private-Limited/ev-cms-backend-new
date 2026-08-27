package customerauth

import (
	"errors"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/shopspring/decimal"
)

var errUnsupportedTariffSemantics = errors.New("unsupported tariff semantics")

// tariffPricing is the single commercial interpretation shared by admission
// and settlement. The legacy flags are intentionally limited to immutable
// snapshot formats released before the canonical per-kWh contract.
type tariffPricing struct {
	pricePerUnit decimal.Decimal
	tariffType   constants.TariffType
	priceType    constants.PriceType
	units        *constants.Unit
	legacyPerKWh bool
	legacyPerWh  bool
}

type tariffAffordableBoundKind string

const (
	tariffAffordableEnergy tariffAffordableBoundKind = "ENERGY_BOUND"
	tariffAffordableTime   tariffAffordableBoundKind = "TIME_BOUND"
	tariffAffordableFixed  tariffAffordableBoundKind = "FIXED_CHARGE"
	tariffAffordableNone   tariffAffordableBoundKind = "NONE"
)

// tariffAffordableBound is the tariff-owned inversion of the same immutable
// pricing calculation used at settlement. It deliberately exposes only the
// tariff's billed dimension: admission must never predict kWh from time or
// time from kWh using charger power.
type tariffAffordableBound struct {
	Kind            tariffAffordableBoundKind
	EnergyLimitWh   int64
	DurationSeconds int64
	Amount          decimal.Decimal
}

func tariffPricingFromTariff(tariff models.Tariff) (tariffPricing, error) {
	if !constants.SupportedChargingTariff(tariff.TariffType, tariff.PriceType, tariff.Units) {
		return tariffPricing{}, errUnsupportedTariffSemantics
	}
	if !tariff.IdleFeePerMin.IsZero() {
		return tariffPricing{}, errUnsupportedTariffSemantics
	}
	return tariffPricing{
		pricePerUnit: tariff.PricePerUnit,
		tariffType:   *tariff.TariffType,
		priceType:    *tariff.PriceType,
		units:        tariff.Units,
	}, nil
}

func tariffPricingFromSnapshot(snapshot models.JSONB) (tariffPricing, error) {
	if _, hasCanonicalPrice := snapshot["price_per_unit"]; hasCanonicalPrice {
		price, ok := snapshotDecimalValue(snapshot, "price_per_unit")
		if !ok {
			return tariffPricing{}, errUnsupportedTariffSemantics
		}
		tariffType := constants.TariffType(snapshotString(snapshot, "tariff_type", ""))
		priceType := constants.PriceType(snapshotString(snapshot, "price_type", ""))
		var units *constants.Unit
		if rawUnits := snapshotString(snapshot, "units", ""); rawUnits != "" {
			value := constants.Unit(rawUnits)
			units = &value
		}
		// This was the short-lived migration-40 contract. It was priced as per
		// Wh by the released code, so historical completion keeps that exact
		// interpretation instead of silently treating it as the corrected kWh.
		if tariffType == constants.TariffTypeFixed && priceType == constants.PriceTypeEnergy && units != nil && *units == constants.LegacyUnitWattHour {
			return tariffPricing{pricePerUnit: price, tariffType: tariffType, priceType: priceType, units: units, legacyPerWh: true}, nil
		}
		if !constants.SupportedChargingTariff(&tariffType, &priceType, units) {
			return tariffPricing{}, errUnsupportedTariffSemantics
		}
		return tariffPricing{pricePerUnit: price, tariffType: tariffType, priceType: priceType, units: units}, nil
	}

	// Historical start intents and sessions only stored this field. It is not a
	// new tariff interpretation: it preserves their established per-kWh amount.
	if price, ok := snapshotDecimalValue(snapshot, "price_per_kwh"); ok {
		return tariffPricing{pricePerUnit: price, tariffType: constants.TariffTypeFixed, priceType: constants.PriceTypeEnergy, legacyPerKWh: true}, nil
	}
	return tariffPricing{}, errUnsupportedTariffSemantics
}

func (pricing tariffPricing) baseAmount(consumedWh int64, startedAt, stoppedAt time.Time) (decimal.Decimal, error) {
	if consumedWh < 0 || stoppedAt.Before(startedAt) {
		return decimal.Zero, errUnsupportedTariffSemantics
	}
	if pricing.legacyPerKWh {
		return pricing.pricePerUnit.Mul(decimal.NewFromInt(consumedWh)).Div(decimal.NewFromInt(1000)), nil
	}
	if pricing.legacyPerWh {
		return pricing.pricePerUnit.Mul(decimal.NewFromInt(consumedWh)), nil
	}
	switch pricing.priceType {
	case constants.PriceTypeEnergy:
		return pricing.pricePerUnit.Mul(decimal.NewFromInt(consumedWh)).Div(decimal.NewFromInt(1000)), nil
	case constants.PriceTypeTime:
		billableMinutes := decimal.NewFromInt(stoppedAt.Sub(startedAt).Nanoseconds()).Div(decimal.NewFromInt(int64(time.Minute)))
		return pricing.pricePerUnit.Mul(billableMinutes), nil
	case constants.PriceTypeSession:
		return pricing.pricePerUnit, nil
	default:
		return decimal.Zero, errUnsupportedTariffSemantics
	}
}

func (pricing tariffPricing) amountWithGST(consumedWh int64, startedAt, stoppedAt time.Time, gst models.GST) (decimal.Decimal, error) {
	baseAmount, err := pricing.baseAmount(consumedWh, startedAt, stoppedAt)
	if err != nil {
		return decimal.Zero, err
	}
	return applyGST(baseAmount, gst)
}

func (pricing tariffPricing) AffordableBound(budget decimal.Decimal, gst models.GST) (tariffAffordableBound, error) {
	if budget.LessThan(decimal.Zero) {
		return tariffAffordableBound{}, errUnsupportedTariffSemantics
	}
	switch pricing.priceType {
	case constants.PriceTypeEnergy:
		if pricing.pricePerUnit.IsZero() {
			return tariffAffordableBound{Kind: tariffAffordableNone}, nil
		}
		multiplier, err := gstMultiplier(gst)
		if err != nil {
			return tariffAffordableBound{}, err
		}
		limitWh := budget.Div(pricing.pricePerUnit.Mul(multiplier)).Mul(decimal.NewFromInt(1000)).Floor().IntPart()
		for limitWh > 0 {
			amount, err := pricing.amountWithGST(limitWh, time.Time{}, time.Time{}, gst)
			if err == nil && !amount.GreaterThan(budget) {
				return tariffAffordableBound{Kind: tariffAffordableEnergy, EnergyLimitWh: limitWh, Amount: amount}, nil
			}
			limitWh--
		}
		return tariffAffordableBound{}, errChargingLimitUnaffordable
	case constants.PriceTypeTime:
		if pricing.pricePerUnit.IsZero() {
			return tariffAffordableBound{Kind: tariffAffordableNone}, nil
		}
		multiplier, err := gstMultiplier(gst)
		if err != nil {
			return tariffAffordableBound{}, err
		}
		duration := budget.Div(pricing.pricePerUnit.Mul(multiplier)).Mul(decimal.NewFromInt(60)).Floor().IntPart()
		maxSeconds := int64((time.Duration(1<<63 - 1)) / time.Second)
		if duration > maxSeconds {
			duration = maxSeconds
		}
		for duration > 0 {
			amount, err := pricing.amountWithGST(0, time.Time{}, time.Time{}.Add(time.Duration(duration)*time.Second), gst)
			if err == nil && !amount.GreaterThan(budget) {
				return tariffAffordableBound{Kind: tariffAffordableTime, DurationSeconds: duration, Amount: amount}, nil
			}
			duration--
		}
		return tariffAffordableBound{}, errChargingLimitUnaffordable
	case constants.PriceTypeSession:
		amount, err := pricing.amountWithGST(0, time.Time{}, time.Time{}, gst)
		if err != nil {
			return tariffAffordableBound{}, err
		}
		if amount.GreaterThan(budget) {
			return tariffAffordableBound{}, errChargingLimitUnaffordable
		}
		return tariffAffordableBound{Kind: tariffAffordableFixed, Amount: amount}, nil
	default:
		return tariffAffordableBound{}, errUnsupportedTariffSemantics
	}
}

func (pricing tariffPricing) snapshot(tariff models.Tariff) models.JSONB {
	snapshot := models.JSONB{
		"tariff_id":        tariff.ID.String(),
		"currency":         tariff.Currency,
		"price_per_unit":   pricing.pricePerUnit.String(),
		"tariff_type":      string(pricing.tariffType),
		"price_type":       string(pricing.priceType),
		"idle_fee_per_min": tariff.IdleFeePerMin.String(),
	}
	if pricing.units != nil {
		snapshot["units"] = string(*pricing.units)
	}
	return snapshot
}

func applyGST(baseAmount decimal.Decimal, gst models.GST) (decimal.Decimal, error) {
	multiplier, err := gstMultiplier(gst)
	if err != nil {
		return decimal.Zero, err
	}
	return baseAmount.Mul(multiplier).Round(2), nil
}

func gstMultiplier(gst models.GST) (decimal.Decimal, error) {
	if err := commercial.ValidateGSTComponents(gst.SGSTRate, gst.CGSTRate, gst.IGSTRate); err != nil {
		return decimal.Zero, errUnsupportedTariffSemantics
	}
	return commercial.GSTMultiplier(*gst.SGSTRate, *gst.CGSTRate, *gst.IGSTRate), nil
}

func (pricing tariffPricing) amountWithTaxSnapshot(consumedWh int64, startedAt, stoppedAt time.Time, tax models.JSONB) (decimal.Decimal, error) {
	baseAmount, err := pricing.baseAmount(consumedWh, startedAt, stoppedAt)
	if err != nil {
		return decimal.Zero, err
	}
	sgst, sgstOK := snapshotDecimalValue(tax, "sgst_rate")
	cgst, cgstOK := snapshotDecimalValue(tax, "cgst_rate")
	igst, igstOK := snapshotDecimalValue(tax, "igst_rate")
	if !sgstOK || !cgstOK || !igstOK {
		return decimal.Zero, errUnsupportedTariffSemantics
	}
	if err := commercial.ValidateGSTComponents(&sgst, &cgst, &igst); err != nil {
		return decimal.Zero, errUnsupportedTariffSemantics
	}
	return baseAmount.Mul(commercial.GSTMultiplier(sgst, cgst, igst)).Round(2), nil
}

func snapshotDecimalValue(snapshot models.JSONB, key string) (decimal.Decimal, bool) {
	value, ok := snapshot[key].(string)
	if !ok || value == "" {
		return decimal.Zero, false
	}
	parsed, err := decimal.NewFromString(value)
	return parsed, err == nil
}
