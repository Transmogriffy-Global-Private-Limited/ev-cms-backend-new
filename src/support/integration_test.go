package support

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/testsupport"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// This test only runs against an explicitly selected disposable PostgreSQL
// database. It verifies the database constraints and row locks that unit tests
// cannot model.
func TestSupportWorkflowWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	gormDB, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	mailBox, err := security.NewSecretBox("support-test", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create mail encryption box: %v", err)
	}
	service := NewService(gormDB).WithNotificationDelivery(
		cmsmail.NewOutbox(mailBox),
		platformops.NewService(gormDB, config.Platform{EventRetention: time.Hour}),
		config.FrontendLinks{CPOSupportTicketTemplate: "https://cms.example.invalid/support/tickets/{ticket_id}"},
	)
	service.now = func() time.Time { return now }
	platformUser := supportUser(t, gormDB, now)
	platform := auth.Principal{UserID: platformUser.ID, Scope: constants.AuthScopePlatform}
	first, firstCPOID := supportCPOPrincipal(t, gormDB, now, "first")
	second, _ := supportCPOPrincipal(t, gormDB, now, "second")
	eligibleAdmin := supportUser(t, gormDB, now)
	supportMembership(t, gormDB, firstCPOID, eligibleAdmin.ID, constants.MembershipStatusActive, now)
	suspendedAdmin := supportUser(t, gormDB, now)
	supportMembership(t, gormDB, firstCPOID, suspendedAdmin.ID, constants.MembershipStatusSuspended, now)
	inactiveAdmin := supportUser(t, gormDB, now)
	if err := gormDB.Model(&models.User{}).Where("id = ?", inactiveAdmin.ID).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate support recipient: %v", err)
	}
	supportMembership(t, gormDB, firstCPOID, inactiveAdmin.ID, constants.MembershipStatusActive, now)

	created, err := service.Create(ctx, first, CreateRequest{Subject: "First connector", Body: "The first connector needs review."})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if created.Status != "OPEN" || len(created.Messages) != 1 || len(created.Events) != 1 || created.Events[0].EventType != "CREATED" {
		t.Fatalf("created detail = %#v", created)
	}
	if countSupportMailJobs(t, gormDB, "CPO_SUPPORT_TICKET_CREATED") != 2 {
		t.Fatal("ticket creation did not enqueue exactly the eligible CPO confirmations")
	}
	createdRecipients := supportMailRecipients(t, gormDB, "CPO_SUPPORT_TICKET_CREATED")
	if !createdRecipients[eligibleAdmin.Email] || createdRecipients[suspendedAdmin.Email] || createdRecipients[inactiveAdmin.Email] {
		t.Fatalf("support confirmation recipients were not safely filtered: %#v", createdRecipients)
	}
	if countSupportPlatformEvents(t, gormDB, "support.ticket.created") != 1 {
		t.Fatal("ticket creation did not publish the durable platform support event")
	}

	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "IN_PROGRESS", Reason: "Assigned for review"}); err != nil {
		t.Fatalf("OPEN -> IN_PROGRESS: %v", err)
	}
	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "OPEN"}); err == nil {
		t.Fatal("IN_PROGRESS -> OPEN unexpectedly succeeded")
	} else if apiError, ok := err.(*auth.APIError); !ok || apiError.Status != 409 || apiError.Code != "invalid_support_transition" {
		t.Fatalf("invalid transition = %#v", err)
	}
	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "RESOLVED", Reason: "Configuration verified"}); err != nil {
		t.Fatalf("IN_PROGRESS -> RESOLVED: %v", err)
	}
	reopened, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "It recurred after the test.", IdempotencyKey: "reopen-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CPO reopen reply: %v", err)
	}
	if countSupportPlatformEvents(t, gormDB, "support.ticket.cpo_replied") != 1 || countSupportPlatformEvents(t, gormDB, "support.ticket.reopened") != 1 {
		t.Fatal("CPO reply/reopen did not publish durable platform activity")
	}
	if reopened.Status != "OPEN" || reopened.ClosedAt != nil || !hasSupportEvent(reopened.Events, "MESSAGE_ADDED", nil, nil) || !hasSupportEvent(reopened.Events, "STATUS_CHANGED", stringPointer("RESOLVED"), stringPointer("OPEN")) {
		t.Fatalf("reopened detail did not contain message and truthful status history: %#v", reopened)
	}
	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "CLOSED", Reason: "Closed for regression test"}); err != nil {
		t.Fatalf("OPEN -> CLOSED: %v", err)
	}
	closedReopened, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Reopen after closure.", IdempotencyKey: "closed-reopen-" + uuid.NewString()})
	if err != nil || closedReopened.Status != "OPEN" || closedReopened.ClosedAt != nil {
		t.Fatalf("closed ticket reopen = %#v, %v", closedReopened, err)
	}

	if _, err := service.Reply(ctx, platform, created.ID, ReplyRequest{Body: "A private platform response that must not leak by email.", IdempotencyKey: "platform-" + uuid.NewString()}); err != nil {
		t.Fatalf("platform reply: %v", err)
	}
	if countSupportMailJobs(t, gormDB, "CPO_SUPPORT_TICKET_PLATFORM_REPLY") != 2 {
		t.Fatal("platform reply did not enqueue exactly the eligible CPO notifications")
	}
	if payload := latestSupportMailPayload(t, gormDB, mailBox, "CPO_SUPPORT_TICKET_PLATFORM_REPLY"); strings.Contains(string(payload), "A private platform response") {
		t.Fatalf("platform reply body leaked into mail payload: %s", payload)
	}
	if payload := latestSupportMailPayload(t, gormDB, mailBox, "CPO_SUPPORT_TICKET_CREATED"); !strings.Contains(string(payload), "https://cms.example.invalid/support/tickets/"+created.ID.String()) {
		t.Fatalf("ticket mail action URL did not carry ticket context: %s", payload)
	}

	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "RESOLVED", Reason: "Resolved notification coverage"}); err != nil {
		t.Fatalf("OPEN -> RESOLVED: %v", err)
	}
	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "OPEN", Reason: "Reopened notification coverage"}); err != nil {
		t.Fatalf("RESOLVED -> OPEN: %v", err)
	}
	if _, err := service.SetStatus(ctx, platform, created.ID, StatusRequest{Status: "CLOSED", Reason: "Closed notification coverage"}); err != nil {
		t.Fatalf("OPEN -> CLOSED notification: %v", err)
	}
	for _, template := range []string{"CPO_SUPPORT_TICKET_RESOLVED", "CPO_SUPPORT_TICKET_REOPENED", "CPO_SUPPORT_TICKET_CLOSED"} {
		if countSupportMailJobs(t, gormDB, template) == 0 {
			t.Fatalf("%s notification was not queued", template)
		}
	}

	key := "reply-" + uuid.NewString()
	if _, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Retry-safe message.", IdempotencyKey: key}); err != nil {
		t.Fatalf("first idempotent reply: %v", err)
	}
	replayed, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Retry-safe message.", IdempotencyKey: key})
	if err != nil || countSupportMessages(replayed.Messages, "Retry-safe message.") != 1 {
		t.Fatalf("replayed reply duplicated message: %#v, %v", replayed, err)
	}
	if countSupportPlatformEvents(t, gormDB, "support.ticket.cpo_replied") != 3 {
		t.Fatal("idempotent retry duplicated the CPO platform notification intent")
	}

	concurrentKey := "concurrent-" + uuid.NewString()
	var replies sync.WaitGroup
	errorsByReply := make(chan error, 2)
	for range 2 {
		replies.Add(1)
		go func() {
			defer replies.Done()
			_, replyErr := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Concurrent retry.", IdempotencyKey: concurrentKey})
			errorsByReply <- replyErr
		}()
	}
	replies.Wait()
	close(errorsByReply)
	for replyErr := range errorsByReply {
		if replyErr != nil {
			t.Fatalf("concurrent reply: %v", replyErr)
		}
	}
	concurrentDetail, err := service.Get(ctx, first, created.ID)
	if err != nil || countSupportMessages(concurrentDetail.Messages, "Concurrent retry.") != 1 {
		t.Fatalf("concurrent idempotency = %#v, %v", concurrentDetail, err)
	}

	secondTicket, err := service.Create(ctx, second, CreateRequest{Subject: "Second connector", Body: "Another CPO ticket."})
	if err != nil {
		t.Fatalf("create second CPO ticket: %v", err)
	}
	if _, err := service.Get(ctx, first, secondTicket.ID); err == nil {
		t.Fatal("cross-CPO detail unexpectedly succeeded")
	} else if apiError, ok := err.(*auth.APIError); !ok || apiError.Code != "support_ticket_not_found" {
		t.Fatalf("cross-CPO detail = %#v", err)
	}

	page, err := service.List(ctx, platform, ListQuery{Limit: 1})
	if err != nil || len(page.Tickets) != 1 || !page.HasMore || page.NextBefore == nil || page.NextBeforeID == nil {
		t.Fatalf("first support page = %#v, %v", page, err)
	}
	if len(page.Tickets[0].Subject) == 0 || page.Tickets[0].MessageCount == 0 || page.Tickets[0].LastMessageAt == nil || page.Tickets[0].LastMessageScope == nil {
		t.Fatalf("bounded summary missing queue fields: %#v", page.Tickets[0])
	}
	secondPage, err := service.List(ctx, platform, ListQuery{Limit: 1, Before: page.NextBefore, BeforeID: page.NextBeforeID})
	if err != nil || len(secondPage.Tickets) != 1 || secondPage.Tickets[0].ID == page.Tickets[0].ID {
		t.Fatalf("cursor page = %#v, %v", secondPage, err)
	}
	filtered, err := service.List(ctx, platform, ListQuery{Limit: 20, CPOID: &firstCPOID, Status: "OPEN", Search: "first"})
	if err != nil || len(filtered.Tickets) != 1 || filtered.Tickets[0].ID != created.ID {
		t.Fatalf("filtered support list = %#v, %v", filtered, err)
	}

	var beforeFailedIntent int64
	if err := gormDB.Model(&models.SupportTicket{}).Where("cpo_id = ?", firstCPOID).Count(&beforeFailedIntent).Error; err != nil {
		t.Fatalf("count tickets before failed mail intent: %v", err)
	}
	brokenDelivery := NewService(gormDB).WithNotificationDelivery(
		cmsmail.NewOutbox(mailBox),
		platformops.NewService(gormDB, config.Platform{EventRetention: time.Hour}),
		config.FrontendLinks{CPOSupportTicketTemplate: "https://cms.example.invalid/support/tickets"},
	)
	brokenDelivery.now = func() time.Time { return now }
	if _, err := brokenDelivery.Create(ctx, first, CreateRequest{Subject: "Must roll back", Body: "The outbox action-link validation fails."}); err == nil {
		t.Fatal("ticket creation unexpectedly committed without a valid mail action URL")
	}
	var afterFailedIntent int64
	if err := gormDB.Model(&models.SupportTicket{}).Where("cpo_id = ?", firstCPOID).Count(&afterFailedIntent).Error; err != nil {
		t.Fatalf("count tickets after failed mail intent: %v", err)
	}
	if afterFailedIntent != beforeFailedIntent {
		t.Fatalf("ticket mutation committed without its required mail intent: before=%d after=%d", beforeFailedIntent, afterFailedIntent)
	}
}

