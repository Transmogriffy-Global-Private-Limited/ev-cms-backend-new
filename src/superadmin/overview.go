package superadmin

import (
	"context"
	"fmt"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

func (service *Service) Overview(ctx context.Context, principal auth.Principal) (OverviewResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return OverviewResponse{}, err
	}
	var cpoRows []struct {
		Status string
		Count  int64
	}
	if err := service.database.WithContext(ctx).Model(&models.CPO{}).Select("status, count(*) AS count").Group("status").Scan(&cpoRows).Error; err != nil {
		return OverviewResponse{}, fmt.Errorf("count CPO states: %w", err)
	}
	cpos := map[string]int64{}
	for _, row := range cpoRows {
		cpos[row.Status] = row.Count
	}
	var activeAdmins int64
	if err := service.database.WithContext(ctx).Model(&models.PlatformAdmin{}).Where("platform_admins.is_active = true").Joins("JOIN users ON users.id = platform_admins.user_id").Where("users.is_active = true").Count(&activeAdmins).Error; err != nil {
		return OverviewResponse{}, err
	}
	var activeSessions int64
	if err := service.database.WithContext(ctx).Model(&models.AuthSession{}).Where("revoked_at IS NULL AND expires_at > ?", service.now()).Count(&activeSessions).Error; err != nil {
		return OverviewResponse{}, err
	}
	var mailRows []struct {
		Status string
		Count  int64
	}
	if err := service.database.WithContext(ctx).Model(&models.MailOutbox{}).Select("status, count(*) AS count").Group("status").Scan(&mailRows).Error; err != nil {
		return OverviewResponse{}, err
	}
	mail := map[string]int64{}
	for _, row := range mailRows {
		mail[row.Status] = row.Count
	}
	workers, err := service.workerViews(ctx, principal)
	if err != nil {
		return OverviewResponse{}, err
	}
	return OverviewResponse{CPOs: cpos, ActivePlatformAdmins: activeAdmins, ActiveSessions: activeSessions, Mail: mail, Workers: workers}, nil
}

func (service *Service) Status(ctx context.Context, principal auth.Principal) (StatusResponse, error) {
	if err := requirePlatform(principal); err != nil {
		return StatusResponse{}, err
	}
	database := "ready"
	sqlDB, err := service.database.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		database = "not_ready"
	}
	workers, workerErr := service.workerViews(ctx, principal)
	if workerErr != nil {
		return StatusResponse{}, workerErr
	}
	return StatusResponse{Service: "ev-cms-backend-new", Version: Version, Database: database, Workers: workers}, nil
}

func (service *Service) workerViews(ctx context.Context, principal auth.Principal) ([]WorkerStatus, error) {
	if service.events == nil {
		return []WorkerStatus{}, nil
	}
	response, err := service.events.ListWorkers(ctx, principal)
	if err != nil {
		return nil, err
	}
	workers := make([]WorkerStatus, 0, len(response.Workers))
	for _, worker := range response.Workers {
		workers = append(workers, WorkerStatus{Name: worker.Name, Status: worker.Status, Required: worker.Required})
	}
	return workers, nil
}
