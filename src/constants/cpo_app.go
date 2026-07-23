package constants

type CPOAppIDMode string

const (
	CPOAppIDModeDummy CPOAppIDMode = "DUMMY"
	CPOAppIDModeLive  CPOAppIDMode = "LIVE"
)

func (mode CPOAppIDMode) Valid() bool {
	switch mode {
	case CPOAppIDModeDummy, CPOAppIDModeLive:
		return true
	default:
		return false
	}
}
