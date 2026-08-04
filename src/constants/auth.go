package constants

type AuthScope string

const (
	AuthScopePlatform AuthScope = "PLATFORM"
	AuthScopeCPO      AuthScope = "CPO"
	AuthScopeCustomer AuthScope = "CUSTOMER"
)

func (scope AuthScope) Valid() bool {
	switch scope {
	case AuthScopePlatform, AuthScopeCPO, AuthScopeCustomer:
		return true
	default:
		return false
	}
}

type AuthChallengePurpose string

const (
	ChallengeLogin2FA      AuthChallengePurpose = "LOGIN_2FA"
	ChallengePasswordReset AuthChallengePurpose = "PASSWORD_RESET"
	ChallengeCustomerLogin AuthChallengePurpose = "CUSTOMER_LOGIN_2FA"
	ChallengeCustomerReset AuthChallengePurpose = "CUSTOMER_PASSWORD_RESET"
)

type MailOutboxStatus string

const (
	MailOutboxPending    MailOutboxStatus = "PENDING"
	MailOutboxProcessing MailOutboxStatus = "PROCESSING"
	MailOutboxSent       MailOutboxStatus = "SENT"
	MailOutboxFailed     MailOutboxStatus = "FAILED"
	MailOutboxCanceled   MailOutboxStatus = "CANCELED"
)

const IntegrationProviderRazorpay = "RAZORPAY"
