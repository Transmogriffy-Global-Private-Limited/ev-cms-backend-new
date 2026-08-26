package constants

// ChargingLimitType records the customer-selected stop boundary frozen with a
// start intent. It is deliberately independent from PriceType: a tariff says
// how a completed session is billed; this value says which bounded execution
// limit the customer asked CMS to derive and send to HAL.
type ChargingLimitType string

const (
	ChargingLimitTypeAuto   ChargingLimitType = "AUTO"
	ChargingLimitTypeEnergy ChargingLimitType = "ENERGY"
	ChargingLimitTypeTime   ChargingLimitType = "TIME"
	ChargingLimitTypeMoney  ChargingLimitType = "MONEY"
)

func (value ChargingLimitType) Valid() bool {
	switch value {
	case ChargingLimitTypeAuto, ChargingLimitTypeEnergy, ChargingLimitTypeTime, ChargingLimitTypeMoney:
		return true
	default:
		return false
	}
}
