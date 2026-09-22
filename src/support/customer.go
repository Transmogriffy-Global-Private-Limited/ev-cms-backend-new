package support

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const customerCPOChannel = "CUSTOMER_CPO"

type CustomerIdentityView struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
}
type CustomerMessageView struct {
	ID           uuid.UUID  `json:"id"`
	AuthorScope  string     `json:"author_scope"`
	Body         string     `json:"body"`
	CreatedAt    time.Time  `json:"created_at"`
	AuthorUserID *uuid.UUID `json:"author_user_id,omitempty"`
}
type CustomerEventView struct {
	ID             uuid.UUID  `json:"id"`
	EventType      string     `json:"event_type"`
	ActorScope     string     `json:"actor_scope"`
	PreviousStatus *string    `json:"previous_status,omitempty"`
	NextStatus     *string    `json:"next_status,omitempty"`
	Reason         *string    `json:"reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ActorUserID    *uuid.UUID `json:"actor_user_id,omitempty"`
}
type CustomerTicketView struct {
	ID         uuid.UUID             `json:"id"`
	CPOID      uuid.UUID             `json:"cpo_id"`
	CustomerID uuid.UUID             `json:"customer_id"`
	Subject    string                `json:"subject"`
	Status     string                `json:"status"`
	ClosedAt   *time.Time            `json:"closed_at,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Customer   *CustomerIdentityView `json:"customer,omitempty"`
	Messages   []CustomerMessageView `json:"messages"`
	Events     []CustomerEventView   `json:"events"`
}
type CustomerTicketSummary struct {
	ID         uuid.UUID             `json:"id"`
	CustomerID uuid.UUID             `json:"customer_id,omitempty"`
	Subject    string                `json:"subject"`
	Status     string                `json:"status"`
	UpdatedAt  time.Time             `json:"updated_at"`
	CreatedAt  time.Time             `json:"created_at"`
	Customer   *CustomerIdentityView `json:"customer,omitempty"`
}
type CustomerTicketPage struct {
	Tickets      []CustomerTicketSummary `json:"tickets"`
	NextBefore   *time.Time              `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID              `json:"next_before_id,omitempty"`
	HasMore      bool                    `json:"has_more"`
}

func validCustomerRequest(r CreateRequest) error {
	r.Subject = strings.TrimSpace(r.Subject)
	r.Body = strings.TrimSpace(r.Body)
	if r.Subject == "" || len(r.Subject) > 200 || r.Body == "" || len(r.Body) > 10000 {
		return invalid()
	}
	return nil
}
func validCustomerReply(r ReplyRequest) error {
	if strings.TrimSpace(r.Body) == "" || len(strings.TrimSpace(r.Body)) > 10000 || strings.TrimSpace(r.IdempotencyKey) == "" || len(strings.TrimSpace(r.IdempotencyKey)) > 120 {
		return invalid()
	}
	return nil
}

func (s *Service) CreateCustomer(ctx context.Context, p customerauth.Principal, r CreateRequest) (CustomerTicketView, error) {
	if err := validCustomerRequest(r); err != nil {
		return CustomerTicketView{}, err
	}
	r.Subject = strings.TrimSpace(r.Subject)
	r.Body = strings.TrimSpace(r.Body)
	now := s.now()
	id := uuid.New()
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO support_tickets (id,cpo_id,channel,customer_id,subject,status,created_at,updated_at) VALUES (?,?, 'CUSTOMER_CPO', ?,?,'OPEN',?,?)`, id, p.CPOID, p.CustomerID, r.Subject, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_customer_id,author_scope,body,created_at) VALUES (?,?,?,'CUSTOMER',?,?)`, uuid.New(), id, p.CustomerID, r.Body, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,actor_customer_id,created_at) VALUES (?,?,'CREATED','CUSTOMER',?,?)`, uuid.New(), id, p.CustomerID, now).Error; err != nil {
			return err
		}
		return s.notifyCustomerCPO(tx, id, "CUSTOMER_CPO_SUPPORT_TICKET_CREATED", true, now)
	})
	if err != nil {
		return CustomerTicketView{}, fmt.Errorf("create customer support ticket: %w", err)
	}
	return s.customerTicket(ctx, id, &p.CPOID, &p.CustomerID, false, false)
}