func TestReplyRequiresIdempotencyKey(t *testing.T) {
	t.Parallel()
	service := NewService(nil)
	principal := auth.Principal{Scope: constants.AuthScopePlatform, UserID: uuid.New()}
	_, err := service.Reply(context.Background(), principal, uuid.New(), ReplyRequest{Body: "Retry-safe reply"})
	if err == nil || !strings.Contains(err.Error(), "invalid_request") {
		t.Fatalf("reply without idempotency key error = %v, want invalid request", err)
	}
}

func TestDeduplicateSupportRecipients(t *testing.T) {
	t.Parallel()
	recipients := deduplicateRecipients([]supportRecipient{
		{Email: "Admin@example.test", FullName: "First"},
		{Email: " admin@example.test ", FullName: "Duplicate"},
		{Email: "", FullName: "Blank"},
		{Email: "owner@example.test", FullName: "Owner"},
	})
	if len(recipients) != 2 || recipients[0].Email != "admin@example.test" || recipients[1].Email != "owner@example.test" {
		t.Fatalf("deduplicated recipients = %#v", recipients)
	}
}

func supportUser(t *testing.T, database *gorm.DB, now time.Time) models.User {
	t.Helper()
	user := models.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", PasswordHash: "integration-test-only", FullName: "Support Tester", IsActive: true, IsVerified: true, PasswordChangedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("create support user: %v", err)
	}
	return user
}

