package mail

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
)

type senderStub struct {
	to       string
	template string
	payload  MessagePayload
}

func (sender *senderStub) SendMessage(
	_ context.Context,
	to string,
	template string,
	payload MessagePayload,
) error {
	sender.to = to
	sender.template = template
	sender.payload = payload
	return nil
}

func TestMailWorkerClaimsDecryptsAndCompletesJobWithPostgreSQL(t *testing.T) {
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

	box, err := security.NewSecretBox(
		"mail-worker-test-v1",
		[]byte(strings.Repeat("w", 32)),
	)
	if err != nil {
		t.Fatalf("create mail secret box: %v", err)
	}
	recipient := "worker-" + uuid.NewString() + "@example.com"
	payload := OTPPayload{
		RecipientName: "Worker Test",
		Code:          "846201",
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
	}
	outbox := NewOutbox(box)
	if err := outbox.EnqueueOTP(
		gormDB,
		recipient,
		"LOGIN_OTP",
		payload,
	); err != nil {
		t.Fatalf("enqueue mail: %v", err)
	}
	var job models.MailOutbox
	if err := gormDB.Where("to_email = ?", recipient).First(&job).Error; err != nil {
		t.Fatalf("load queued mail: %v", err)
	}
	if err := gormDB.Model(&models.MailOutbox{}).
		Where("id = ?", job.ID).
		Update("available_at", time.Now().UTC().Add(-24*time.Hour)).Error; err != nil {
		t.Fatalf("prioritize test mail: %v", err)
	}

	sender := &senderStub{}
	worker := NewWorker(gormDB, box, sender, time.Second, 5*time.Second)
	if err := worker.processOne(ctx); err != nil {
		t.Fatalf("process mail job: %v", err)
	}
	if sender.to != recipient ||
		sender.template != "LOGIN_OTP" ||
		sender.payload.Code != payload.Code {
		t.Fatalf("unexpected delivered mail: %#v", sender)
	}
	if err := gormDB.First(&job, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload mail job: %v", err)
	}
	if job.Status != "SENT" || job.SentAt == nil || job.Attempts != 1 {
		t.Fatalf("unexpected completed mail state: %#v", job)
	}
}
