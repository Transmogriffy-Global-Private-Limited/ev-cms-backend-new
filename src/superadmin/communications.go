package superadmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (service *Service) CreateAnnouncement(ctx context.Context, principal auth.Principal, request AnnouncementRequest) (AnnouncementView, error) {
	if err := requirePlatform(principal); err != nil {
		return AnnouncementView{}, err
	}
	audience := strings.ToUpper(strings.TrimSpace(request.Audience))
	if audience != "PLATFORM" && audience != "CPO" {
		return AnnouncementView{}, invalid("audience", "Audience must be PLATFORM or CPO.")
	}
	cpoIDs := uniqueAnnouncementCPOIDs(request.CPOID, request.CPOIDs)
	if audience == "PLATFORM" && len(cpoIDs) != 0 {
		return AnnouncementView{}, invalid("cpo_ids", "CPO targets are not allowed for PLATFORM audience.")
	}
	if audience == "CPO" && len(cpoIDs) == 0 {
		return AnnouncementView{}, invalid("cpo_ids", "At least one CPO target is required for CPO audience.")
	}
	title := strings.TrimSpace(request.Title)
	body := strings.TrimSpace(request.Body)
	if title == "" || len(title) > maxTitle {
		return AnnouncementView{}, invalid("title", "Title is required and must not exceed 200 characters.")
	}
	if body == "" || len(body) > maxBody {
		return AnnouncementView{}, invalid("body", "Body is required and must not exceed 10000 characters.")
	}
	if request.ExpiresAt != nil && !request.ExpiresAt.After(service.now()) {
		return AnnouncementView{}, invalid("expires_at", "expires_at must be in the future.")
	}
	var view AnnouncementView
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := service.now()
		if len(cpoIDs) > 0 {
			var count int64
			if err := tx.Model(&models.CPO{}).Where("id IN ?", cpoIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(cpoIDs)) {
				return &auth.APIError{Status: http.StatusNotFound, Code: "cpo_not_found", Message: "One or more CPO targets were not found."}
			}
		}
		var legacyCPOID *uuid.UUID
		if len(cpoIDs) == 1 {
			legacyCPOID = &cpoIDs[0]
		}
		record := models.PlatformAnnouncement{ID: uuid.New(), Audience: audience, CPOID: legacyCPOID, Title: title, Body: body, CreatedByUserID: principal.UserID, CreatedAt: now, ExpiresAt: request.ExpiresAt}
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("create announcement: %w", err)
		}
		if len(cpoIDs) > 0 {
			targets := make([]models.PlatformAnnouncementCPO, 0, len(cpoIDs))
			for _, cpoID := range cpoIDs {
				targets = append(targets, models.PlatformAnnouncementCPO{AnnouncementID: record.ID, CPOID: cpoID, CreatedAt: now})
			}
			if err := tx.Create(&targets).Error; err != nil {
				return fmt.Errorf("snapshot announcement CPO targets: %w", err)
			}
		}
		type recipient struct {
			UserID uuid.UUID
			CPOID  *uuid.UUID
		}
		recipients := make([]recipient, 0)
		if audience == "PLATFORM" {
			var recipientIDs []uuid.UUID
			if err := tx.Model(&models.PlatformAdmin{}).Where("platform_admins.is_active = true").Joins("JOIN users ON users.id = platform_admins.user_id").Where("users.is_active = true").Pluck("platform_admins.user_id", &recipientIDs).Error; err != nil {
				return err
			}
			for _, recipientID := range recipientIDs {
				recipients = append(recipients, recipient{UserID: recipientID})
			}
		} else {
			for _, cpoID := range cpoIDs {
				var recipientIDs []uuid.UUID
				if err := tx.Model(&models.CPOMembership{}).Where("cpo_id = ? AND status = ?", cpoID, constants.MembershipStatusActive).Joins("JOIN users ON users.id = cpo_memberships.user_id").Where("users.is_active = true").Pluck("cpo_memberships.user_id", &recipientIDs).Error; err != nil {
					return err
				}
				for _, recipientID := range recipientIDs {
					target := cpoID
					recipients = append(recipients, recipient{UserID: recipientID, CPOID: &target})
				}
			}
		}
		notifications := make([]models.PlatformNotification, 0, len(recipients))
		for _, recipient := range recipients {
			notifications = append(notifications, models.PlatformNotification{ID: uuid.New(), AnnouncementID: record.ID, RecipientUserID: recipient.UserID, CPOID: recipient.CPOID, CreatedAt: now})
		}
		if len(notifications) > 0 {
			if err := tx.Create(&notifications).Error; err != nil {
				return fmt.Errorf("create announcement notifications: %w", err)
			}
		}
		data := models.JSONB{"audience": audience, "recipient_count": len(recipients), "cpo_count": len(cpoIDs)}
		if err := service.audit(tx, principal.UserID, "PLATFORM_ANNOUNCEMENT_CREATED", "PLATFORM_ANNOUNCEMENT", &record.ID, data, now); err != nil {
			return err
		}
		if err := service.emit(tx, principal.UserID, "platform.announcement.created", "PLATFORM_ANNOUNCEMENT", record.ID, data); err != nil {
			return err
		}
		view = viewAnnouncement(record, int64(len(recipients)))
		view.CPOIDs = append([]uuid.UUID(nil), cpoIDs...)
		return nil
	})
	return view, err
}

