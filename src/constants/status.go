package constants

type CPOStatus string

const (
	CPOStatusPending   CPOStatus = "PENDING"
	CPOStatusActive    CPOStatus = "ACTIVE"
	CPOStatusSuspended CPOStatus = "SUSPENDED"
)

func (status CPOStatus) Valid() bool {
	switch status {
	case CPOStatusPending, CPOStatusActive, CPOStatusSuspended:
		return true
	default:
		return false
	}
}

type MembershipStatus string

const (
	MembershipStatusActive    MembershipStatus = "ACTIVE"
	MembershipStatusSuspended MembershipStatus = "SUSPENDED"
	MembershipStatusRevoked   MembershipStatus = "REVOKED"
)

func (status MembershipStatus) Valid() bool {
	switch status {
	case MembershipStatusActive, MembershipStatusSuspended, MembershipStatusRevoked:
		return true
	default:
		return false
	}
}

type CustomerStatus string

const (
	CustomerStatusActive  CustomerStatus = "ACTIVE"
	CustomerStatusBlocked CustomerStatus = "BLOCKED"
)

func (status CustomerStatus) Valid() bool {
	switch status {
	case CustomerStatusActive, CustomerStatusBlocked:
		return true
	default:
		return false
	}
}
