package invoice

import (
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/shopspring/decimal"
)

func invoiceSessionChargeRows(snapshot issuanceSnapshot) []invoiceChargeRow {
	rows := invoiceChargeRows(snapshot.Commercial)
	rows[0].Basis = invoiceSessionBillingBasis(snapshot.Commercial, snapshot.Charging)
	return rows
}

// A tariff determines what is billed. The customer's requested/effective stop
// limits never supply a billable quantity, even when their dimension matches.
func invoiceSessionBillingBasis(commercial commercialSnapshot, charging chargingSnapshot) string {
	const unavailable = "Billing basis unavailable"
	tariff := commercial.Tariff
	if tariff.TariffType == nil || tariff.PriceType == nil || tariff.PricePerUnit == nil {
		return unavailable
	}
	rate, err := decimal.NewFromString(*tariff.PricePerUnit)
	if err != nil || rate.IsNegative() {
		return unavailable
	}
	tariffType, priceType := constants.TariffType(*tariff.TariffType), constants.PriceType(*tariff.PriceType)
	var unit *constants.Unit
	if tariff.BillingUnit != nil {
		value := constants.Unit(*tariff.BillingUnit)
		unit = &value
	}
	legacyWh := tariffType == constants.TariffTypeFixed && priceType == constants.PriceTypeEnergy && unit != nil && *unit == constants.LegacyUnitWattHour
	if !legacyWh && !constants.SupportedChargingTariff(&tariffType, &priceType, unit) {
		return unavailable
	}
	price := formatMoney(commercial.Currency, *tariff.PricePerUnit)
	switch priceType {
	case constants.PriceTypeEnergy:
		energy, err := decimal.NewFromString(commercial.TotalKWh)
		if err != nil || energy.IsNegative() {
			if legacyWh {
				return "Energy unavailable · " + price + "/Wh"
			}
			return "Energy unavailable · " + price + "/kWh"
		}
		if legacyWh {
			return energy.Mul(decimal.NewFromInt(1000)).String() + " Wh × " + price + "/Wh"
		}
		return commercial.TotalKWh + " kWh × " + price + "/kWh"
	case constants.PriceTypeTime:
		if charging.StartedAt.IsZero() || charging.EndedAt == nil || charging.EndedAt.Before(charging.StartedAt) {
			return "Elapsed time unavailable · " + price + "/min"
		}
		// DurationSeconds and the summary formatter discard sub-second/second
		// precision. Use the same frozen timestamps as final settlement instead.
		elapsed := charging.EndedAt.Sub(charging.StartedAt)
		minutes := elapsed / time.Minute
		seconds := decimal.New(int64(elapsed%time.Minute), -9)
		quantity := fmt.Sprintf("%d min", minutes)
		if !seconds.IsZero() {
			quantity += " " + seconds.String() + " sec"
		}
		return quantity + " at " + price + "/min"
	case constants.PriceTypeSession:
		return "1 session × " + price + "/session"
	default:
		return unavailable
	}
}
