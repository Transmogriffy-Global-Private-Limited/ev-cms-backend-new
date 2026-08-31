package customerauth

import (
	"context"
	"fmt"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReconcileStopCommand applies exact HAL command evidence to the CMS session.
// Only rejected or confirmed-absent commands can make a still-live session
// stoppable again; every delivery-ambiguous state remains STOP_PENDING.
func (service *Service) ReconcileStopCommand(ctx context.Context, commandID uuid.UUID, command halops.Command) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "cms_command_id = ? AND kind = ?", commandID, "STOP").Error; err != nil {
			return err
		}
		if record.ChargingSessionID == nil {
			return fmt.Errorf("stop command %s has no charging session", commandID)
		}
		var session models.ChargingSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ?", *record.ChargingSessionID).Error; err != nil {
			return err
		}
		if session.EndTime != nil || session.Status == constants.SessionStatusCompleted {
			return nil
		}
		state := constants.SessionStatusStopPending
		if command.State == "OCPP_REJECTED" {
			state = constants.SessionStatusActive
		}
		if session.Status == state {
			return nil
		}
		if err := tx.Model(&session).Updates(map[string]any{"status": state, "updated_at": service.now()}).Error; err != nil {
			return err
		}
		session.Status = state
		return service.emitChargingSessionChanged(tx, session)
	})
}

// ReconcileConfirmedAbsentStopCommand handles only an exact HAL 404 for the
// known STOP command. It proves the command never became provider state, so
// an incomplete session can safely return to ACTIVE and be stopped again.
func (service *Service) ReconcileConfirmedAbsentStopCommand(ctx context.Context, commandID uuid.UUID) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var command models.HALCommandRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&command, "cms_command_id = ? AND kind = ?", commandID, "STOP").Error; err != nil {
			return err
		}
		if command.ChargingSessionID == nil {
			return fmt.Errorf("stop command %s has no charging session", commandID)
		}
		var session models.ChargingSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ?", *command.ChargingSessionID).Error; err != nil {
			return err
		}
		now := service.now()
		if err := tx.Model(&command).Updates(map[string]any{"state": "CONFIRMED_ABSENT", "last_error_category": "confirmed_absent", "last_error_detail": "HAL exact stop-command lookup confirmed no durable command", "updated_at": now}).Error; err != nil {
			return err
		}
		if session.EndTime != nil || session.Status == constants.SessionStatusCompleted {
			return nil
		}
		if session.Status == constants.SessionStatusActive {
			return nil
		}
		if err := tx.Model(&session).Updates(map[string]any{"status": constants.SessionStatusActive, "updated_at": now}).Error; err != nil {
			return err
		}
		session.Status = constants.SessionStatusActive
		return service.emitChargingSessionChanged(tx, session)
	})
}
