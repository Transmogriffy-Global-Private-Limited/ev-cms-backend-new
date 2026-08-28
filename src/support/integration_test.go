package support

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
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
	service := NewService(gormDB)
	service.now = func() time.Time { return now }
	platformUser := supportUser(t, gormDB, now)
	platform := auth.Principal{UserID: platformUser.ID, Scope: constants.AuthScopePlatform}
	first, firstCPOID := supportCPOPrincipal(t, gormDB, now, "first")
	second, _ := supportCPOPrincipal(t, gormDB, now, "second")

	created, err := service.Create(ctx, first, CreateRequest{Subject: "First connector", Body: "The first connector needs review."})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if created.Status != "OPEN" || len(created.Messages) != 1 || len(created.Events) != 1 || created.Events[0].EventType != "CREATED" {
		t.Fatalf("created detail = %#v", created)
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

	key := "reply-" + uuid.NewString()
	if _, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Retry-safe message.", IdempotencyKey: key}); err != nil {
		t.Fatalf("first idempotent reply: %v", err)
	}
	replayed, err := service.Reply(ctx, first, created.ID, ReplyRequest{Body: "Retry-safe message.", IdempotencyKey: key})
	if err != nil || countSupportMessages(replayed.Messages, "Retry-safe message.") != 1 {
		t.Fatalf("replayed reply duplicated message: %#v, %v", replayed, err)
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
