package support

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

type Service struct {
	database *gorm.DB
	now      func() time.Time
}

func NewService(database *gorm.DB) *Service {
	return &Service{database: database, now: func() time.Time { return time.Now().UTC() }}
}

type CreateRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
type ReplyRequest struct {
	Body string `json:"body"`
}
type StatusRequest struct {
	Status string `json:"status"`
}
type TicketView struct {
	models.SupportTicket
	Messages []models.SupportTicketMessage `json:"messages"`
}

func (service *Service) Create(ctx context.Context, principal auth.Principal, request CreateRequest) (TicketView, error) {
	if err := requireCPO(principal); err != nil {
		return TicketView{}, err
	}
	request.Subject, request.Body = strings.TrimSpace(request.Subject), strings.TrimSpace(request.Body)
	if request.Subject == "" || len(request.Subject) > 200 || request.Body == "" || len(request.Body) > 10000 {
		return TicketView{}, invalid()
	}
	now := service.now()
	ticket := models.SupportTicket{ID: uuid.New(), CPOID: *principal.CPOID, Subject: request.Subject, Status: "OPEN", CreatedByUserID: principal.UserID, CreatedAt: now, UpdatedAt: now}
	err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ticket).Error; err != nil {
			return err
		}
		return tx.Create(&models.SupportTicketMessage{ID: uuid.New(), TicketID: ticket.ID, AuthorUserID: principal.UserID, AuthorScope: "CPO", Body: request.Body, CreatedAt: now}).Error
	})
	if err != nil {
		return TicketView{}, fmt.Errorf("create support ticket: %w", err)
	}
	return service.Get(ctx, principal, ticket.ID)
}
func (service *Service) List(ctx context.Context, principal auth.Principal) ([]TicketView, error) {
	var tickets []models.SupportTicket
	query := service.database.WithContext(ctx).Order("updated_at DESC, id DESC")
	if principal.Scope == constants.AuthScopeCPO {
		if err := requireCPO(principal); err != nil {
			return nil, err
		}
		query = query.Where("cpo_id = ?", *principal.CPOID)
	} else if principal.Scope != constants.AuthScopePlatform {
		return nil, forbidden()
	}
	if err := query.Find(&tickets).Error; err != nil {
		return nil, fmt.Errorf("list support tickets: %w", err)
	}
	result := make([]TicketView, 0, len(tickets))
	for _, ticket := range tickets {
		view, err := service.Get(ctx, principal, ticket.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}
func (service *Service) Get(ctx context.Context, principal auth.Principal, ticketID uuid.UUID) (TicketView, error) {
	var ticket models.SupportTicket
	query := service.database.WithContext(ctx).Where("id = ?", ticketID)
	if principal.Scope == constants.AuthScopeCPO {
		if err := requireCPO(principal); err != nil {
			return TicketView{}, err
		}
		query = query.Where("cpo_id = ?", *principal.CPOID)
	} else if principal.Scope != constants.AuthScopePlatform {
		return TicketView{}, forbidden()
	}
	if err := query.First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TicketView{}, notFound()
		}
		return TicketView{}, err
	}
	var messages []models.SupportTicketMessage
	if err := service.database.WithContext(ctx).Where("ticket_id = ?", ticket.ID).Order("created_at ASC, id ASC").Find(&messages).Error; err != nil {
		return TicketView{}, err
	}
	return TicketView{SupportTicket: ticket, Messages: messages}, nil
}
func (service *Service) Reply(ctx context.Context, principal auth.Principal, ticketID uuid.UUID, request ReplyRequest) (TicketView, error) {
	request.Body = strings.TrimSpace(request.Body)
	if request.Body == "" || len(request.Body) > 10000 {
		return TicketView{}, invalid()
	}
	ticket, err := service.Get(ctx, principal, ticketID)
	if err != nil {
		return TicketView{}, err
	}
	scope := string(principal.Scope)
	now := service.now()
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.SupportTicketMessage{ID: uuid.New(), TicketID: ticket.ID, AuthorUserID: principal.UserID, AuthorScope: scope, Body: request.Body, CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.SupportTicket{}).Where("id = ?", ticket.ID).Updates(map[string]any{"updated_at": now, "status": "PENDING"}).Error
	}); err != nil {
		return TicketView{}, err
	}
	return service.Get(ctx, principal, ticketID)
}
func (service *Service) SetStatus(ctx context.Context, principal auth.Principal, ticketID uuid.UUID, request StatusRequest) (TicketView, error) {
	if principal.Scope != constants.AuthScopePlatform {
		return TicketView{}, forbidden()
	}
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	if status != "OPEN" && status != "PENDING" && status != "RESOLVED" && status != "CLOSED" {
		return TicketView{}, invalid()
	}
	now := service.now()
	updates := map[string]any{"status": status, "updated_at": now}
	if status == "CLOSED" {
		updates["closed_at"] = now
	}
	if err := service.database.WithContext(ctx).Model(&models.SupportTicket{}).Where("id=?", ticketID).Updates(updates).Error; err != nil {
		return TicketView{}, err
	}
	return service.Get(ctx, principal, ticketID)
}
func requireCPO(p auth.Principal) error {
	if p.Scope != constants.AuthScopeCPO || p.CPOID == nil || p.Role == nil {
		return forbidden()
	}
	return nil
}
func forbidden() error {
	return &auth.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "You do not have access to this operation."}
}
func notFound() error {
	return &auth.APIError{Status: http.StatusNotFound, Code: "support_ticket_not_found", Message: "The support ticket was not found."}
}
func invalid() error {
	return &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The support ticket request is invalid."}
}