func (s *Service) ListCustomer(ctx context.Context, p customerauth.Principal, q ListQuery) (CustomerTicketPage, error) {
	return s.listCustomerTickets(ctx, p.CPOID, &p.CustomerID, q, false)
}
func (s *Service) ListCPOCustomer(ctx context.Context, p auth.Principal, q ListQuery) (CustomerTicketPage, error) {
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportRead); err != nil {
		return CustomerTicketPage{}, err
	}
	return s.listCustomerTickets(ctx, *p.CPOID, nil, q, true)
}
func (s *Service) listCustomerTickets(ctx context.Context, cpoID uuid.UUID, customerID *uuid.UUID, q ListQuery, includeCustomer bool) (CustomerTicketPage, error) {
	q.CPOID = nil
	var err error
	if q, err = normalizeListQuery(q); err != nil {
		return CustomerTicketPage{}, err
	}
	query := s.database.WithContext(ctx).Table("support_tickets").Select("support_tickets.id,support_tickets.customer_id,support_tickets.subject,support_tickets.status,support_tickets.created_at,support_tickets.updated_at, customers.full_name AS customer_full_name, customers.email AS customer_email").Joins("JOIN customers ON customers.id=support_tickets.customer_id AND customers.cpo_id=support_tickets.cpo_id").Where("support_tickets.cpo_id=? AND support_tickets.channel=?", cpoID, customerCPOChannel).Order("support_tickets.updated_at DESC,support_tickets.id DESC").Limit(q.Limit + 1)
	if customerID != nil {
		query = query.Where("support_tickets.customer_id=?", *customerID)
	} else if q.CustomerID != nil {
		query = query.Where("support_tickets.customer_id=?", *q.CustomerID)
	}
	if q.Status != "" {
		query = query.Where("support_tickets.status=?", q.Status)
	}
	if q.Search != "" {
		v := "%" + q.Search + "%"
		query = query.Where("support_tickets.subject ILIKE ? OR CAST(support_tickets.id AS text) ILIKE ? OR customers.full_name ILIKE ? OR customers.email ILIKE ?", v, v, v, v)
	}
	if q.Before != nil {
		query = query.Where("(support_tickets.updated_at,support_tickets.id)<(?,?)", *q.Before, *q.BeforeID)
	}
	var rows []struct {
		CustomerTicketSummary
		CustomerFullName string
		CustomerEmail    string
	}
	if err := query.Find(&rows).Error; err != nil {
		return CustomerTicketPage{}, err
	}
	page := CustomerTicketPage{Tickets: make([]CustomerTicketSummary, 0, len(rows))}
	for _, row := range rows {
		if includeCustomer {
			row.Customer = &CustomerIdentityView{ID: row.CustomerID, FullName: row.CustomerFullName, Email: row.CustomerEmail}
		}
		page.Tickets = append(page.Tickets, row.CustomerTicketSummary)
	}
	if len(page.Tickets) > q.Limit {
		page.HasMore = true
		page.Tickets = page.Tickets[:q.Limit]
	}
	if page.HasMore {
		last := page.Tickets[len(page.Tickets)-1]
		page.NextBefore = &last.UpdatedAt
		page.NextBeforeID = &last.ID
	}
	return page, nil
}

