package constants

type TariffType string

const (
	TariffTypeFixed TariffType = "fixed"
)

type PriceType string

const (
	PriceTypeSession PriceType = "sessions"
	PriceTypeTime    PriceType = "time"
	PriceTypeEnergy  PriceType = "energy"
)

type Unit string

const (
	UnitMinutes  Unit = "minutes"
	UnitWattHour Unit = "watt/hour"
)
