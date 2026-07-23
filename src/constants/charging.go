package constants

type ChargerStatus string

const (
	ChargerStatusAvailable     ChargerStatus = "AVAILABLE"
	ChargerStatusPreparing     ChargerStatus = "PREPARING"
	ChargerStatusCharging      ChargerStatus = "CHARGING"
	ChargerStatusSuspendedEV   ChargerStatus = "SUSPENDED_EV"
	ChargerStatusSuspendedEVSE ChargerStatus = "SUSPENDED_EVSE"
	ChargerStatusFinishing     ChargerStatus = "FINISHING"
	ChargerStatusReserved      ChargerStatus = "RESERVED"
	ChargerStatusUnavailable   ChargerStatus = "UNAVAILABLE"
	ChargerStatusFaulted       ChargerStatus = "FAULTED"
	ChargerStatusOffline       ChargerStatus = "OFFLINE"
)

func (status ChargerStatus) Valid() bool {
	switch status {
	case ChargerStatusAvailable,
		ChargerStatusPreparing,
		ChargerStatusCharging,
		ChargerStatusSuspendedEV,
		ChargerStatusSuspendedEVSE,
		ChargerStatusFinishing,
		ChargerStatusReserved,
		ChargerStatusUnavailable,
		ChargerStatusFaulted,
		ChargerStatusOffline:
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
