package superadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CPOCustomerIntelligenceResponse ...
type CPOCustomerIntelligenceResponse struct {
	CPO          CPOInfo             `json:"cpo"`
	Metrics      IntelligenceMetrics `json:"metrics"`
	TopCustomers []TopCustomer       `json:"top_customers"`
}

type CPOInfo struct {
	ID           uuid.UUID `json:"id"`
	BusinessName string    `json:"business_name"`
}

type IntelligenceMetrics struct {
	TotalAppUsers      int64   `json:"total_app_users"`
	ActiveUsers        int64   `json:"active_users"`
	MonthlyActiveUsers int64   `json:"monthly_active_users"`
	ChargingCustomers  int64   `json:"charging_customers"`
	TotalSessions      int64   `json:"total_sessions"`
	EnergyConsumed     float64 `json:"energy_consumed_kwh"`
	CustomerRevenue    float64 `json:"customer_revenue"`
	RepeatCustomerRate float64 `json:"repeat_customer_rate"`
	CustomerRetention  float64 `json:"customer_retention"`
}

type TopCustomer struct {
	Rank      int       `json:"rank"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Sessions  int64     `json:"sessions"`
	EnergyKWh float64   `json:"energy_kwh"`
	Spend     float64   `json:"spend"`
	Chargers  int64     `json:"chargers_used"`
}

// CustomerIntelligence ...
func (service *Service) CustomerIntelligence(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) (CPOCustomerIntelligenceResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return CPOCustomerIntelligenceResponse{}, err
	}

	var cpo models.CPO
	if err := service.database.WithContext(ctx).Where("id = ?", cpoID).First(&cpo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return CPOCustomerIntelligenceResponse{}, fmt.Errorf("cpo not found")
		}
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to fetch CPO: %w", err)
	}

	metrics, err := service.computeIntelligenceMetrics(ctx, cpoID)
	if err != nil {
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to compute metrics: %w", err)
	}

	topCustomers, err := service.fetchTopCustomers(ctx, cpoID, 10)
	if err != nil {
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to fetch top customers: %w", err)
	}

	return CPOCustomerIntelligenceResponse{
		CPO:          CPOInfo{ID: cpo.ID, BusinessName: cpo.BusinessName},
		Metrics:      metrics,
		TopCustomers: topCustomers,
	}, nil
}

// computeIntelligenceMetrics ...
func (service *Service) computeIntelligenceMetrics(ctx context.Context, cpoID uuid.UUID) (IntelligenceMetrics, error) {
	var metrics IntelligenceMetrics
	db := service.database.WithContext(ctx)

	// 1. Total app users (customers belonging to this CPO)
	if err := db.Model(&models.Customer{}).Where("cpo_id = ?", cpoID).Count(&metrics.TotalAppUsers).Error; err != nil {
		return metrics, fmt.Errorf("failed to count total users: %w", err)
	}

	// 2. Total sessions
	if err := db.Model(&models.ChargingSession{}).Where("cpo_id = ?", cpoID).Count(&metrics.TotalSessions).Error; err != nil {
		return metrics, fmt.Errorf("failed to count total sessions: %w", err)
	}

	// 3. Energy & revenue sums — using correct column names from ChargingSession model
	var energy, revenue float64
	if err := db.Model(&models.ChargingSession{}).
		Select("COALESCE(SUM(total_kwh), 0), COALESCE(SUM(total_amount), 0)").
		Where("cpo_id = ?", cpoID).
		Row().Scan(&energy, &revenue); err != nil {
		return metrics, fmt.Errorf("failed to aggregate energy/revenue: %w", err)
	}
	metrics.EnergyConsumed = energy
	metrics.CustomerRevenue = revenue

	// 4. Charging customers (distinct customers with at least one session)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ?", cpoID).
		Distinct("customer_id").
		Count(&metrics.ChargingCustomers).Error; err != nil {
		return metrics, fmt.Errorf("failed to count charging customers: %w", err)
	}

	// 5. Active users (last 90 days)
	activeCutoff := time.Now().AddDate(0, 0, -90)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ? AND start_time >= ?", cpoID, activeCutoff).
		Distinct("customer_id").
		Count(&metrics.ActiveUsers).Error; err != nil {
		return metrics, fmt.Errorf("failed to count active users: %w", err)
	}

	// 6. Monthly active users (last 30 days)
	monthlyCutoff := time.Now().AddDate(0, 0, -30)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ? AND start_time >= ?", cpoID, monthlyCutoff).
		Distinct("customer_id").
		Count(&metrics.MonthlyActiveUsers).Error; err != nil {
		return metrics, fmt.Errorf("failed to count monthly active users: %w", err)
	}

	// 7. Repeat customer rate
	var repeatCustomers int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT customer_id
			FROM charging_sessions
			WHERE cpo_id = ?
			GROUP BY customer_id
			HAVING COUNT(*) > 1
		) AS repeat_customers
	`, cpoID).Scan(&repeatCustomers).Error; err != nil {
		return metrics, fmt.Errorf("failed to count repeat customers: %w", err)
	}
	if metrics.ChargingCustomers > 0 {
		metrics.RepeatCustomerRate = (float64(repeatCustomers) / float64(metrics.ChargingCustomers)) * 100
	}

	// 8. Customer retention
	priorStart := time.Now().AddDate(0, 0, -180)
	priorEnd := time.Now().AddDate(0, 0, -90)
	currentStart := time.Now().AddDate(0, 0, -90)

	var priorCustomers []uuid.UUID
	if err := db.Model(&models.ChargingSession{}).
		Select("DISTINCT customer_id").
		Where("cpo_id = ? AND start_time BETWEEN ? AND ?", cpoID, priorStart, priorEnd).
		Find(&priorCustomers).Error; err != nil {
		return metrics, fmt.Errorf("failed to fetch prior customers: %w", err)
	}

	var currentCustomers []uuid.UUID
	if err := db.Model(&models.ChargingSession{}).
		Select("DISTINCT customer_id").
		Where("cpo_id = ? AND start_time >= ?", cpoID, currentStart).
		Find(&currentCustomers).Error; err != nil {
		return metrics, fmt.Errorf("failed to fetch current customers: %w", err)
	}

	priorMap := make(map[uuid.UUID]bool, len(priorCustomers))
	for _, id := range priorCustomers {
		priorMap[id] = true
	}
	retained := 0
	for _, id := range currentCustomers {
		if priorMap[id] {
			retained++
		}
	}
	if len(priorCustomers) > 0 {
		metrics.CustomerRetention = (float64(retained) / float64(len(priorCustomers))) * 100
	}

	return metrics, nil
}