func (service *Service) ListAnnouncements(ctx context.Context, principal auth.Principal, query PageQuery) (AnnouncementPage, error) {
	if err := requirePlatform(principal); err != nil {
		return AnnouncementPage{}, err
	}
	page, err := pageQuery(query)
	if err != nil {
		return AnnouncementPage{}, err
	}
	database := service.database.WithContext(ctx).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("created_at < ?", page.Before.UTC())
		} else {
			database = database.Where("(created_at < ?) OR (created_at = ? AND id < ?)", page.Before.UTC(), page.Before.UTC(), *page.BeforeID)
		}
	}
	var records []models.PlatformAnnouncement
	if err := database.Find(&records).Error; err != nil {
		return AnnouncementPage{}, fmt.Errorf("list announcements: %w", err)
	}
	hasMore := len(records) > page.Limit
	if hasMore {
		records = records[:page.Limit]
	}
	views := make([]AnnouncementView, 0, len(records))
	for _, record := range records {
		var count int64
		if err := service.database.WithContext(ctx).Model(&models.PlatformNotification{}).Where("announcement_id = ?", record.ID).Count(&count).Error; err != nil {
			return AnnouncementPage{}, err
		}
		view := viewAnnouncement(record, count)
		view.CPOIDs, err = service.announcementCPOIDs(ctx, record.ID)
		if err != nil {
			return AnnouncementPage{}, err
		}
		views = append(views, view)
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(views) > 0 {
		next = &views[len(views)-1].CreatedAt
		nextID = &views[len(views)-1].ID
	}
	return AnnouncementPage{Announcements: views, NextBefore: next, NextBeforeID: nextID, HasMore: hasMore}, nil
}