func supportCPOPrincipal(t *testing.T, database *gorm.DB, now time.Time, label string) (auth.Principal, uuid.UUID) {
	t.Helper()
	user := supportUser(t, database, now)
	cpo := models.CPO{ID: uuid.New(), Slug: label + "-support-" + uuid.NewString()[:8], BusinessName: label + " Support CPO", CompanyType: constants.CPOCompanyTypeCompany, GSTIN: testsupport.ValidGSTIN("19"), Address: "1 Test Road", City: "Kolkata", State: "West Bengal", Pincode: "700001", Status: constants.CPOStatusActive, StatusReason: "Integration test", StatusChangedAt: now, AppID: "cpo_dummy_" + uuid.NewString(), AppIDMode: constants.CPOAppIDModeDummy, AppIDUpdatedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&cpo).Error; err != nil {
		t.Fatalf("create support CPO: %v", err)
	}
	role := constants.CPORoleAdmin
	membership := models.CPOMembership{ID: uuid.New(), CPOID: cpo.ID, UserID: user.ID, Role: role, Status: constants.MembershipStatusActive, IsPrimaryAdmin: true, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&membership).Error; err != nil {
		t.Fatalf("create support membership: %v", err)
	}
	return auth.Principal{UserID: user.ID, Scope: constants.AuthScopeCPO, CPOID: &cpo.ID, Role: &role}, cpo.ID
}

