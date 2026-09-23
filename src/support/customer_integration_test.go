package support

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpopermissions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestCustomerCPOWorkflowWithPostgreSQL covers database constraints, row locks,
// channel isolation, and outbox idempotency. It only accepts a deliberately
// selected disposable database.
func TestCustomerCPOWorkflowWithPostgreSQL(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations including 74: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	box, err := security.NewSecretBox("customer-support-test", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(gormDB).WithNotificationDelivery(cmsmail.NewOutbox(box), nil, config.FrontendLinks{CPOCustomerSupportTicketTemplate: "https://cms.example.invalid/customer-support/tickets/{ticket_id}", CustomerSupportTicketTemplate: "https://app.example.invalid/support/tickets/{ticket_id}"})
	s.now = func() time.Time { return now }
	cpo, cpoID := supportCPOPrincipal(t, gormDB, now, "customer-channel")
	platformUser := supportUser(t, gormDB, now)
	platform := auth.Principal{UserID: platformUser.ID, Scope: constants.AuthScopePlatform}
	first := supportCustomer(t, gormDB, cpoID, now, "first")
	second := supportCustomer(t, gormDB, cpoID, now, "second")
	otherCPO, otherCPOID := supportCPOPrincipal(t, gormDB, now, "other-channel")
	other := supportCustomer(t, gormDB, otherCPOID, now, "other")

	created, err := s.CreateCustomer(ctx, first, CreateRequest{Subject: "Billing question", Body: "Private customer body"})
	if err != nil || created.CustomerID != first.CustomerID || created.CPOID != cpoID || created.Status != "OPEN" {
		t.Fatalf("create = %#v, %v", created, err)
	}
	if len(created.Messages) != 1 || created.Messages[0].AuthorScope != "CUSTOMER" || created.Messages[0].AuthorUserID != nil || len(created.Events) != 1 || created.Events[0].ActorUserID != nil || created.Events[0].Reason != nil {
		t.Fatalf("customer projection leaked audit identity: %#v", created)
	}
	createdSecond, err := s.CreateCustomer(ctx, second, CreateRequest{Subject: "Other subject", Body: "Second customer body"})
	if err != nil {
		t.Fatalf("create second customer ticket: %v", err)
	}
	if _, err := s.GetCustomer(ctx, second, created.ID); err == nil {
		t.Fatal("same-CPO other customer read succeeded")
	}
	if _, err := s.ReplyCustomer(ctx, second, created.ID, ReplyRequest{Body: "no", IdempotencyKey: "other"}); err == nil {
		t.Fatal("same-CPO other customer reply succeeded")
	}
	if _, err := s.GetCustomer(ctx, other, created.ID); err == nil {
		t.Fatal("cross-CPO customer read succeeded")
	}
	if page, err := s.List(ctx, cpo, ListQuery{Limit: 20}); err != nil || len(page.Tickets) != 0 {
		t.Fatalf("legacy CPO queue leaked customer ticket: %#v, %v", page, err)
	}
	if _, err := s.Get(ctx, cpo, created.ID); err == nil {
		t.Fatal("legacy CPO detail leaked customer ticket")
	}
	if _, err := s.Reply(ctx, cpo, created.ID, ReplyRequest{Body: "no", IdempotencyKey: "legacy-cpo"}); err == nil {
		t.Fatal("legacy CPO reply reached customer ticket")
	}
	if page, err := s.List(ctx, platform, ListQuery{Limit: 20, CPOID: &cpoID}); err != nil || len(page.Tickets) != 0 {
		t.Fatalf("platform queue leaked customer ticket: %#v, %v", page, err)
	}
	if _, err := s.Get(ctx, platform, created.ID); err == nil {
		t.Fatal("platform detail leaked customer ticket")
	}
	if _, err := s.Reply(ctx, platform, created.ID, ReplyRequest{Body: "no", IdempotencyKey: "legacy-platform"}); err == nil {
		t.Fatal("platform reply reached customer ticket")
	}
	if _, err := s.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "CLOSED"}); err == nil {
		t.Fatal("platform status reached customer ticket")
	}
	page, err := s.ListCustomer(ctx, first, ListQuery{Limit: 20})
	if err != nil || len(page.Tickets) != 1 || page.Tickets[0].ID != created.ID || page.Tickets[0].Customer != nil {
		t.Fatalf("customer page=%#v err=%v", page, err)
	}
	if page, err := s.ListCPOCustomer(ctx, cpo, ListQuery{Limit: 20, CustomerID: &first.CustomerID, Search: "first@example.invalid"}); err != nil || len(page.Tickets) != 1 || page.Tickets[0].Customer == nil || page.Tickets[0].Customer.Email == "" {
		t.Fatalf("CPO filtered queue=%#v err=%v", page, err)
	}
	for search, expectedID := range map[string]uuid.UUID{
		created.ID.String():      created.ID,
		"Billing question":       created.ID,
		"first customer":         created.ID,
		"first@example.invalid":  created.ID,
		"second@example.invalid": createdSecond.ID,
	} {
		page, err := s.ListCPOCustomer(ctx, cpo, ListQuery{Limit: 1, Search: search})
		if err != nil || len(page.Tickets) != 1 || page.Tickets[0].ID != expectedID {
			t.Fatalf("CPO q=%q page=%#v err=%v", search, page, err)
		}
	}
	firstPage, err := s.ListCPOCustomer(ctx, cpo, ListQuery{Limit: 1})
	if err != nil || !firstPage.HasMore || firstPage.NextBefore == nil || firstPage.NextBeforeID == nil {
		t.Fatalf("CPO first keyset page=%#v err=%v", firstPage, err)
	}
	secondPage, err := s.ListCPOCustomer(ctx, cpo, ListQuery{Limit: 1, Before: firstPage.NextBefore, BeforeID: firstPage.NextBeforeID})
	if err != nil || len(secondPage.Tickets) != 1 || secondPage.Tickets[0].ID == firstPage.Tickets[0].ID {
		t.Fatalf("CPO keyset continuation=%#v err=%v", secondPage, err)
	}

	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "RESOLVED"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if page, err := s.ListCPOCustomer(ctx, cpo, ListQuery{Limit: 20, Status: "RESOLVED"}); err != nil || len(page.Tickets) != 1 || page.Tickets[0].ID != created.ID {
		t.Fatalf("CPO status filter=%#v err=%v", page, err)
	}
	reopened, err := s.ReplyCustomer(ctx, first, created.ID, ReplyRequest{Body: "Still unresolved", IdempotencyKey: "reopen"})
	if err != nil || reopened.Status != "OPEN" || reopened.ClosedAt != nil {
		t.Fatalf("resolved reopen=%#v %v", reopened, err)
	}
	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "CLOSED"}); err != nil {
		t.Fatalf("close: %v", err)
	}
	closed, err := s.ReplyCustomer(ctx, first, created.ID, ReplyRequest{Body: "Reopen closed", IdempotencyKey: "reopen-closed"})
	if err != nil || closed.Status != "OPEN" || closed.ClosedAt != nil {
		t.Fatalf("closed reopen=%#v %v", closed, err)
	}
	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "IN_PROGRESS"}); err != nil {
		t.Fatalf("mark in progress: %v", err)
	}
	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "OPEN"}); err == nil {
		t.Fatal("invalid IN_PROGRESS -> OPEN accepted")
	}
	beforeReply, err := s.GetCPOCustomer(ctx, cpo, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterReply, err := s.ReplyCPOCustomer(ctx, cpo, created.ID, ReplyRequest{Body: "CPO answer", IdempotencyKey: "cpo-answer"})
	if err != nil || afterReply.Status != "IN_PROGRESS" || len(afterReply.Messages) != len(beforeReply.Messages)+1 {
		t.Fatalf("CPO reply changed lifecycle=%#v err=%v", afterReply, err)
	}
	if last := afterReply.Messages[len(afterReply.Messages)-1]; last.AuthorUserID == nil || *last.AuthorUserID != cpo.UserID {
		t.Fatalf("CPO message is not backed by the administrative user: %#v", last)
	}
	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "IN_PROGRESS"}); err != nil {
		t.Fatalf("same status must be side-effect free: %v", err)
	}
	var mailsBeforeConcurrent int64
	if err := gormDB.Model(&models.MailOutbox{}).Where("template=?", "CUSTOMER_CPO_SUPPORT_TICKET_REPLY").Count(&mailsBeforeConcurrent).Error; err != nil {
		t.Fatalf("count mail intents before concurrent reply: %v", err)
	}

	key := "concurrent-" + uuid.NewString()
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.ReplyCustomer(ctx, first, created.ID, ReplyRequest{Body: "one durable reply", IdempotencyKey: key})
			errors <- e
		}()
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatalf("concurrent reply: %v", e)
		}
	}
	var messages, events, mails int64
	gormDB.Table("support_ticket_messages").Where("ticket_id=? AND body=?", created.ID, "one durable reply").Count(&messages)
	gormDB.Table("support_ticket_events").Where("ticket_id=? AND idempotency_key=?", created.ID, key).Count(&events)
	gormDB.Model(&models.MailOutbox{}).Where("template=?", "CUSTOMER_CPO_SUPPORT_TICKET_REPLY").Count(&mails)
	if messages != 1 || events != 1 || mails != mailsBeforeConcurrent+1 {
		t.Fatalf("idempotency messages=%d events=%d mails=%d before=%d", messages, events, mails, mailsBeforeConcurrent)
	}

	if _, err := s.ListCPOCustomer(ctx, otherCPO, ListQuery{Limit: 20}); err != nil {
		t.Fatalf("other tenant queue: %v", err)
	}
	if _, err := s.GetCPOCustomer(ctx, otherCPO, created.ID); err == nil {
		t.Fatal("cross-CPO staff detail succeeded")
	}
	if _, err := s.ReplyCPOCustomer(ctx, otherCPO, created.ID, ReplyRequest{Body: "no", IdempotencyKey: "cross"}); err == nil {
		t.Fatal("cross-CPO staff reply succeeded")
	}
	if _, err := s.SetCPOCustomerStatus(ctx, otherCPO, created.ID, StatusRequest{Status: "CLOSED"}); err == nil {
		t.Fatal("cross-CPO staff status succeeded")
	}

	membership := membershipID(t, gormDB, cpoID, cpo.UserID)
	for _, permission := range []string{cpopermissions.CustomerSupportReply, cpopermissions.CustomerSupportManage} {
		if err := gormDB.Create(&models.CPOMembershipPermissionOverride{ID: uuid.New(), MembershipID: membership, Permission: permission, Effect: "DENY", CreatedBy: cpo.UserID, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			t.Fatalf("add %s DENY: %v", permission, err)
		}
	}
	if _, err := s.ReplyCPOCustomer(ctx, cpo, created.ID, ReplyRequest{Body: "denied", IdempotencyKey: "denied-reply"}); err == nil {
		t.Fatal("customer_support.reply DENY did not block reply")
	}
	if _, err := s.SetCPOCustomerStatus(ctx, cpo, created.ID, StatusRequest{Status: "CLOSED"}); err == nil {
		t.Fatal("customer_support.manage DENY did not block status mutation")
	}
	if err := gormDB.Create(&models.CPOMembershipPermissionOverride{ID: uuid.New(), MembershipID: membership, Permission: cpopermissions.CustomerSupportRead, Effect: "DENY", CreatedBy: cpo.UserID, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("add read DENY: %v", err)
	}
	if _, err := s.ListCPOCustomer(ctx, cpo, ListQuery{}); err == nil {
		t.Fatal("explicit DENY did not win")
	}

	if err := gormDB.Exec(`INSERT INTO support_tickets (id,cpo_id,channel,customer_id,subject,status,created_at,updated_at) VALUES (?,?, 'CUSTOMER_CPO', ?,?,'OPEN',?,?)`, uuid.New(), otherCPOID, first.CustomerID, "bad tenant", now, now).Error; err == nil {
		t.Fatal("cross-CPO ticket customer FK accepted")
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_scope,body,created_at) VALUES (?,?, 'CUSTOMER',?,?)`, uuid.New(), created.ID, "missing actor", now).Error; err == nil {
		t.Fatal("invalid CUSTOMER actor accepted")
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_user_id,author_scope,body,created_at) VALUES (?,?,?,'CUSTOMER',?,?)`, uuid.New(), created.ID, cpo.UserID, "masquerading admin", now).Error; err == nil {
		t.Fatal("CUSTOMER actor backed by users row accepted")
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_scope,created_at) VALUES (?,?,'MESSAGE_ADDED','CUSTOMER',?)`, uuid.New(), created.ID, now).Error; err == nil {
		t.Fatal("CUSTOMER event without customer actor accepted")
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_user_id,actor_scope,created_at) VALUES (?,?, 'MESSAGE_ADDED',?,'CUSTOMER',?)`, uuid.New(), created.ID, cpo.UserID, now).Error; err == nil {
		t.Fatal("CUSTOMER event backed by users row accepted")
	}
	for _, wrongCustomer := range []uuid.UUID{second.CustomerID, other.CustomerID, uuid.New()} {
		if err := gormDB.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_customer_id,author_scope,body,created_at) VALUES (?,?,?,'CUSTOMER',?,?)`, uuid.New(), created.ID, wrongCustomer, "wrong customer", now).Error; err == nil {
			t.Fatalf("wrong CUSTOMER message actor %s accepted", wrongCustomer)
		}
		if err := gormDB.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_customer_id,actor_scope,created_at) VALUES (?,?,'MESSAGE_ADDED',?,'CUSTOMER',?)`, uuid.New(), created.ID, wrongCustomer, now).Error; err == nil {
			t.Fatalf("wrong CUSTOMER event actor %s accepted", wrongCustomer)
		}
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_user_id,author_customer_id,author_scope,body,created_at) VALUES (?,?,?,?,'CUSTOMER',?,?)`, uuid.New(), created.ID, cpo.UserID, first.CustomerID, "mixed actor", now).Error; err == nil {
		t.Fatal("mixed CUSTOMER message actor accepted")
	}
	if err := gormDB.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_user_id,actor_customer_id,actor_scope,created_at) VALUES (?,?, 'MESSAGE_ADDED',?,?,'CUSTOMER',?)`, uuid.New(), created.ID, cpo.UserID, first.CustomerID, now).Error; err == nil {
		t.Fatal("mixed CUSTOMER event actor accepted")
	}
	validMessageID := uuid.New()
	if err := gormDB.Exec(`INSERT INTO support_ticket_messages (id,ticket_id,author_customer_id,author_scope,body,created_at) VALUES (?,?,?,'CUSTOMER',?,?)`, validMessageID, created.ID, first.CustomerID, "valid direct customer actor", now).Error; err != nil {
		t.Fatalf("valid CUSTOMER message actor rejected: %v", err)
	}
	assertCustomerSupportTicketActorForeignKeyRules(t, gormDB, "RESTRICT", "CASCADE")
	if err := gormDB.Exec(`UPDATE support_tickets SET customer_id=? WHERE id=?`, second.CustomerID, created.ID).Error; err == nil {
		t.Fatal("ticket-owner change after CUSTOMER message cascaded into actor history")
	}
	var ticketAfterMessage struct {
		CustomerID uuid.UUID `gorm:"column:customer_id"`
	}
	if err := gormDB.Table("support_tickets").Select("customer_id").Where("id=?", created.ID).Take(&ticketAfterMessage).Error; err != nil || ticketAfterMessage.CustomerID != first.CustomerID {
		t.Fatalf("failed ticket-owner change after message mutated ticket owner=%s err=%v", ticketAfterMessage.CustomerID, err)
	}
	var messageAfterFailedOwnerChange struct {
		AuthorCustomerID uuid.UUID `gorm:"column:author_customer_id"`
	}
	if err := gormDB.Table("support_ticket_messages").Select("author_customer_id").Where("id=?", validMessageID).Take(&messageAfterFailedOwnerChange).Error; err != nil || messageAfterFailedOwnerChange.AuthorCustomerID != first.CustomerID {
		t.Fatalf("failed ticket-owner change mutated message actor=%s err=%v", messageAfterFailedOwnerChange.AuthorCustomerID, err)
	}
	validEventID := uuid.New()
	if err := gormDB.Exec(`INSERT INTO support_ticket_events (id,ticket_id,event_type,actor_customer_id,actor_scope,created_at) VALUES (?,?,'MESSAGE_ADDED',?,'CUSTOMER',?)`, validEventID, created.ID, first.CustomerID, now).Error; err != nil {
		t.Fatalf("valid CUSTOMER event actor rejected: %v", err)
	}
	if err := gormDB.Exec(`UPDATE support_tickets SET customer_id=? WHERE id=?`, second.CustomerID, created.ID).Error; err == nil {
		t.Fatal("ticket-owner change after CUSTOMER event cascaded into actor history")
	}
	var persistedTicket struct {
		CustomerID uuid.UUID `gorm:"column:customer_id"`
	}
	if err := gormDB.Table("support_tickets").Select("customer_id").Where("id=?", created.ID).Take(&persistedTicket).Error; err != nil || persistedTicket.CustomerID != first.CustomerID {
		t.Fatalf("failed ticket-owner change mutated ticket owner=%s err=%v", persistedTicket.CustomerID, err)
	}
	var persistedMessage struct {
		AuthorCustomerID uuid.UUID `gorm:"column:author_customer_id"`
	}
	if err := gormDB.Table("support_ticket_messages").Select("author_customer_id").Where("id=?", validMessageID).Take(&persistedMessage).Error; err != nil || persistedMessage.AuthorCustomerID != first.CustomerID {
		t.Fatalf("failed ticket-owner change mutated message actor=%s err=%v", persistedMessage.AuthorCustomerID, err)
	}
	var persistedEvent struct {
		ActorCustomerID uuid.UUID `gorm:"column:actor_customer_id"`
	}
	if err := gormDB.Table("support_ticket_events").Select("actor_customer_id").Where("id=?", validEventID).Take(&persistedEvent).Error; err != nil || persistedEvent.ActorCustomerID != first.CustomerID {
		t.Fatalf("failed ticket-owner change mutated event actor=%s err=%v", persistedEvent.ActorCustomerID, err)
	}
	if _, err := s.ReplyCustomer(ctx, first, created.ID, ReplyRequest{Body: "Customer reply after rejected owner change", IdempotencyKey: "owner-unchanged"}); err != nil {
		t.Fatalf("valid customer reply after rejected owner change: %v", err)
	}
	if err := db.RollbackLastMigration(ctx, sqlDB); err != nil {
		t.Fatalf("rollback migration 74: %v", err)
	}
	assertCustomerSupportTicketActorForeignKeyRules(t, gormDB, "CASCADE", "CASCADE")
	var preserved int64
	if err := gormDB.Table("support_tickets").Where("id=? AND channel='CUSTOMER_CPO'", created.ID).Count(&preserved).Error; err != nil || preserved != 1 {
		t.Fatalf("migration 74 rollback did not preserve customer support history count=%d err=%v", preserved, err)
	}
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("reapply migration 74 after rollback: %v", err)
	}
	assertCustomerSupportTicketActorForeignKeyRules(t, gormDB, "RESTRICT", "CASCADE")
}