// fetchTopCustomers ...
func (service *Service) fetchTopCustomers(ctx context.Context, cpoID uuid.UUID, limit int) ([]TopCustomer, error) {
	type customerAgg struct {
		CustomerID   uuid.UUID `gorm:"column:customer_id"`
		CustomerName string    `gorm:"column:customer_name"`
		Sessions     int64     `gorm:"column:sessions"`
		Energy       float64   `gorm:"column:energy"`
		Spend        float64   `gorm:"column:spend"`
		Chargers     int64     `gorm:"column:chargers"`
	}

	var aggResults []customerAgg
	db := service.database.WithContext(ctx)

	err := db.Table("charging_sessions").
		Select(`
			charging_sessions.customer_id,
			COALESCE(NULLIF(customers.full_name, ''), customers.email, 'Unknown') AS customer_name,
			COUNT(charging_sessions.id) AS sessions,
			COALESCE(SUM(charging_sessions.total_kwh), 0) AS energy,
			COALESCE(SUM(charging_sessions.total_amount), 0) AS spend,
			COUNT(DISTINCT charging_sessions.charger_id) AS chargers
		`).
		Joins("INNER JOIN customers ON customers.id = charging_sessions.customer_id").
		Where("charging_sessions.cpo_id = ?", cpoID).
		Group("charging_sessions.customer_id, customers.full_name, customers.email").
		Order("spend DESC").
		Limit(limit).
		Scan(&aggResults).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query top customers: %w", err)
	}

	topCustomers := make([]TopCustomer, len(aggResults))
	for i, agg := range aggResults {
		topCustomers[i] = TopCustomer{
			Rank:      i + 1,
			UserID:    agg.CustomerID,
			Name:      agg.CustomerName,
			Sessions:  agg.Sessions,
			EnergyKWh: agg.Energy,
			Spend:     agg.Spend,
			Chargers:  agg.Chargers,
		}
	}
	return topCustomers, nil
}
