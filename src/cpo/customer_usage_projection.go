package cpo

import (
	"context"
	"fmt"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type customerUsageProjection struct {
	CustomerID    uuid.UUID       `gorm:"column:customer_id"`
	TotalUsageKWh decimal.Decimal `gorm:"column:total_usage_kwh"`
	SessionCount  int64           `gorm:"column:session_count"`
}

// GetCustomerWithCurrentUsage makes total_usage_kwh represent actual current
// customer usage.
//
// Completed/reconciled sessions contribute their persisted total_kwh.
// Sessions still in progress contribute their latest persisted meter delta.
func (service *Service) GetCustomerWithCurrentUsage(
	ctx context.Context,
	principal auth.Principal,
	customerID uuid.UUID,
) (CPOAdminCustomerView, error) {
	view, err := service.GetCustomer(ctx, principal, customerID)
	if err != nil {
		return CPOAdminCustomerView{}, err
	}

	if principal.CPOID == nil {
		return CPOAdminCustomerView{},
			fmt.Errorf("CPO context disappeared during customer usage projection")
	}

	summaries, err := service.customerUsageProjections(
		ctx,
		*principal.CPOID,
		[]uuid.UUID{customerID},
	)
	if err != nil {
		return CPOAdminCustomerView{}, err
	}

	view.TotalUsage = decimal.Zero
	view.NoOfSessions = 0

	if summary, ok := summaries[customerID]; ok {
		view.TotalUsage = summary.TotalUsageKWh
		view.NoOfSessions = summary.SessionCount
	}

	return view, nil
}

func (service *Service) ListCustomersWithCurrentUsage(
	ctx context.Context,
	principal auth.Principal,
	query CPOAdminCustomerListQuery,
) (CPOAdminCustomerListResponse, error) {
	response, err := service.ListCustomers(ctx, principal, query)
	if err != nil {
		return CPOAdminCustomerListResponse{}, err
	}

	if len(response.Customers) == 0 {
		return response, nil
	}

	if principal.CPOID == nil {
		return CPOAdminCustomerListResponse{},
			fmt.Errorf("CPO context disappeared during customer usage projection")
	}

	ids := make([]uuid.UUID, 0, len(response.Customers))
	for _, customer := range response.Customers {
		ids = append(ids, customer.ID)
	}

	summaries, err := service.customerUsageProjections(
		ctx,
		*principal.CPOID,
		ids,
	)
	if err != nil {
		return CPOAdminCustomerListResponse{}, err
	}

	for i := range response.Customers {
		response.Customers[i].TotalUsage = decimal.Zero
		response.Customers[i].NoOfSessions = 0

		if summary, ok := summaries[response.Customers[i].ID]; ok {
			response.Customers[i].TotalUsage = summary.TotalUsageKWh
			response.Customers[i].NoOfSessions = summary.SessionCount
		}
	}

	return response, nil
}

func (service *Service) customerUsageProjections(
	ctx context.Context,
	cpoID uuid.UUID,
	customerIDs []uuid.UUID,
) (map[uuid.UUID]customerUsageProjection, error) {
	result := make(
		map[uuid.UUID]customerUsageProjection,
		len(customerIDs),
	)

	if len(customerIDs) == 0 {
		return result, nil
	}

	query := `
SELECT
customer_id,
COALESCE(
SUM(
CASE
WHEN end_time IS NULL AND status IN (?, ?, ?) THEN
GREATEST(
COALESCE(latest_meter_wh, meter_start_wh) - meter_start_wh,
0
)::numeric / 1000
WHEN status IN (?, ?) THEN
total_kwh
ELSE
0
END
),
0
) AS total_usage_kwh,
COALESCE(
SUM(
CASE
WHEN status IN (?, ?, ?, ?) THEN 1
ELSE 0
END
),
0
)::bigint AS session_count
FROM charging_sessions
WHERE cpo_id = ?
  AND customer_id IN ?
GROUP BY customer_id
`

	var rows []customerUsageProjection

	if err := service.database.
		WithContext(ctx).
		Raw(
			query,

			constants.SessionStatusActive,
			constants.SessionStatusStopPending,
			constants.SessionStatusReconciliationRequired,

			constants.SessionStatusCompleted,
			constants.SessionStatusReconciliationRequired,

			constants.SessionStatusActive,
			constants.SessionStatusStopPending,
			constants.SessionStatusCompleted,
			constants.SessionStatusReconciliationRequired,

			cpoID,
			customerIDs,
		).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("project customer usage: %w", err)
	}

	for _, row := range rows {
		result[row.CustomerID] = row
	}

	return result, nil
}
