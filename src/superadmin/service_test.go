package superadmin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sqlRecorder struct {
	logger.Interface
	queries []string
}

func (recorder *sqlRecorder) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	recorder.queries = append(recorder.queries, sql)
}

func TestSuperadminInputValidation(t *testing.T) {
	t.Parallel()

	if got, err := normalizeEmail("  ADMIN@Example.COM "); err != nil || got != "admin@example.com" {
		t.Fatalf("normalizeEmail() = %q, %v; want normalized address", got, err)
	}
	if _, err := normalizeEmail("Admin <admin@example.com>"); err == nil {
		t.Fatal("normalizeEmail accepted a display-name address")
	}
	if _, err := validateReason("no"); err == nil {
		t.Fatal("validateReason accepted a reason shorter than three characters")
	}
	if got, err := validateReason("  reviewed by security  "); err != nil || got != "reviewed by security" {
		t.Fatalf("validateReason() = %q, %v; want trimmed reason", got, err)
	}

	beforeID := uuid.New()
	if _, err := pageQuery(PageQuery{Limit: 20, BeforeID: &beforeID}); err == nil {
		t.Fatal("pageQuery accepted before_id without before")
	}
	if got, err := pageQuery(PageQuery{}); err != nil || got.Limit != defaultPageSize {
		t.Fatalf("pageQuery() = %+v, %v; want default page size", got, err)
	}
	if got, err := pageQuery(PageQuery{Limit: maxPageSize}); err != nil || got.Limit != maxPageSize {
		t.Fatalf("pageQuery(max) = %+v, %v; want max page size", got, err)
	}
}

func TestMailJobViewDoesNotExposeMailError(t *testing.T) {
	t.Parallel()

	lastError := "SMTP response contained sensitive details"
	job := models.MailOutbox{
		ID: uuid.New(), ToEmail: "admin@example.com", Template: "PLATFORM_ADMIN_INVITE",
		Status: constants.MailOutboxFailed, Attempts: 2, MaxAttempts: 5,
		AvailableAt: time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC),
		LastError:   &lastError,
	}
	view := mailJobView(job)
	if !view.ErrorPresent {
		t.Fatal("mailJobView did not mark a failed job as having an error")
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal mail job view: %v", err)
	}
	if strings.Contains(string(body), lastError) || strings.Contains(string(body), "last_error") {
		t.Fatalf("mail job view exposed the stored error: %s", body)
	}
}

func TestListAdministratorsUsesPlatformAdminsAsBaseTable(t *testing.T) {
	recorder := &sqlRecorder{Interface: logger.Default}
	database, err := gorm.Open(
		postgres.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: recorder},
	)
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	service := NewService(database, nil, nil, false)
	principal := auth.Principal{Scope: constants.AuthScopePlatform}

	if _, err := service.ListAdministrators(context.Background(), principal, AdministratorQuery{}); err != nil {
		t.Fatalf("list active platform administrators: %v", err)
	}
	if _, err := service.ListAdministrators(context.Background(), principal, AdministratorQuery{IncludeInactive: true}); err != nil {
		t.Fatalf("list all platform administrators: %v", err)
	}
	if len(recorder.queries) != 2 {
		t.Fatalf("recorded %d queries, want 2", len(recorder.queries))
	}
	for _, sql := range recorder.queries {
		if !strings.Contains(sql, `FROM "platform_admins" JOIN users ON users.id = platform_admins.user_id`) {
			t.Fatalf("administrator list did not establish platform_admins as its base table: %s", sql)
		}
		if strings.Contains(sql, `FROM " JOIN`) {
			t.Fatalf("administrator list generated an unterminated quoted base table: %s", sql)
		}
	}
	if !strings.Contains(recorder.queries[0], `WHERE platform_admins.is_active = true`) {
		t.Fatalf("default administrator list did not restrict to active authority: %s", recorder.queries[0])
	}
	if strings.Contains(recorder.queries[1], `platform_admins.is_active = true`) {
		t.Fatalf("include-inactive administrator list retained active-only filter: %s", recorder.queries[1])
	}
}
