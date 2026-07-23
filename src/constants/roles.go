package constants

type CPORole string

const (
	CPORoleOwner    CPORole = "OWNER"
	CPORoleAdmin    CPORole = "ADMIN"
	CPORoleOperator CPORole = "OPERATOR"
	CPORoleViewer   CPORole = "VIEWER"
)

func (role CPORole) Valid() bool {
	switch role {
	case CPORoleOwner, CPORoleAdmin, CPORoleOperator, CPORoleViewer:
		return true
	default:
		return false
	}
}
