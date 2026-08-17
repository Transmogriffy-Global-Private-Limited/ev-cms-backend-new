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

func (e TariffType) Valid() bool {
	switch e {
	case TariffTypeFixed:
		return true
	default:
		return false
	}
}

type PriceType string

const (
	PriceTypeSession PriceType = "sessions"
	PriceTypeTime    PriceType = "time"
	PriceTypeEnergy  PriceType = "energy"
)

func (e PriceType) Valid() bool {
	switch e {
	case PriceTypeSession, PriceTypeTime, PriceTypeEnergy:
		return true
	default:
		return false
	}
}

type Unit string

const (
	UnitMinutes  Unit = "minutes"
	UnitWattHour Unit = "watt/hour"
)

func (e Unit) Valid() bool {
	switch e {
	case UnitMinutes, UnitWattHour:
		return true
	default:
		return false
	}
}
