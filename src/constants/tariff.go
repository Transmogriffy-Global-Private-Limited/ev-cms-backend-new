package constants

type TariffAssignmentType string

// TariffAssignedType remains a source-compatible alias for the short-lived
// initial name introduced with migration 34.
type TariffAssignedType = TariffAssignmentType

const (
	TariffAssignedUserGroup TariffAssignmentType = "usergroup"
	TariffAssignedHub       TariffAssignmentType = "hub"
	TariffAssignedCharger   TariffAssignmentType = "charger"
)

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
