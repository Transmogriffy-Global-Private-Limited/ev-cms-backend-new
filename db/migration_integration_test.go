package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestMailOutboxTemplateConstraintAcceptsApplicationCatalogueWithPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	gormDB, sqlDB, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer sqlDB.Close()
	if err := ApplyMigrations(ctx, sqlDB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	now := time.Now().UTC()
	ids := make([]uuid.UUID, 0, len(cmsmail.SupportedDurableTemplateNames())+1)
	t.Cleanup(func() {
		if len(ids) > 0 {
			if err := gormDB.Where("id IN ?", ids).Delete(&models.MailOutbox{}).Error; err != nil {
				t.Errorf("clean up mail-outbox catalogue rows: %v", err)
			}
		}
	})

	for _, template := range cmsmail.SupportedDurableTemplateNames() {
		job := models.MailOutbox{
			ID:                uuid.New(),
			ToEmail:           "template-" + uuid.NewString() + "@example.invalid",
			Template:          template,
			PayloadCiphertext: []byte("test-ciphertext"),
			EncryptionKeyID:   "test-key",
			Status:            constants.MailOutboxPending,
			MaxAttempts:       1,
			AvailableAt:       now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := gormDB.Create(&job).Error; err != nil {
			t.Fatalf("insert application-supported template %q: %v", template, err)
		}
		ids = append(ids, job.ID)
	}

	unsupported := models.MailOutbox{
		ID:                uuid.New(),
		ToEmail:           "unsupported-" + uuid.NewString() + "@example.invalid",
		Template:          "UNSUPPORTED_TEMPLATE",
		PayloadCiphertext: []byte("test-ciphertext"),
		EncryptionKeyID:   "test-key",
		Status:            constants.MailOutboxPending,
		MaxAttempts:       1,
		AvailableAt:       now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := gormDB.Create(&unsupported).Error; err == nil {
		t.Fatal("mail_outbox CHECK accepted an unsupported template")
	}
}
