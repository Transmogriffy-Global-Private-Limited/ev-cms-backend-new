package constants

type IndianState string

const (
	AndhraPradesh                      IndianState = "Andhra Pradesh"
	ArunachalPradesh                   IndianState = "Arunachal Pradesh"
	Assam                              IndianState = "Assam"
	Bihar                              IndianState = "Bihar"
	Chhattisgarh                       IndianState = "Chhattisgarh"
	Goa                                IndianState = "Goa"
	Gujarat                            IndianState = "Gujarat"
	Haryana                            IndianState = "Haryana"
	HimachalPradesh                    IndianState = "Himachal Pradesh"
	Jharkhand                          IndianState = "Jharkhand"
	Karnataka                          IndianState = "Karnataka"
	Kerala                             IndianState = "Kerala"
	MadhyaPradesh                      IndianState = "Madhya Pradesh"
	Maharashtra                        IndianState = "Maharashtra"
	Manipur                            IndianState = "Manipur"
	Meghalaya                          IndianState = "Meghalaya"
	Mizoram                            IndianState = "Mizoram"
	Nagaland                           IndianState = "Nagaland"
	Odisha                             IndianState = "Odisha"
	Punjab                             IndianState = "Punjab"
	Rajasthan                          IndianState = "Rajasthan"
	Sikkim                             IndianState = "Sikkim"
	TamilNadu                          IndianState = "Tamil Nadu"
	Telangana                          IndianState = "Telangana"
	Tripura                            IndianState = "Tripura"
	UttarPradesh                       IndianState = "Uttar Pradesh"
	Uttarakhand                        IndianState = "Uttarakhand"
	WestBengal                         IndianState = "West Bengal"
	AndamanAndNicobarIslands           IndianState = "Andaman and Nicobar Islands"
	Chandigarh                         IndianState = "Chandigarh"
	DadraAndNagarHaveliAndDamanAndDiu  IndianState = "Dadra and Nagar Haveli and Daman and Diu"
	Delhi                              IndianState = "Delhi (National Capital Territory of Delhi)"
	JammuAndKashmir                    IndianState = "Jammu and Kashmir"
	Ladakh                             IndianState = "Ladakh"
	Lakshadweep                        IndianState = "Lakshadweep"
	Puducherry                         IndianState = "Puducherry"
)

var allIndianStates = map[IndianState]struct{}{
	AndhraPradesh:                      {},
	ArunachalPradesh:                   {},
	Assam:                              {},
	Bihar:                              {},
	Chhattisgarh:                       {},
	Goa:                                {},
	Gujarat:                            {},
	Haryana:                            {},
	HimachalPradesh:                    {},
	Jharkhand:                          {},
	Karnataka:                          {},
	Kerala:                             {},
	MadhyaPradesh:                      {},
	Maharashtra:                        {},
	Manipur:                            {},
	Meghalaya:                          {},
	Mizoram:                            {},
	Nagaland:                           {},
	Odisha:                             {},
	Punjab:                             {},
	Rajasthan:                          {},
	Sikkim:                             {},
	TamilNadu:                          {},
	Telangana:                          {},
	Tripura:                            {},
	UttarPradesh:                       {},
	Uttarakhand:                        {},
	WestBengal:                         {},
	AndamanAndNicobarIslands:           {},
	Chandigarh:                         {},
	DadraAndNagarHaveliAndDamanAndDiu:  {},
	Delhi:                              {},
	JammuAndKashmir:                    {},
	Ladakh:                             {},
	Lakshadweep:                        {},
	Puducherry:                         {},
}

func (s IndianState) Valid() bool {
	_, ok := allIndianStates[s]
	return ok
}