func supportMembership(t *testing.T, database *gorm.DB, cpoID, userID uuid.UUID, status constants.MembershipStatus, now time.Time) {
	t.Helper()
	role := constants.CPORoleAdmin
	membership := models.CPOMembership{ID: uuid.New(), CPOID: cpoID, UserID: userID, Role: role, Status: status, CreatedAt: now, UpdatedAt: now}
	if err := database.Create(&membership).Error; err != nil {
		t.Fatalf("create support recipient membership: %v", err)
	}
}

func hasSupportEvent(events []models.SupportTicketEvent, typ string, previous, next *string) bool {
	for _, event := range events {
		if event.EventType == typ && equalSupportStatus(event.PreviousStatus, previous) && equalSupportStatus(event.NextStatus, next) {
			return true
		}
	}
	return false
}

func equalSupportStatus(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func stringPointer(value string) *string { return &value }

func countSupportMessages(messages []models.SupportTicketMessage, body string) int {
	count := 0
	for _, message := range messages {
		if strings.TrimSpace(message.Body) == body {
			count++
		}
	}
	return count
}

func countSupportMailJobs(t *testing.T, database *gorm.DB, template string) int64 {
	t.Helper()
	var count int64
	if err := database.Model(&models.MailOutbox{}).Where("template = ?", template).Count(&count).Error; err != nil {
		t.Fatalf("count %s mail jobs: %v", template, err)
	}
	return count
}

func countSupportPlatformEvents(t *testing.T, database *gorm.DB, eventType string) int64 {
	t.Helper()
	var count int64
	if err := database.Model(&models.PlatformEvent{}).Where("event_type = ?", eventType).Count(&count).Error; err != nil {
		t.Fatalf("count %s platform events: %v", eventType, err)
	}
	return count
}

func latestSupportMailPayload(t *testing.T, database *gorm.DB, box *security.SecretBox, template string) []byte {
	t.Helper()
	var job models.MailOutbox
	if err := database.Where("template = ?", template).Order("created_at DESC, id DESC").First(&job).Error; err != nil {
		t.Fatalf("load %s mail job: %v", template, err)
	}
	payload, err := box.Open(job.PayloadCiphertext, []byte("ev-cms-mail:"+job.Template+":"+strings.ToLower(strings.TrimSpace(job.ToEmail))))
	if err != nil {
		t.Fatalf("decrypt %s mail job: %v", template, err)
	}
	var decoded cmsmail.MessagePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode %s mail payload: %v", template, err)
	}
	if decoded.SupportSubject == "" || decoded.ActionURL == "" {
		t.Fatalf("incomplete %s mail payload: %#v", template, decoded)
	}
	return payload
}

func supportMailRecipients(t *testing.T, database *gorm.DB, template string) map[string]bool {
	t.Helper()
	var jobs []models.MailOutbox
	if err := database.Where("template = ?", template).Find(&jobs).Error; err != nil {
		t.Fatalf("load %s mail recipients: %v", template, err)
	}
	recipients := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		recipients[job.ToEmail] = true
	}
	return recipients
}
