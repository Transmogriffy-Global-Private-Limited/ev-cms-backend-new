package cpo

import (
	"context"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LiveChargingSessionFinancialView enriches the existing operational
// live-session projection with the amount accrued so far.
//
// ProjectedAmount is deliberately distinct from the final persisted
// TotalAmount used after settlement.
type LiveChargingSessionFinancialView struct {
	LiveChargingSessionView
	ProjectedAmount decimal.Decimal `json:"projected_amount"`
	Currency        string          `json:"currency"`
}

type LiveChargingSessionFinancialListResponse struct {
	Sessions           []LiveChargingSessionFinancialView `json:"sessions"`
	NextAfterStartedAt *time.Time                         `json:"next_after_started_at,omitempty"`
	NextAfterID        *uuid.UUID                         `json:"next_after_id,omitempty"`
	HasMore            bool                               `json:"has_more"`
	AsOf               time.Time                          `json:"as_of"`
}

func (service *Service) ListLiveChargingSessionsWithFinancialProjection(
	ctx context.Context,
	principal auth.Principal,
	query LiveChargingSessionListQuery,
) (LiveChargingSessionFinancialListResponse, error) {
	base, err := service.ListLiveChargingSessions(ctx, principal, query)
	if err != nil {
		return LiveChargingSessionFinancialListResponse{}, err
	}

	response := LiveChargingSessionFinancialListResponse{
		Sessions:           make([]LiveChargingSessionFinancialView, 0, len(base.Sessions)),
		NextAfterStartedAt: base.NextAfterStartedAt,
		NextAfterID:        base.NextAfterID,
		HasMore:            base.HasMore,
		AsOf:               base.AsOf,
	}

	if len(base.Sessions) == 0 {
		return response, nil
	}

	if principal.CPOID == nil {
		return LiveChargingSessionFinancialListResponse{},
			fmt.Errorf("CPO context disappeared during live-session projection")
	}

	ids := make([]uuid.UUID, 0, len(base.Sessions))
	for _, session := range base.Sessions {
		ids = append(ids, session.SessionID)
	}

	var sources []models.ChargingSession

	if err := service.database.
		WithContext(ctx).
		Select(
			"id",
			"cpo_id",
			"tariff_snapshot",
			"tax_snapshot",
			"currency",
			"start_time",
			"meter_start_wh",
			"latest_meter_wh",
		).
		Where("cpo_id = ? AND id IN ?", *principal.CPOID, ids).
		Find(&sources).Error; err != nil {
		return LiveChargingSessionFinancialListResponse{},
			fmt.Errorf("load live-session financial sources: %w", err)
	}

	sourceByID := make(
		map[uuid.UUID]models.ChargingSession,
		len(sources),
	)

	for _, source := range sources {
		sourceByID[source.ID] = source
	}

	for _, view := range base.Sessions {
		source, ok := sourceByID[view.SessionID]
		if !ok {
			return LiveChargingSessionFinancialListResponse{},
				fmt.Errorf("live-session financial source disappeared during read")
		}

		projected, err := projectLiveChargingSessionFinancial(
			view,
			source,
			base.AsOf,
		)
		if err != nil {
			return LiveChargingSessionFinancialListResponse{},
				fmt.Errorf("project live-session amount: %w", err)
		}

		response.Sessions = append(response.Sessions, projected)
	}

	return response, nil
}

func projectLiveChargingSessionFinancial(
	view LiveChargingSessionView,
	source models.ChargingSession,
	asOf time.Time,
) (LiveChargingSessionFinancialView, error) {
	consumedWh := int64(0)

	if view.ConsumedWh != nil {
		consumedWh = *view.ConsumedWh
	} else if source.LatestMeterWh != nil &&
		*source.LatestMeterWh >= source.MeterStartWh {
		consumedWh = *source.LatestMeterWh - source.MeterStartWh
	}

	amount, err := commercial.SessionAmountFromSnapshots(
		source.TariffSnapshot,
		source.TaxSnapshot,
		consumedWh,
		source.StartTime,
		asOf,
	)
	if err != nil {
		return LiveChargingSessionFinancialView{}, err
	}

	return LiveChargingSessionFinancialView{
		LiveChargingSessionView: view,
		ProjectedAmount:         amount,
		Currency:                source.Currency,
	}, nil
}