func (s *Service) GetCustomer(ctx context.Context, p customerauth.Principal, id uuid.UUID) (CustomerTicketView, error) {
	return s.customerTicket(ctx, id, &p.CPOID, &p.CustomerID, false, false)
}
func (s *Service) GetCPOCustomer(ctx context.Context, p auth.Principal, id uuid.UUID) (CustomerTicketView, error) {
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportRead); err != nil {
		return CustomerTicketView{}, err
	}
	return s.customerTicket(ctx, id, p.CPOID, nil, true, true)
}
func (s *Service) customerTicket(ctx context.Context, id uuid.UUID, cpoID, customerID *uuid.UUID, includeCustomer, includeStaffIdentity bool) (CustomerTicketView, error) {
	var v CustomerTicketView
	q := s.database.WithContext(ctx).Table("support_tickets").Select("support_tickets.id,support_tickets.cpo_id,support_tickets.customer_id,support_tickets.subject,support_tickets.status,support_tickets.closed_at,support_tickets.created_at,support_tickets.updated_at").Where("support_tickets.id=? AND support_tickets.channel=?", id, customerCPOChannel)
	if cpoID != nil {
		q = q.Where("support_tickets.cpo_id=?", *cpoID)
	}
	if customerID != nil {
		q = q.Where("support_tickets.customer_id=?", *customerID)
	}
	if err := q.Scan(&v).Error; err != nil {
		return CustomerTicketView{}, err
	}
	if v.ID == uuid.Nil {
		return CustomerTicketView{}, notFound()
	}
	if includeCustomer {
		var x CustomerIdentityView
		if err := s.database.WithContext(ctx).Table("customers").Select("id,full_name,email").Where("id=? AND cpo_id=?", v.CustomerID, v.CPOID).Scan(&x).Error; err != nil {
			return CustomerTicketView{}, err
		}
		v.Customer = &x
	}
	messageSelect := "id,author_scope,body,created_at"
	eventSelect := "id,event_type,actor_scope,previous_status,next_status,created_at"
	if includeStaffIdentity {
		messageSelect += ",author_user_id"
		eventSelect += ",reason,actor_user_id"
	}
	if err := s.database.WithContext(ctx).Table("support_ticket_messages").Select(messageSelect).Where("ticket_id=?", id).Order("created_at,id").Scan(&v.Messages).Error; err != nil {
		return CustomerTicketView{}, err
	}
	if err := s.database.WithContext(ctx).Table("support_ticket_events").Select(eventSelect).Where("ticket_id=?", id).Order("created_at,id").Scan(&v.Events).Error; err != nil {
		return CustomerTicketView{}, err
	}
	if v.Messages == nil {
		v.Messages = []CustomerMessageView{}
	}
	if v.Events == nil {
		v.Events = []CustomerEventView{}
	}
	return v, nil
}

