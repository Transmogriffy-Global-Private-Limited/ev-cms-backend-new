package cpo

import (
	"context"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Repository interface {
	GetAnalytics(ctx context.Context, cpoID uuid.UUID) (Analytics, error)
	GetChargingSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error)
	ListChargingSessions(ctx context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error)
	ListChargersByHub(ctx context.Context, cpoID, hubID uuid.UUID) ([]models.Charger, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

type Analytics struct {
	TotalChargers   int64
	TotalConnectors int64
	TotalRevenue    decimal.Decimal
	TotalUsage      decimal.Decimal
	TotalSessions   int64
}

func (r *repository) GetAnalytics(ctx context.Context, cpoID uuid.UUID) (Analytics, error) {
	var analytics Analytics
	if err := r.db.WithContext(ctx).Model(&models.Charger{}).Where("cpo_id = ?", cpoID).Count(&analytics.TotalChargers).Error; err != nil {
		return Analytics{}, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Connector{}).Where("cpo_id = ?", cpoID).Count(&analytics.TotalConnectors).Error; err != nil {
		return Analytics{}, err
	}

	var sessionAnalytics struct {
		TotalRevenue  decimal.Decimal
		TotalUsage    decimal.Decimal
		TotalSessions int64
	}
	if err := r.db.WithContext(ctx).Model(&models.ChargingSession{}).
		Select("COALESCE(SUM(total_amount), 0) as total_revenue, COALESCE(SUM(total_kwh), 0) as total_usage, COUNT(*) as total_sessions").
		Where("cpo_id = ?", cpoID).
		Scan(&sessionAnalytics).Error; err != nil {
		return Analytics{}, err
	}
	analytics.TotalRevenue = sessionAnalytics.TotalRevenue
	analytics.TotalUsage = sessionAnalytics.TotalUsage
	analytics.TotalSessions = sessionAnalytics.TotalSessions

	return analytics, nil
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

func (r *repository) ListChargersByHub(ctx context.Context, cpoID, hubID uuid.UUID) ([]models.Charger, error) {
	var chargers []models.Charger
	if err := r.db.WithContext(ctx).Where("cpo_id = ? AND hub_id = ?", cpoID, hubID).Find(&chargers).Error; err != nil {
		return nil, err
	}
	return chargers, nil
}