func assertCustomerSupportTicketActorForeignKeyRules(t *testing.T, database *gorm.DB, wantUpdate, wantDelete string) {
	t.Helper()
	for _, name := range []string{"fk_support_ticket_messages_ticket_customer", "fk_support_ticket_events_ticket_customer"} {
		var rule struct {
			UpdateRule string `gorm:"column:update_rule"`
			DeleteRule string `gorm:"column:delete_rule"`
		}
		if err := database.Raw(`SELECT update_rule, delete_rule FROM information_schema.referential_constraints WHERE constraint_schema = current_schema() AND constraint_name = ?`, name).Scan(&rule).Error; err != nil {
			t.Fatalf("load %s referential rule: %v", name, err)
		}
		if rule.UpdateRule != wantUpdate || rule.DeleteRule != wantDelete {
			t.Fatalf("%s rules update=%q delete=%q, want update=%q delete=%q", name, rule.UpdateRule, rule.DeleteRule, wantUpdate, wantDelete)
		}
	}
}

func ptrRole(v constants.CPORole) *constants.CPORole { return &v }
func supportCustomer(t *testing.T, database *gorm.DB, cpoID uuid.UUID, now time.Time, label string) customerauth.Principal {
	t.Helper()
	customer := models.Customer{ID: uuid.New(), CPOID: cpoID, Email: label + "@example.invalid", PasswordHash: "test-only", FullName: label + " customer", IsVerified: true, PasswordChangedAt: now, Status: constants.CustomerStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	return customerauth.Principal{UserID: customer.ID, CustomerID: customer.ID, CPOID: cpoID, SessionID: uuid.New()}
}
func membershipID(t *testing.T, database *gorm.DB, cpoID, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var m models.CPOMembership
	if err := database.Where("cpo_id=? AND user_id=?", cpoID, userID).First(&m).Error; err != nil {
		t.Fatalf("load membership: %v", err)
	}
	return m.ID
}