func (s *Service) ReplyCustomer(ctx context.Context, p customerauth.Principal, id uuid.UUID, r ReplyRequest) (CustomerTicketView, error) {
	if err := validCustomerReply(r); err != nil {
		return CustomerTicketView{}, err
	}
	return s.customerReply(ctx, p.CPOID, p.CustomerID, uuid.Nil, id, r, false)
}
func (s *Service) ReplyCPOCustomer(ctx context.Context, p auth.Principal, id uuid.UUID, r ReplyRequest) (CustomerTicketView, error) {
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportRead); err != nil {
		return CustomerTicketView{}, err
	}
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportReply); err != nil {
		return CustomerTicketView{}, err
	}
	if err := validCustomerReply(r); err != nil {
		return CustomerTicketView{}, err
	}
	return s.customerReply(ctx, *p.CPOID, uuid.Nil, p.UserID, id, r, true)
}
func (s *Service) customerReply(ctx context.Context, cpoID, customerID, userID uuid.UUID, id uuid.UUID, r ReplyRequest, asCPO bool) (CustomerTicketView, error) {
	now := s.now()
	r.Body = strings.TrimSpace(r.Body)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t struct {
			ID         uuid.UUID
			Status     string
			CustomerID uuid.UUID
			Subject    string
		}
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("support_tickets").Select("id,status,customer_id,subject").Where("id=? AND cpo_id=? AND channel=?", id, cpoID, customerCPOChannel)
		if !asCPO {
			q = q.Where("customer_id=?", customerID)
		}
		if err := q.Scan(&t).Error; err != nil {
			return err
		}
		if t.ID == uuid.Nil {
			return notFound()
		}
		var existing uuid.UUID
		if err := tx.Table("support_ticket_events").Select("id").Where("ticket_id=? AND idempotency_key=?", id, r.IdempotencyKey).Scan(&existing).Error; err != nil {
			return err
		}
		if existing != uuid.Nil {
			return nil
		}
		next := t.Status
		if !asCPO && (next == "RESOLVED" || next == "CLOSED") {
			next = "OPEN"
		}
		if asCPO {
			if err := tx.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_user_id,author_scope,body,created_at) VALUES (?,?,?,'CPO',?,?)`, uuid.New(), id, userID, r.Body, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,actor_user_id,idempotency_key,created_at) VALUES (?,?,'MESSAGE_ADDED','CPO',?,?,?)`, uuid.New(), id, userID, r.IdempotencyKey, now).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_customer_id,author_scope,body,created_at) VALUES (?,?,?,'CUSTOMER',?,?)`, uuid.New(), id, customerID, r.Body, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,actor_customer_id,idempotency_key,created_at) VALUES (?,?,'MESSAGE_ADDED','CUSTOMER',?,?,?)`, uuid.New(), id, customerID, r.IdempotencyKey, now).Error; err != nil {
				return err
			}
		}
		if next != t.Status {
			if err := tx.Exec("UPDATE support_tickets SET status=?,closed_at=NULL,updated_at=? WHERE id=?", next, now, id).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,actor_customer_id,previous_status,next_status,created_at) VALUES (?,?,'STATUS_CHANGED','CUSTOMER',?,?,?,?)`, uuid.New(), id, customerID, t.Status, next, now).Error; err != nil {
				return err
			}
		}
		if next == t.Status {
			if err := tx.Exec("UPDATE support_tickets SET updated_at=? WHERE id=?", now, id).Error; err != nil {
				return err
			}
		}
		if asCPO {
			return s.notifyCustomerCPO(tx, id, "CUSTOMER_CPO_SUPPORT_TICKET_REPLY", false, now)
		}
		return s.notifyCustomerCPO(tx, id, "CUSTOMER_CPO_SUPPORT_TICKET_REPLY", true, now)
	})
	if err != nil {
		return CustomerTicketView{}, err
	}
	return s.customerTicket(ctx, id, &cpoID, map[bool]*uuid.UUID{true: nil, false: &customerID}[asCPO], asCPO, asCPO)
}
func (s *Service) SetCPOCustomerStatus(ctx context.Context, p auth.Principal, id uuid.UUID, r StatusRequest) (CustomerTicketView, error) {
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportManage); err != nil {
		return CustomerTicketView{}, err
	}
	if err := s.requireCPOPermission(ctx, p, cpopermissions.CustomerSupportRead); err != nil {
		return CustomerTicketView{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(r.Status))
	r.Reason = strings.TrimSpace(r.Reason)
	if !isStatus(status) || len(r.Reason) > 500 {
		return CustomerTicketView{}, invalid()
	}
	now := s.now()
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t struct {
			ID     uuid.UUID
			Status string
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("support_tickets").Select("id,status").Where("id=? AND cpo_id=? AND channel=?", id, *p.CPOID, customerCPOChannel).Scan(&t).Error; err != nil {
			return err
		}
		if t.ID == uuid.Nil {
			return notFound()
		}
		if t.Status == status {
			return nil
		}
		if !validTransition(t.Status, status) {
			return invalidTransition()
		}
		var closed any = nil
		if status == "CLOSED" {
			closed = now
		}
		if err := tx.Exec("UPDATE support_tickets SET status=?,closed_at=?,updated_at=? WHERE id=?", status, closed, now, id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,actor_user_id,previous_status,next_status,reason,created_at) VALUES (?,?,'STATUS_CHANGED','CPO',?,?,?,?,?)`, uuid.New(), id, p.UserID, t.Status, status, r.Reason, now).Error; err != nil {
			return err
		}
		return s.notifyCustomerCPO(tx, id, "CUSTOMER_CPO_SUPPORT_TICKET_STATUS_CHANGED", false, now)
	})
	if err != nil {
		return CustomerTicketView{}, err
	}
	return s.GetCPOCustomer(ctx, p, id)
}

