package constants

type ChargerStatus string

const (
	ChargerStatusActive           ChargerStatus = "ACTIVE"
	ChargerStatusInactive         ChargerStatus = "INACTIVE"
	ChargerStatusSuspended        ChargerStatus = "SUSPENDED"
	ChargerStatusUnderMaintenance ChargerStatus = "UNDERMAINTENANCE"
	ChargerStatusDecommissioned   ChargerStatus = "DECOMMISSIONED"
)

func (status ChargerStatus) Valid() bool {
	switch status {
	case ChargerStatusActive,
		ChargerStatusInactive,
		ChargerStatusSuspended,
		ChargerStatusUnderMaintenance,
		ChargerStatusDecommissioned:
		return true
	default:
		return false
	}
}

type SessionStatus string

const (
	SessionStatusStartPending SessionStatus = "START_PENDING"
	SessionStatusActive       SessionStatus = "ACTIVE"
	SessionStatusStopPending  SessionStatus = "STOP_PENDING"
	SessionStatusCompleted    SessionStatus = "COMPLETED"
	SessionStatusFailed       SessionStatus = "FAILED"
)

func (status SessionStatus) Valid() bool {
	switch status {
	case SessionStatusStartPending,
		SessionStatusActive,
		SessionStatusStopPending,
		SessionStatusCompleted,
		SessionStatusFailed:
		return true
	default:
		return false
	}
}

// StartIntentStatus records CMS business progress separately from OCPP truth.
// ACTUALLY_STARTED is set only from a validated charger-originated HAL fact.
type StartIntentStatus string

const (
	StartIntentStatusRequested            StartIntentStatus = "REQUESTED"
	StartIntentStatusAcceptedForDelivery  StartIntentStatus = "ACCEPTED_FOR_DELIVERY"
	StartIntentStatusProtocolAcknowledged StartIntentStatus = "PROTOCOL_ACKNOWLEDGED"
	StartIntentStatusActuallyStarted      StartIntentStatus = "ACTUALLY_STARTED"
	StartIntentStatusRejected             StartIntentStatus = "REJECTED"
	StartIntentStatusExpired              StartIntentStatus = "EXPIRED"
	StartIntentStatusReconciliation       StartIntentStatus = "RECONCILIATION_REQUIRED"
)

type WalletHoldStatus string

const (
	WalletHoldStatusHeld        WalletHoldStatus = "HELD"
	WalletHoldStatusCaptured    WalletHoldStatus = "CAPTURED"
	WalletHoldStatusReleased    WalletHoldStatus = "RELEASED"
	WalletHoldStatusReconciling WalletHoldStatus = "RECONCILIATION_REQUIRED"
)