func uniqueAnnouncementCPOIDs(legacy *uuid.UUID, values []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	result := make([]uuid.UUID, 0, len(values)+1)
	add := func(id uuid.UUID) {
		if id != uuid.Nil {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	if legacy != nil {
		add(*legacy)
	}
	for _, id := range values {
		add(id)
	}
	return result
}

func (service *Service) announcementCPOIDs(ctx context.Context, announcementID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := service.database.WithContext(ctx).Model(&models.PlatformAnnouncementCPO{}).Where("announcement_id = ?", announcementID).Order("cpo_id ASC").Pluck("cpo_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list announcement CPO targets: %w", err)
	}
	return ids, nil
}

func (service *Service) ListNotifications(ctx context.Context, principal auth.Principal, query PageQuery, unreadOnly bool) (NotificationPage, error) {
	if principal.Scope != constants.AuthScopePlatform && principal.Scope != constants.AuthScopeCPO {
		return NotificationPage{}, &auth.APIError{Status: 403, Code: "forbidden", Message: "You do not have access to this operation."}
	}
	page, err := pageQuery(query)
	if err != nil {
		return NotificationPage{}, err
	}
	database := service.database.WithContext(ctx).Model(&models.PlatformNotification{}).
		Joins("JOIN platform_announcements ON platform_announcements.id = platform_notifications.announcement_id").
		Where("platform_notifications.recipient_user_id = ?", principal.UserID).
		Where("(platform_announcements.expires_at IS NULL OR platform_announcements.expires_at > ?)", service.now()).
		Order("platform_notifications.created_at DESC, platform_notifications.id DESC").Limit(page.Limit + 1)
	if principal.Scope == constants.AuthScopeCPO {
		if principal.CPOID == nil {
			return NotificationPage{}, errors.New("CPO principal has no CPO")
		}
		database = database.Where("platform_notifications.cpo_id = ?", *principal.CPOID)
	} else {
		database = database.Where("platform_notifications.cpo_id IS NULL")
	}
	if unreadOnly {
		database = database.Where("platform_notifications.read_at IS NULL")
	}
	if page.Before != nil {
		if page.BeforeID == nil {
			database = database.Where("platform_notifications.created_at < ?", page.Before.UTC())
		} else {
			database = database.Where("(platform_notifications.created_at < ?) OR (platform_notifications.created_at = ? AND platform_notifications.id < ?)", page.Before.UTC(), page.Before.UTC(), *page.BeforeID)
		}
	}
	var rows []struct {
		models.PlatformNotification
		Audience  string
		Title     string
		Body      string
		ExpiresAt *time.Time
	}
	if err := database.Select("platform_notifications.*, platform_announcements.audience, platform_announcements.title, platform_announcements.body, platform_announcements.expires_at").Find(&rows).Error; err != nil {
		return NotificationPage{}, fmt.Errorf("list notifications: %w", err)
	}
	hasMore := len(rows) > page.Limit
	if hasMore {
		rows = rows[:page.Limit]
	}
	views := make([]NotificationView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NotificationView{ID: row.ID, AnnouncementID: row.AnnouncementID, Audience: row.Audience, CPOID: row.CPOID, Title: row.Title, Body: row.Body, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, ReadAt: row.ReadAt})
	}
	var next *time.Time
	var nextID *uuid.UUID
	if hasMore && len(views) > 0 {
		next = &views[len(views)-1].CreatedAt
		nextID = &views[len(views)-1].ID
	}
	return NotificationPage{Notifications: views, NextBefore: next, NextBeforeID: nextID, HasMore: hasMore}, nil
}

func (service *Service) MarkNotificationRead(ctx context.Context, principal auth.Principal, notificationID uuid.UUID) error {
	if principal.Scope != constants.AuthScopePlatform && principal.Scope != constants.AuthScopeCPO {
		return &auth.APIError{Status: 403, Code: "forbidden", Message: "You do not have access to this operation."}
	}
	if notificationID == uuid.Nil {
		return invalid("notification_id", "Notification ID is invalid.")
	}
	now := service.now()
	query := service.database.WithContext(ctx).Model(&models.PlatformNotification{}).Where("id = ? AND recipient_user_id = ? AND read_at IS NULL", notificationID, principal.UserID)
	if principal.Scope == constants.AuthScopeCPO {
		if principal.CPOID == nil {
			return errors.New("CPO principal has no CPO")
		}
		query = query.Where("cpo_id = ?", *principal.CPOID)
	} else {
		query = query.Where("cpo_id IS NULL")
	}
	result := query.Update("read_at", now)
	if result.Error != nil {
		return fmt.Errorf("mark notification read: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return &auth.APIError{Status: 404, Code: "notification_not_found", Message: "The notification was not found."}
	}
	return nil
}
