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
