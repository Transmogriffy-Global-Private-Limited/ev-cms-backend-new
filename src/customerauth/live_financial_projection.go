package customerauth

import (
	"context"
	"fmt"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/commercial"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

// ChargingSessionFinancialProjectionView keeps the current accrued amount
// semantically separate from final total_amount.
//
// Once final completion/settlement exists, total_amount remains authoritative
// and projected_amount is omitted.
type ChargingSessionFinancialProjectionView struct {
	ChargingSessionView
	ProjectedAmount *string `json:"projected_amount,omitempty"`
}

func (service *Service) GetChargingSessionWithFinancialProjection(
	ctx context.Context,
	principal Principal,
	sessionID uuid.UUID,
) (ChargingSessionFinancialProjectionView, error) {
	base, err := service.GetChargingSession(ctx, principal, sessionID)
	if err != nil {
		return ChargingSessionFinancialProjectionView{}, err
	}

	var source models.ChargingSession

	if err := service.database.
		WithContext(ctx).
		Select(
			"id",
			"cpo_id",
			"customer_id",
			"tariff_snapshot",
			"tax_snapshot",
			"currency",
			"start_time",
			"end_time",
			"meter_start_wh",
			"latest_meter_wh",
		).
		First(
			&source,
			"id = ? AND cpo_id = ? AND customer_id = ?",
			sessionID,
			principal.CPOID,
			principal.CustomerID,
		).Error; err != nil {
		return ChargingSessionFinancialProjectionView{},
			fmt.Errorf("load charging-session financial source: %w", err)
	}

	result := ChargingSessionFinancialProjectionView{
		ChargingSessionView: base,
	}

	// Final settled amount becomes authoritative once the session has ended.
	if source.EndTime != nil {
		return result, nil
	}

	consumedWh := int64(0)

	if base.ConsumedWh != nil {
		consumedWh = *base.ConsumedWh
	} else if source.LatestMeterWh != nil &&
		*source.LatestMeterWh >= source.MeterStartWh {
		consumedWh = *source.LatestMeterWh - source.MeterStartWh
	}

	projectedAt := service.now()

	if base.CompletedAt != nil && base.CompletedAt.Before(projectedAt) {
		projectedAt = *base.CompletedAt
	}

	if projectedAt.Before(source.StartTime) {
		projectedAt = source.StartTime
	}

	amount, err := commercial.SessionAmountFromSnapshots(
		source.TariffSnapshot,
		source.TaxSnapshot,
		consumedWh,
		source.StartTime,
		projectedAt,
	)
	if err != nil {
		return ChargingSessionFinancialProjectionView{},
			fmt.Errorf("project charging-session amount: %w", err)
	}

	value := amount.StringFixed(2)
	result.ProjectedAmount = &value

	return result, nil
}