// notifyCustomerCPO persists only bounded metadata in the encrypted mail outbox.
// Message bodies stay in the authorized ticket detail, never in mail payloads.
func (s *Service) notifyCustomerCPO(tx *gorm.DB, ticketID uuid.UUID, template string, toCPO bool, occurredAt time.Time) error {
	if s.outbox == nil {
		return nil
	}
	var row struct {
		ID            uuid.UUID
		CPOID         uuid.UUID
		CustomerID    uuid.UUID
		Subject       string
		Status        string
		CPOName       string
		CustomerEmail string
		CustomerName  string
	}
	if err := tx.Table("support_tickets").Select("support_tickets.id,support_tickets.cpo_id,support_tickets.customer_id,support_tickets.subject,support_tickets.status,cpos.business_name AS cpo_name,customers.email AS customer_email,customers.full_name AS customer_name").Joins("JOIN cpos ON cpos.id=support_tickets.cpo_id").Joins("JOIN customers ON customers.id=support_tickets.customer_id AND customers.cpo_id=support_tickets.cpo_id").Where("support_tickets.id=? AND support_tickets.channel=?", ticketID, customerCPOChannel).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == uuid.Nil {
		return notFound()
	}
	if toCPO {
		url, err := config.BuildActionURL(s.frontend.CPOCustomerSupportTicketTemplate, map[string]string{"ticket_id": ticketID.String()}, "ticket_id")
		if err != nil {
			return err
		}
		recipients, err := s.customerSupportRecipients(tx, row.CPOID)
		if err != nil {
			return err
		}
		for _, r := range recipients {
			if err := s.outbox.EnqueueMessageWithContext(tx, r.Email, template, cmsmail.MessagePayload{RecipientName: r.FullName, CPOName: row.CPOName, SupportSubject: row.Subject, SupportStatus: row.Status, OccurredAt: occurredAt, ActionURL: url}, cmsmail.MessageContext{CPOID: &row.CPOID, UserID: &r.UserID}); err != nil {
				return err
			}
		}
		return nil
	}
	url, err := config.BuildActionURL(s.frontend.CustomerSupportTicketTemplate, map[string]string{"ticket_id": ticketID.String()}, "ticket_id")
	if err != nil {
		return err
	}
	return s.outbox.EnqueueMessageWithContext(tx, row.CustomerEmail, template, cmsmail.MessagePayload{RecipientName: row.CustomerName, CPOName: row.CPOName, SupportSubject: row.Subject, SupportStatus: row.Status, OccurredAt: occurredAt, ActionURL: url}, cmsmail.MessageContext{CPOID: &row.CPOID})
}

func (s *Service) customerSupportRecipients(tx *gorm.DB, cpoID uuid.UUID) ([]supportRecipient, error) {
	var members []struct {
		UserID         uuid.UUID
		Email          string
		FullName       string
		Role           constants.CPORole
		IsPrimaryAdmin bool
	}
	if err := tx.Table("cpo_memberships").Select("cpo_memberships.user_id,users.email,users.full_name,cpo_memberships.role,cpo_memberships.is_primary_admin").Joins("JOIN users ON users.id=cpo_memberships.user_id").Where("cpo_memberships.cpo_id=? AND cpo_memberships.status=? AND users.is_active=?", cpoID, constants.MembershipStatusActive, true).Scan(&members).Error; err != nil {
		return nil, err
	}
	result := make([]supportRecipient, 0, len(members))
	for _, m := range members {
		p := auth.Principal{UserID: m.UserID, Scope: constants.AuthScopeCPO, CPOID: &cpoID, Role: &m.Role}
		_, allowed, err := auth.EvaluateCPOPermission(context.Background(), tx, p, cpopermissions.CustomerSupportRead)
		if err != nil {
			return nil, err
		}
		if allowed || m.IsPrimaryAdmin {
			result = append(result, supportRecipient{UserID: m.UserID, Email: m.Email, FullName: m.FullName})
		}
	}
	return deduplicateRecipients(result), nil
}
