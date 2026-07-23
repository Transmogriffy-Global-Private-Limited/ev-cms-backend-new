package constants

type CPOCompanyType string

const (
	CPOCompanyTypeIndividual CPOCompanyType = "INDIVIDUAL"
	CPOCompanyTypeCompany    CPOCompanyType = "COMPANY"
)

func (companyType CPOCompanyType) Valid() bool {
	switch companyType {
	case CPOCompanyTypeIndividual, CPOCompanyTypeCompany:
		return true
	default:
		return false
	}
}
