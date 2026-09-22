// Package ratingaggregate owns the neutral, read-time charger-rating
// aggregate shared by customer and CPO projections. Authorization remains in
// the calling surface; this package only accepts an already-scoped CPO ID.
package ratingaggregate

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ChargerAggregate struct {
	ChargerID     uuid.UUID       `gorm:"column:charger_id"`
	AverageRating decimal.Decimal `gorm:"column:average_rating"`
	RatingCount   int64           `gorm:"column:rating_count"`
}

// JoinSQL is the canonical session-owned overall-rating relation. It is safe
// to use in a left join because it is tenant keyed as well as charger keyed.
const JoinSQL = `LEFT JOIN (
	SELECT cpo_id, charger_id,
		ROUND(AVG(overall_rating)::numeric, 2) AS average_rating,
		COUNT(*) AS rating_count
	FROM customer_ratings
	WHERE session_id IS NOT NULL
	GROUP BY cpo_id, charger_id
) AS rating_aggregate ON rating_aggregate.cpo_id = chargers.cpo_id
	AND rating_aggregate.charger_id = chargers.id`

// Load returns canonical aggregates for a bounded, already-authorized set of
// chargers. Only overall_rating from session-owned rows participates.
func Load(ctx context.Context, database *gorm.DB, cpoID uuid.UUID, chargerIDs []uuid.UUID) (map[uuid.UUID]ChargerAggregate, error) {
	result := make(map[uuid.UUID]ChargerAggregate, len(chargerIDs))
	if len(chargerIDs) == 0 {
		return result, nil
	}
	var rows []ChargerAggregate
	if err := database.WithContext(ctx).Table("customer_ratings").
		Select("charger_id, ROUND(AVG(overall_rating)::numeric, 2) AS average_rating, COUNT(*) AS rating_count").
		Where("cpo_id = ? AND session_id IS NOT NULL AND charger_id IN ?", cpoID, chargerIDs).
		Group("charger_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ChargerID] = row
	}
	return result, nil
}
