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

// CPOCustomerIntelligenceResponse is the top-level response for the CPO customer intelligence dashboard.
type CPOCustomerIntelligenceResponse struct {
	CPO          CPOInfo             `json:"cpo"`
	Metrics      IntelligenceMetrics `json:"metrics"`
	TopCustomers []TopCustomer       `json:"top_customers"`
}

// CPOInfo contains basic CPO details.
type CPOInfo struct {
	ID           uuid.UUID `json:"id"`
	BusinessName string    `json:"business_name"`
}

// IntelligenceMetrics holds the aggregated KPIs.
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

// TopCustomer represents a single customer in the top list.
type TopCustomer struct {
	Rank      int       `json:"rank"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Sessions  int64     `json:"sessions"`
	EnergyKWh float64   `json:"energy_kwh"`
	Spend     float64   `json:"spend"`
	Chargers  int64     `json:"chargers_used"`
}

// CustomerIntelligence returns the customer intelligence dashboard data for a given CPO.
func (service *Service) CustomerIntelligence(ctx context.Context, principal auth.Principal, cpoID uuid.UUID) (CPOCustomerIntelligenceResponse, error) {
	// 1. Authorization: only superadmin can access this endpoint.
	if err := requirePlatform(principal); err != nil {
		return CPOCustomerIntelligenceResponse{}, err
	}

	// 2. Fetch the CPO details.
	var cpo models.CPO
	if err := service.database.WithContext(ctx).Where("id = ?", cpoID).First(&cpo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return CPOCustomerIntelligenceResponse{}, fmt.Errorf("cpo not found")
		}
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to fetch CPO: %w", err)
	}

	// 3. Compute all metrics.
	metrics, err := service.computeIntelligenceMetrics(ctx, cpoID)
	if err != nil {
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to compute metrics: %w", err)
	}

	// 4. Fetch top customers.
	topCustomers, err := service.fetchTopCustomers(ctx, cpoID, 10) // top 10 by spend
	if err != nil {
		return CPOCustomerIntelligenceResponse{}, fmt.Errorf("failed to fetch top customers: %w", err)
	}

	// 5. Build response.
	resp := CPOCustomerIntelligenceResponse{
		CPO: CPOInfo{
			ID:           cpo.ID,
			BusinessName: cpo.BusinessName,
		},
		Metrics:      metrics,
		TopCustomers: topCustomers,
	}

	return resp, nil
}

// computeIntelligenceMetrics aggregates KPIs for the given CPO.
func (service *Service) computeIntelligenceMetrics(ctx context.Context, cpoID uuid.UUID) (IntelligenceMetrics, error) {
	var metrics IntelligenceMetrics
	db := service.database.WithContext(ctx)

	// Total app users (users belonging to this CPO tenant)
	if err := db.Model(&models.User{}).Where("tenant_id = ?", cpoID).Count(&metrics.TotalAppUsers).Error; err != nil {
		return metrics, err
	}

	// Active users: users who have at least one session in the last 30 days? Or just any session?
	// We'll define "active" as having at least one session in the last 90 days (common definition).
	// For "charging customers", we'll count distinct users with at least one session ever.
	// We'll do these in a single query where possible.

	// We'll compute active, monthly active, charging customers, total sessions, energy, revenue.
	// All based on charging_sessions table.

	// For simplicity, let's use subqueries or separate queries.

	// Total sessions
	if err := db.Model(&models.ChargingSession{}).Where("cpo_id = ?", cpoID).Count(&metrics.TotalSessions).Error; err != nil {
		return metrics, err
	}

	// Energy and revenue sums
	var energy, revenue float64
	if err := db.Model(&models.ChargingSession{}).
		Select("COALESCE(SUM(energy_kwh), 0), COALESCE(SUM(amount), 0)").
		Where("cpo_id = ?", cpoID).
		Row().Scan(&energy, &revenue); err != nil {
		return metrics, err
	}
	metrics.EnergyConsumed = energy
	metrics.CustomerRevenue = revenue

	// Distinct users who have at least one session (charging customers)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ?", cpoID).
		Distinct("user_id").
		Count(&metrics.ChargingCustomers).Error; err != nil {
		return metrics, err
	}

	// Active users: users with session in last 90 days
	activeCutoff := time.Now().AddDate(0, 0, -90)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ? AND start_time >= ?", cpoID, activeCutoff).
		Distinct("user_id").
		Count(&metrics.ActiveUsers).Error; err != nil {
		return metrics, err
	}

	// Monthly active users: users with session in last 30 days
	monthlyCutoff := time.Now().AddDate(0, 0, -30)
	if err := db.Model(&models.ChargingSession{}).
		Where("cpo_id = ? AND start_time >= ?", cpoID, monthlyCutoff).
		Distinct("user_id").
		Count(&metrics.MonthlyActiveUsers).Error; err != nil {
		return metrics, err
	}

	// Repeat customer rate: percentage of users with more than one session.
	var repeatUsers int64
	if err := db.Model(&models.ChargingSession{}).
		Select("user_id").
		Where("cpo_id = ?", cpoID).
		Group("user_id").
		Having("COUNT(*) > 1").
		Count(&repeatUsers).Error; err != nil {
		return metrics, err
	}
	if metrics.ChargingCustomers > 0 {
		metrics.RepeatCustomerRate = float64(repeatUsers) / float64(metrics.ChargingCustomers) * 100
	} else {
		metrics.RepeatCustomerRate = 0
	}

	// Customer retention: percentage of users active in both current and previous period (e.g., previous 90 days vs prior 90 days)
	// We'll compute users active in previous period (days -180 to -90) and also active in current period (-90 to now).
	// Then intersect.
	priorStart := time.Now().AddDate(0, 0, -180)
	priorEnd := time.Now().AddDate(0, 0, -90)
	currentStart := time.Now().AddDate(0, 0, -90)

	var priorUsers []uuid.UUID
	if err := db.Model(&models.ChargingSession{}).
		Select("DISTINCT user_id").
		Where("cpo_id = ? AND start_time BETWEEN ? AND ?", cpoID, priorStart, priorEnd).
		Find(&priorUsers).Error; err != nil {
		return metrics, err
	}

	var currentUsers []uuid.UUID
	if err := db.Model(&models.ChargingSession{}).
		Select("DISTINCT user_id").
		Where("cpo_id = ? AND start_time >= ?", cpoID, currentStart).
		Find(&currentUsers).Error; err != nil {
		return metrics, err
	}

	// Calculate intersection
	priorMap := make(map[uuid.UUID]bool, len(priorUsers))
	for _, id := range priorUsers {
		priorMap[id] = true
	}
	retained := 0
	for _, id := range currentUsers {
		if priorMap[id] {
			retained++
		}
	}
	if len(priorUsers) > 0 {
		metrics.CustomerRetention = float64(retained) / float64(len(priorUsers)) * 100
	} else {
		metrics.CustomerRetention = 0
	}

	return metrics, nil
}

// fetchTopCustomers returns the top N customers by spend for a given CPO.
func (service *Service) fetchTopCustomers(ctx context.Context, cpoID uuid.UUID, limit int) ([]TopCustomer, error) {
	type customerAgg struct {
		UserID   uuid.UUID `gorm:"column:user_id"`
		UserName string    `gorm:"column:user_name"`
		Sessions int64     `gorm:"column:sessions"`
		Energy   float64   `gorm:"column:energy"`
		Spend    float64   `gorm:"column:spend"`
		Chargers int64     `gorm:"column:chargers"`
	}

	var aggResults []customerAgg
	db := service.database.WithContext(ctx)

	// Join with users table to get name.
	// We assume that charging_sessions has user_id and cpo_id, and users table has name.
	// Also assume that amount is stored in charging_sessions (or we might need to join with invoices).
	// For simplicity, we'll sum amount directly from sessions.
	err := db.Table("charging_sessions").
		Select(`
			charging_sessions.user_id,
			users.name as user_name,
			COUNT(*) as sessions,
			COALESCE(SUM(charging_sessions.energy_kwh), 0) as energy,
			COALESCE(SUM(charging_sessions.amount), 0) as spend,
			COUNT(DISTINCT charging_sessions.charger_id) as chargers
		`).
		Joins("INNER JOIN users ON users.id = charging_sessions.user_id").
		Where("charging_sessions.cpo_id = ?", cpoID).
		Group("charging_sessions.user_id, users.name").
		Order("spend DESC").
		Limit(limit).
		Scan(&aggResults).Error

	if err != nil {
		return nil, err
	}

	// Convert to TopCustomer slice with rank.
	topCustomers := make([]TopCustomer, len(aggResults))
	for i, agg := range aggResults {
		topCustomers[i] = TopCustomer{
			Rank:      i + 1,
			UserID:    agg.UserID,
			Name:      agg.UserName,
			Sessions:  agg.Sessions,
			EnergyKWh: agg.Energy,
			Spend:     agg.Spend,
			Chargers:  agg.Chargers,
		}
	}
	return topCustomers, nil
}
