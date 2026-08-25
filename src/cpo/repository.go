package cpo

import (
	"context"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants" // <-- added

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Repository interface {
	GetAnalytics(ctx context.Context, cpoID uuid.UUID, query AnalyticsQuery) (Analytics, error)
	ListWalletTransactions(ctx context.Context, cpoID uuid.UUID, query WalletTransactionListQuery) ([]WalletTransactionDetail, error)
	GetChargingSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error)
	ListChargingSessions(ctx context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error)
	ListChargerTransactions(ctx context.Context, cpoID uuid.UUID, query ChargerTransactionListQuery) ([]ChargerTransaction, error)
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

// AnalyticsQuery holds optional time‑range filters for analytics.
// It should be defined in schemas.go (or alongside the repository).

// GetAnalytics returns overall and optionally period‑filtered analytics.
func (r *repository) GetAnalytics(ctx context.Context, cpoID uuid.UUID, query AnalyticsQuery) (Analytics, error) {
	var analytics Analytics

	// 1) Total chargers and connectors – always overall (not time‑filtered)
	if err := r.db.WithContext(ctx).Model(&models.Charger{}).Where("cpo_id = ?", cpoID).Count(&analytics.TotalChargers).Error; err != nil {
		return Analytics{}, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Connector{}).Where("cpo_id = ?", cpoID).Count(&analytics.TotalConnectors).Error; err != nil {
		return Analytics{}, err
	}

	// 2) Build session aggregation query
	db := r.db.WithContext(ctx).Model(&models.ChargingSession{}).
		Where("cpo_id = ?", cpoID)

	// 3) Apply time filter if period and date are given and valid
	if query.Period != "" && query.Date != "" {
		var start, end time.Time
		var err error

		switch query.Period {
		case "day":
			start, err = time.Parse("2006-01-02", query.Date)
			if err == nil {
				end = start.Add(24 * time.Hour)
				db = db.Where("start_time >= ? AND start_time < ?", start, end)
			}
		case "week":
			// Compute start of the week (Monday) from the given date
			t, err := time.Parse("2006-01-02", query.Date)
			if err == nil {
				// Go's weekday: Sunday=0, Monday=1, ...
				offset := int((t.Weekday() + 6) % 7) // days back to Monday
				start = t.AddDate(0, 0, -offset)
				end = start.AddDate(0, 0, 7)
				db = db.Where("start_time >= ? AND start_time < ?", start, end)
			}
		case "month":
			t, err := time.Parse("2006-01-02", query.Date)
			if err == nil {
				start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
				end = start.AddDate(0, 1, 0)
				db = db.Where("start_time >= ? AND start_time < ?", start, end)
			}
		case "year":
			t, err := time.Parse("2006-01-02", query.Date)
			if err == nil {
				start = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
				end = start.AddDate(1, 0, 0)
				db = db.Where("start_time >= ? AND start_time < ?", start, end)
			}
		default:
			// Unsupported period -> ignore filter (or you may return an error)
		}
	}

	// 4) Execute aggregation
	var sessionAnalytics struct {
		TotalRevenue  decimal.Decimal
		TotalUsage    decimal.Decimal
		TotalSessions int64
	}
	if err := db.Select(
		"COALESCE(SUM(total_amount), 0) as total_revenue, " +
			"COALESCE(SUM(total_kwh), 0) as total_usage, " +
			"COUNT(*) as total_sessions",
	).Scan(&sessionAnalytics).Error; err != nil {
		return Analytics{}, err
	}

	analytics.TotalRevenue = sessionAnalytics.TotalRevenue
	analytics.TotalUsage = sessionAnalytics.TotalUsage
	analytics.TotalSessions = sessionAnalytics.TotalSessions

	return analytics, nil
}

func (r *repository) GetChargingSession(ctx context.Context, cpoID, sessionID uuid.UUID) (*models.ChargingSession, error) {
	var session models.ChargingSession
	err := r.db.WithContext(ctx).
		Preload("Customer").
		Preload("Charger").
		Preload("Charger.Hub").
		Preload("Connector").
		Preload("Connector.Charger").     // new
		Preload("Connector.Charger.Hub"). // new
		Where("cpo_id = ? AND id = ?", cpoID, sessionID).
		First(&session).Error
	if err != nil {
		return nil, err
	}

	// Fallback: if session.Charger is empty but connector has a charger, use it.
	if session.Charger.ID == uuid.Nil && session.Connector.Charger.ID != uuid.Nil {
		session.Charger = session.Connector.Charger
		session.ChargerID = session.Connector.Charger.ID
	}
	return &session, nil
}

func (r *repository) ListChargingSessions(ctx context.Context, cpoID uuid.UUID, query ChargingSessionListQuery) ([]models.ChargingSession, error) {
	var sessions []models.ChargingSession
	db := r.db.WithContext(ctx).
		Preload("Customer").
		Preload("Charger").
		Preload("Charger.Hub").
		Preload("Connector").
		Preload("Connector.Charger").     // new
		Preload("Connector.Charger.Hub"). // new
		Where("cpo_id = ?", cpoID)

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
	err := db.Order("created_at DESC, id DESC").Find(&sessions).Error
	if err != nil {
		return nil, err
	}

	// For each session, fallback to connector's charger if needed.
	for i := range sessions {
		if sessions[i].Charger.ID == uuid.Nil && sessions[i].Connector.Charger.ID != uuid.Nil {
			sessions[i].Charger = sessions[i].Connector.Charger
			sessions[i].ChargerID = sessions[i].Connector.Charger.ID
		}
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

type ChargerTransaction struct {
	models.ChargingSession
	PaymentStatus       constants.FinancialStatus `gorm:"column:payment_status"`
	ChargerCode         string                    `gorm:"column:charger_code"`
	ChargerOCPPIdentity string                    `gorm:"column:charger_ocpp_identity"`
	ChargerName         string                    `gorm:"column:charger_name"`
	HubID               *uuid.UUID                `gorm:"column:hub_id"`
	HubName             string                    `gorm:"column:hub_name"`
	HubAddress          string                    `gorm:"column:hub_address"`
	ConnectorNumber     *int                      `gorm:"column:connector_number"`
	ConnectorType       string                    `gorm:"column:connector_type"`
	TariffPricePerUnit  decimal.Decimal           `gorm:"column:tariff_price_per_unit"`
	CPOBusinessName     string                    `gorm:"column:cpo_business_name"`
	ChargerHostName     string                    `gorm:"column:charger_host_name"`
	ChargerHostPhoneNo  string                    `gorm:"column:charger_host_phone_no"`
	CustomerFullName    string                    `gorm:"column:customer_full_name"`
	CustomerEmail       string                    `gorm:"column:customer_email"`
	CustomerPhone       *string                   `gorm:"column:customer_phone"`
}

func (r *repository) ListChargerTransactions(ctx context.Context, cpoID uuid.UUID, query ChargerTransactionListQuery) ([]ChargerTransaction, error) {
	var transactions []ChargerTransaction
	db := r.db.WithContext(ctx).Model(&models.ChargingSession{}).
		Joins("LEFT JOIN wallet_transactions ON wallet_transactions.session_id = charging_sessions.id").
		Joins("LEFT JOIN connectors AS session_connectors ON session_connectors.id = charging_sessions.connector_id").
		Joins("LEFT JOIN chargers AS session_chargers ON session_chargers.id = charging_sessions.charger_id").
		Joins("LEFT JOIN chargers AS connector_chargers ON connector_chargers.id = session_connectors.charger_id").
		Joins("LEFT JOIN hubs ON hubs.id = COALESCE(session_chargers.hub_id, connector_chargers.hub_id)").
		Joins("LEFT JOIN tariffs ON tariffs.id = charging_sessions.tariff_id").
		Joins("LEFT JOIN cpos ON cpos.id = charging_sessions.cpo_id").
		Joins("LEFT JOIN customers ON customers.id = charging_sessions.customer_id").
		Where("charging_sessions.cpo_id = ?", cpoID)

	if query.ChargerID != nil {
		db = db.Where("charging_sessions.charger_id = ?", *query.ChargerID)
	}
	if query.CustomerID != nil {
		db = db.Where("charging_sessions.customer_id = ?", *query.CustomerID)
	}

	if query.Before != nil {
		if query.BeforeID != nil {
			db = db.Where("(charging_sessions.created_at, charging_sessions.id) < (?, ?)", *query.Before, *query.BeforeID)
		} else {
			db = db.Where("charging_sessions.created_at < ?", *query.Before)
		}
	}

	if query.Limit > 0 {
		db = db.Limit(query.Limit + 1)
	}

	err := db.Select(
		"charging_sessions.*",
		"wallet_transactions.status as payment_status",
		"COALESCE(session_chargers.charger_id, connector_chargers.charger_id) as charger_code",
		"COALESCE(session_chargers.ocpp_identity, connector_chargers.ocpp_identity) as charger_ocpp_identity",
		"COALESCE(session_chargers.charger_name, connector_chargers.charger_name) as charger_name",
		"hubs.id as hub_id",
		"hubs.name as hub_name",
		"hubs.address as hub_address",
		"session_connectors.connector_number",
		"session_connectors.connector_type",
		"tariffs.price_per_unit as tariff_price_per_unit",
		"cpos.business_name as cpo_business_name",
		"COALESCE(session_chargers.charger_host_name, connector_chargers.charger_host_name) as charger_host_name",
		"COALESCE(session_chargers.charger_host_phone_no, connector_chargers.charger_host_phone_no) as charger_host_phone_no",
		"customers.full_name as customer_full_name",
		"customers.email as customer_email",
		"customers.phone as customer_phone",
	).
		Order("charging_sessions.created_at DESC, charging_sessions.id DESC").
		Scan(&transactions).Error

	if err != nil {
		return nil, err
	}

	return transactions, nil
}

// Define a new struct to hold the joined data from wallet_transactions, wallets, and customers.
type WalletTransactionDetail struct {
	models.WalletTransaction
	CustomerID    uuid.UUID `gorm:"column:customer_id"`
	CustomerName  string    `gorm:"column:customer_name"`
	CustomerEmail string    `gorm:"column:customer_email"`
	Currency      string    `gorm:"column:currency"`
}

// Implement the new method for the repository struct (around line 114)
func (r *repository) ListWalletTransactions(ctx context.Context, cpoID uuid.UUID, query WalletTransactionListQuery) ([]WalletTransactionDetail, error) {
	var transactions []WalletTransactionDetail
	db := r.db.WithContext(ctx).
		Table("wallet_transactions").
		Select("wallet_transactions.*, customers.id as customer_id, customers.full_name as customer_name, customers.email as customer_email, wallets.currency").
		Joins("JOIN wallets ON wallets.id = wallet_transactions.wallet_id").
		Joins("JOIN customers ON customers.id = wallets.customer_id").
		Where("wallet_transactions.cpo_id = ?", cpoID)

	if query.CustomerID != nil {
		db = db.Where("customers.id = ?", *query.CustomerID)
	}

	if query.Before != nil {
		if query.BeforeID != nil {
			db = db.Where("(wallet_transactions.created_at, wallet_transactions.id) < (?, ?)", *query.Before, *query.BeforeID)
		} else {
			db = db.Where("wallet_transactions.created_at < ?", *query.Before)
		}
	}

	if query.Limit > 0 {
		db = db.Limit(query.Limit + 1)
	}

	if err := db.Order("wallet_transactions.created_at DESC, wallet_transactions.id DESC").Scan(&transactions).Error; err != nil {
		return nil, err
	}

	return transactions, nil
}
