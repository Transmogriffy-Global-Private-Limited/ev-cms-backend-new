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
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key"`
}
type StatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type TicketView struct {
	models.SupportTicket
	Messages []models.SupportTicketMessage `json:"messages"`
	Events   []models.SupportTicketEvent   `json:"events"`
}
type TicketSummary struct {
	ID               uuid.UUID  `json:"id"`
	CPOID            uuid.UUID  `json:"cpo_id"`
	CPOName          string     `json:"cpo_name"`
	Subject          string     `json:"subject"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	MessageCount     int64      `json:"message_count"`
	LastMessageAt    *time.Time `json:"last_message_at,omitempty"`
	LastMessageScope *string    `json:"last_message_scope,omitempty"`
}

type ListQuery struct {
	Limit    int
	Before   *time.Time
	BeforeID *uuid.UUID
	Status   string
	CPOID    *uuid.UUID
	Search   string
}

type TicketListPage struct {
	Tickets      []TicketSummary `json:"tickets"`
	NextBefore   *time.Time      `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID      `json:"next_before_id,omitempty"`
	HasMore      bool            `json:"has_more"`
}

func (service *Service) Create(ctx context.Context, principal auth.Principal, request CreateRequest) (TicketView, error) {
	if err := service.requireCPOPermission(ctx, principal, cpopermissions.SupportCreate); err != nil {
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
		if err := tx.Create(&models.SupportTicketMessage{ID: uuid.New(), TicketID: ticket.ID, AuthorUserID: principal.UserID, AuthorScope: "CPO", Body: request.Body, CreatedAt: now}).Error; err != nil {
			return err
		}
		return event(tx, ticket.ID, "CREATED", "CPO", &principal.UserID, nil, nil, "", "", now)
	})
	if err != nil {
		return TicketView{}, fmt.Errorf("create support ticket: %w", err)
	}
	return service.Get(ctx, principal, ticket.ID)
}
func (service *Service) List(ctx context.Context, principal auth.Principal, request ListQuery) (TicketListPage, error) {
	if principal.Scope == constants.AuthScopeCPO {
		if err := service.requireCPOPermission(ctx, principal, cpopermissions.SupportRead); err != nil {
			return TicketListPage{}, err
		}
	} else if principal.Scope != constants.AuthScopePlatform {
		return TicketListPage{}, forbidden()
	}

	request, err := normalizeListQuery(request)
	if err != nil {
		return TicketListPage{}, err
	}

	// This is intentionally a scalar-summary query. It never joins or emits a
	// message body, so the support queue remains bounded even for old tickets.
	query := service.database.WithContext(ctx).Table("support_tickets").
		Select(`support_tickets.id, support_tickets.cpo_id, cpos.business_name AS cpo_name,
			support_tickets.subject, support_tickets.status, support_tickets.created_at, support_tickets.updated_at,
			(SELECT count(*) FROM support_ticket_messages message_count WHERE message_count.ticket_id = support_tickets.id) AS message_count,
			(SELECT created_at FROM support_ticket_messages last_message WHERE last_message.ticket_id = support_tickets.id ORDER BY created_at DESC, id DESC LIMIT 1) AS last_message_at,
			(SELECT author_scope FROM support_ticket_messages last_message WHERE last_message.ticket_id = support_tickets.id ORDER BY created_at DESC, id DESC LIMIT 1) AS last_message_scope`).
		Joins("JOIN cpos ON cpos.id = support_tickets.cpo_id").
		Order("support_tickets.updated_at DESC, support_tickets.id DESC").
		Limit(request.Limit + 1)

	if principal.Scope == constants.AuthScopeCPO {
		query = query.Where("support_tickets.cpo_id = ?", *principal.CPOID)
	} else if request.CPOID != nil {
		query = query.Where("support_tickets.cpo_id = ?", *request.CPOID)
	}
	if request.Status != "" {
		query = query.Where("support_tickets.status = ?", request.Status)
	}
	if request.Search != "" {
		pattern := "%" + request.Search + "%"
		query = query.Where("(support_tickets.subject ILIKE ? OR CAST(support_tickets.id AS text) ILIKE ?)", pattern, pattern)
	}
	if request.Before != nil {
		query = query.Where("(support_tickets.updated_at, support_tickets.id) < (?, ?)", *request.Before, *request.BeforeID)
	}

	var tickets []TicketSummary
	if err := query.Find(&tickets).Error; err != nil {
		return TicketListPage{}, fmt.Errorf("list support tickets: %w", err)
	}
	page := TicketListPage{Tickets: tickets}
	if page.Tickets == nil {
		page.Tickets = []TicketSummary{}
	}
	if len(page.Tickets) > request.Limit {
		page.HasMore = true
		page.Tickets = page.Tickets[:request.Limit]
	}
	if page.HasMore {
		last := page.Tickets[len(page.Tickets)-1]
		before, beforeID := last.UpdatedAt, last.ID
		page.NextBefore, page.NextBeforeID = &before, &beforeID
	}
	return page, nil
}
func (service *Service) Get(ctx context.Context, principal auth.Principal, ticketID uuid.UUID) (TicketView, error) {
	if principal.Scope == constants.AuthScopeCPO {
		if err := service.requireCPOPermission(ctx, principal, cpopermissions.SupportRead); err != nil {
			return TicketView{}, err
		}
	} else if principal.Scope != constants.AuthScopePlatform {
		return TicketView{}, forbidden()
	}
	return service.loadTicket(ctx, principal, ticketID)
}

