package superadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/google/uuid"
)

func (service *Service) ListSecurityEvents(ctx context.Context, principal auth.Principal, query PageQuery) (platformops.AuditPage, error) {
	if err := requirePlatform(principal); err != nil {
		return platformops.AuditPage{}, err
	}
	page, err := pageQuery(query)
	if err != nil {
		return platformops.AuditPage{}, err
	}
	database := service.database.WithContext(ctx).
		Where("(action LIKE 'AUTH_%' OR action LIKE 'SECURITY_%')").
		Order("created_at DESC, id DESC").
		Limit(page.Limit + 1)
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("created_at < ?", page.Before.UTC())
		} else {
			database = database.Where(
				"(created_at < ?) OR (created_at = ? AND id < ?)",
				page.Before.UTC(), page.Before.UTC(), *page.BeforeID,
			)
		}
	}
	var records []models.AuditLog
	if err := database.Find(&records).Error; err != nil {
		return platformops.AuditPage{}, fmt.Errorf("list security events: %w", err)
	}
	hasMore := len(records) > page.Limit
	if hasMore {
		records = records[:page.Limit]
	}
	var nextTime *time.Time
	var nextID *uuid.UUID
	if hasMore && len(records) > 0 {
		timestamp := records[len(records)-1].CreatedAt
		id := records[len(records)-1].ID
		nextTime = &timestamp
		nextID = &id
	}
	return platformops.AuditPage{Records: records, NextBefore: nextTime, NextBeforeID: nextID, HasMore: hasMore}, nil
}
