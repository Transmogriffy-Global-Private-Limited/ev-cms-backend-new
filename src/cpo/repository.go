package cpo

import (
	"context"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository interface {
	GetChargingSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error)
	ListChargingSessions(ctx context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) GetChargingSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error) {
	var session models.ChargingSession
	if err := r.db.WithContext(ctx).Where("cpo_id = ? AND id = ?", cpoID, sessionID).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *repository) ListChargingSessions(ctx context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error) {
	var sessions []models.ChargingSession
	db := r.db.WithContext(ctx).Where("cpo_id = ?", cpoID)

	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.ChargerID != nil {
		db = db.Where("charger_id = ?", *query.ChargerID)
	}
	if query.CustomerID != nil {
		db = db.Where("customer_id = ?", *query.CustomerID)
	}

	if query.Before != nil {
		if query.BeforeID != nil {
			db = db.Where("(created_at, id) < (?, ?)", *query.Before, *query.BeforeID)
		} else {
			db = db.Where("created_at < ?", *query.Before)
		}
	}

	if query.Limit > 0 {
		db = db.Limit(query.Limit + 1)
	}

	if err := db.Order("created_at DESC, id DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}

	return sessions, nil
}