func (service *Service) loadTicket(ctx context.Context, principal auth.Principal, ticketID uuid.UUID) (TicketView, error) {
	var ticket models.SupportTicket
	query := service.database.WithContext(ctx).Where("id = ?", ticketID)
	if principal.Scope == constants.AuthScopeCPO {
		query = query.Where("cpo_id = ?", *principal.CPOID)
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
	var events []models.SupportTicketEvent
	if err := service.database.WithContext(ctx).Where("ticket_id = ?", ticket.ID).Order("created_at ASC, id ASC").Find(&events).Error; err != nil {
		return TicketView{}, err
	}
	return TicketView{SupportTicket: ticket, Messages: messages, Events: events}, nil
}
func (service *Service) Reply(ctx context.Context, principal auth.Principal, ticketID uuid.UUID, request ReplyRequest) (TicketView, error) {
	if principal.Scope == constants.AuthScopeCPO {
		if err := service.requireCPOPermission(ctx, principal, cpopermissions.SupportReply); err != nil {
			return TicketView{}, err
		}
	}
	if principal.Scope != constants.AuthScopeCPO && principal.Scope != constants.AuthScopePlatform {
		return TicketView{}, forbidden()
	}
	request.Body = strings.TrimSpace(request.Body)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Body == "" || len(request.Body) > 10000 || len(request.IdempotencyKey) > 120 {
		return TicketView{}, invalid()
	}
	scope := string(principal.Scope)
	now := service.now()
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked models.SupportTicket
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", ticketID)
		if principal.Scope == constants.AuthScopeCPO {
			query = query.Where("cpo_id = ?", *principal.CPOID)
		}
		if err := query.First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound()
			}
			return err
		}
		if request.IdempotencyKey != "" {
			var existing models.SupportTicketEvent
			if err := tx.Where("ticket_id = ? AND idempotency_key = ?", locked.ID, request.IdempotencyKey).First(&existing).Error; err == nil {
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		previous := locked.Status
		next := previous
		if principal.Scope == constants.AuthScopeCPO && (previous == "RESOLVED" || previous == "CLOSED") {
			next = "OPEN"
		}
		if err := tx.Create(&models.SupportTicketMessage{ID: uuid.New(), TicketID: locked.ID, AuthorUserID: principal.UserID, AuthorScope: scope, Body: request.Body, CreatedAt: now}).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": now, "status": next}
		if next != "CLOSED" {
			updates["closed_at"] = nil
		}
		if err := tx.Model(&models.SupportTicket{}).Where("id = ?", locked.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := event(tx, locked.ID, "MESSAGE_ADDED", scope, &principal.UserID, nil, nil, "", request.IdempotencyKey, now); err != nil {
			return err
		}
		if next != previous {
			return event(tx, locked.ID, "STATUS_CHANGED", scope, &principal.UserID, &previous, &next, "", "", now)
		}
		return nil
	}); err != nil {
		return TicketView{}, err
	}
	return service.loadTicket(ctx, principal, ticketID)
}
func (service *Service) SetStatus(ctx context.Context, principal auth.Principal, ticketID uuid.UUID, request StatusRequest) (TicketView, error) {
	if principal.Scope != constants.AuthScopePlatform {
		return TicketView{}, forbidden()
	}
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	if status != "OPEN" && status != "IN_PROGRESS" && status != "RESOLVED" && status != "CLOSED" {
		return TicketView{}, invalid()
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if len(request.Reason) > 500 {
		return TicketView{}, invalid()
	}
	now := service.now()
	if err := service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket models.SupportTicket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, "id = ?", ticketID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound()
			}
			return err
		}
		previous := ticket.Status
		if previous == status {
			return nil
		}
		if !validTransition(previous, status) {
			return invalidTransition()
		}
		updates := map[string]any{"status": status, "updated_at": now}
		if status == "CLOSED" {
			updates["closed_at"] = now
		} else {
			updates["closed_at"] = nil
		}
		if err := tx.Model(&ticket).Updates(updates).Error; err != nil {
			return err
		}
		return event(tx, ticket.ID, "STATUS_CHANGED", "PLATFORM", &principal.UserID, &previous, &status, request.Reason, "", now)
	}); err != nil {
		return TicketView{}, err
	}
	return service.Get(ctx, principal, ticketID)
}

func validTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case "OPEN":
		return to == "IN_PROGRESS" || to == "RESOLVED" || to == "CLOSED"
	case "IN_PROGRESS":
		return to == "RESOLVED" || to == "CLOSED"
	case "RESOLVED":
		return to == "CLOSED" || to == "OPEN"
	case "CLOSED":
		return to == "OPEN"
	}
	return false
}

func normalizeListQuery(request ListQuery) (ListQuery, error) {
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > 100 {
		return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_limit", Message: "Limit must be a number between 1 and 100."}
	}
	if (request.Before == nil) != (request.BeforeID == nil) {
		return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_cursor", Message: "Both before and before_id are required for support cursor pagination."}
	}
	request.Status = strings.ToUpper(strings.TrimSpace(request.Status))
	if request.Status != "" && !isStatus(request.Status) {
		return ListQuery{}, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_support_status", Message: "The support status filter is invalid."}
	}
	request.Search = strings.TrimSpace(request.Search)
	if len(request.Search) > 100 {
		return ListQuery{}, invalid()
	}
	return request, nil
}

func isStatus(status string) bool {
	return status == "OPEN" || status == "IN_PROGRESS" || status == "RESOLVED" || status == "CLOSED"
}

func event(tx *gorm.DB, ticketID uuid.UUID, typ, scope string, actor *uuid.UUID, previous, next *string, reason, key string, now time.Time) error {
	var r *string
	if reason != "" {
		r = &reason
	}
	var k *string
	if key != "" {
		k = &key
	}
	return tx.Create(&models.SupportTicketEvent{ID: uuid.New(), TicketID: ticketID, EventType: typ, ActorScope: scope, ActorUserID: actor, PreviousStatus: previous, NextStatus: next, Reason: r, IdempotencyKey: k, CreatedAt: now}).Error
}
func requireCPO(p auth.Principal) error {
	if p.Scope != constants.AuthScopeCPO || p.CPOID == nil || p.Role == nil {
		return forbidden()
	}
	return nil
}

func (service *Service) requireCPOPermission(ctx context.Context, principal auth.Principal, permission string) error {
	if err := requireCPO(principal); err != nil {
		return err
	}
	_, allowed, err := auth.EvaluateCPOPermission(ctx, service.database, principal, permission)
	if err != nil || !allowed {
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

func invalidTransition() error {
	return &auth.APIError{Status: http.StatusConflict, Code: "invalid_support_transition", Message: "The requested support status transition is not allowed."}
}
