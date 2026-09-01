package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const staleJobAge = 5 * time.Minute

type MessagePayload struct {
	RecipientName     string    `json:"recipient_name"`
	Code              string    `json:"code,omitempty"`
	ChallengeID       string    `json:"challenge_id,omitempty"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
	TemporaryPassword string    `json:"temporary_password,omitempty"`
	CPOName           string    `json:"cpo_name,omitempty"`
	CPOID             string    `json:"cpo_id,omitempty"`
	CPOAppID          string    `json:"cpo_app_id,omitempty"`
	Role              string    `json:"role,omitempty"`
	ActionURL         string    `json:"action_url,omitempty"`
	PlanName          string    `json:"plan_name,omitempty"`
	SupportSubject    string    `json:"support_subject,omitempty"`
	SupportStatus     string    `json:"support_status,omitempty"`
	OccurredAt        time.Time `json:"occurred_at,omitempty"`
}

type MessageContext struct {
	CPOID  *uuid.UUID
	UserID *uuid.UUID
}

type Outbox struct {
	box *security.SecretBox
}

func NewOutbox(box *security.SecretBox) *Outbox {
	return &Outbox{box: box}
}

func (outbox *Outbox) EnqueueMessage(
	tx *gorm.DB,
	toEmail string,
	template string,
	payload MessagePayload,
) error {
	return outbox.EnqueueMessageWithContext(
		tx,
		toEmail,
		template,
		payload,
		MessageContext{},
	)
}

func (outbox *Outbox) EnqueueMessageWithContext(
	tx *gorm.DB,
	toEmail string,
	template string,
	payload MessagePayload,
	messageContext MessageContext,
) error {
	if err := validateMessagePayload(template, payload); err != nil {
		return err
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(toEmail))
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode mail payload: %w", err)
	}
	ciphertext, err := outbox.box.Seal(body, mailAAD(template, normalizedEmail))
	if err != nil {
		return fmt.Errorf("encrypt mail payload: %w", err)
	}
	job := models.MailOutbox{
		ID:                uuid.New(),
		ToEmail:           normalizedEmail,
		CPOID:             messageContext.CPOID,
		UserID:            messageContext.UserID,
		Template:          template,
		PayloadCiphertext: ciphertext,
		EncryptionKeyID:   outbox.box.KeyID(),
		Status:            constants.MailOutboxPending,
		MaxAttempts:       8,
		AvailableAt:       time.Now().UTC(),
	}
	if err := tx.Create(&job).Error; err != nil {
		return fmt.Errorf("enqueue mail: %w", err)
	}
	return nil
}

func validateMessagePayload(template string, payload MessagePayload) error {
	if !isSupportedDurableTemplate(template) {
		return fmt.Errorf("validate mail payload: unknown template %q", template)
	}

	switch template {
	case "LOGIN_OTP", "CUSTOMER_LOGIN_OTP", "CUSTOMER_SIGNUP_OTP":
		if strings.TrimSpace(payload.Code) == "" || payload.ExpiresAt.IsZero() {
			return fmt.Errorf("validate %s mail payload: code and expiry are required", template)
		}
	case "PASSWORD_RESET_OTP", "CUSTOMER_PASSWORD_RESET_OTP":
		if _, err := uuid.Parse(strings.TrimSpace(payload.ChallengeID)); err != nil {
			return fmt.Errorf("validate %s mail payload: recovery challenge ID is required", template)
		}
		if strings.TrimSpace(payload.Code) == "" || payload.ExpiresAt.IsZero() {
			return fmt.Errorf("validate %s mail payload: reset code and expiry are required", template)
		}
	case "CPO_ADMIN_WELCOME":
		if strings.TrimSpace(payload.TemporaryPassword) == "" {
			return errors.New("validate CPO_ADMIN_WELCOME mail payload: temporary password is required")
		}
	case "CPO_STAFF_NEW_IDENTITY":
		if strings.TrimSpace(payload.TemporaryPassword) == "" || strings.TrimSpace(payload.CPOName) == "" || strings.TrimSpace(payload.Role) == "" || strings.TrimSpace(payload.ActionURL) == "" {
			return errors.New("validate CPO_STAFF_NEW_IDENTITY mail payload: temporary password, CPO name, role, and action URL are required")
		}
	case "PLATFORM_ADMIN_INVITE":
		if strings.TrimSpace(payload.TemporaryPassword) == "" {
			return errors.New("validate PLATFORM_ADMIN_INVITE mail payload: temporary password is required")
		}
	case "CPO_MEMBERSHIP_ASSIGNED":
		if strings.TrimSpace(payload.CPOName) == "" {
			return fmt.Errorf("validate %s mail payload: CPO name is required", template)
		}
	case "CPO_ONBOARDING_RESENT", "CPO_STAFF_EXISTING_IDENTITY", "CPO_STAFF_ROLE_CHANGED", "CPO_STAFF_SUSPENDED", "CPO_STAFF_REACTIVATED", "CPO_STAFF_REVOKED":
		if strings.TrimSpace(payload.CPOName) == "" || strings.TrimSpace(payload.ActionURL) == "" {
			return fmt.Errorf("validate %s mail payload: CPO name and action URL are required", template)
		}
	case "PASSWORD_CHANGE_REMINDER", "PLATFORM_ADMIN_GRANTED":
		return nil
	case "CPO_SUBSCRIPTION_EXPIRY_WARNING", "CPO_SUBSCRIPTION_EXPIRED":
		if strings.TrimSpace(payload.CPOName) == "" || payload.ExpiresAt.IsZero() {
			return fmt.Errorf("validate %s mail payload: CPO name and expiry are required", template)
		}
	case "CPO_SUPPORT_TICKET_CREATED", "CPO_SUPPORT_TICKET_PLATFORM_REPLY", "CPO_SUPPORT_TICKET_RESOLVED", "CPO_SUPPORT_TICKET_CLOSED", "CPO_SUPPORT_TICKET_REOPENED":
		if strings.TrimSpace(payload.CPOName) == "" || strings.TrimSpace(payload.SupportSubject) == "" || strings.TrimSpace(payload.SupportStatus) == "" || payload.OccurredAt.IsZero() || strings.TrimSpace(payload.ActionURL) == "" {
			return fmt.Errorf("validate %s mail payload: CPO name, subject, status, time, and action URL are required", template)
		}
	default:
		return fmt.Errorf("validate mail payload: template %q has no validation rule", template)
	}
	return nil
}

type Sender interface {
	SendMessage(context.Context, string, string, MessagePayload) error
}

type WorkerObserver interface {
	Heartbeat(context.Context, string, string) error
	JobCompleted(context.Context, string, string) error
}

type Worker struct {
	database    *gorm.DB
	box         *security.SecretBox
	sender      Sender
	pollEvery   time.Duration
	sendTimeout time.Duration
	observer    WorkerObserver
	workerName  string
	instanceKey string
	lastBeat    time.Time
}

func NewWorker(
	database *gorm.DB,
	box *security.SecretBox,
	sender Sender,
	pollEvery time.Duration,
	sendTimeout time.Duration,
) *Worker {
	return &Worker{
		database:    database,
		box:         box,
		sender:      sender,
		pollEvery:   pollEvery,
		sendTimeout: sendTimeout,
	}
}

func (worker *Worker) WithObserver(
	observer WorkerObserver,
	workerName string,
	instanceKey string,
) *Worker {
	worker.observer = observer
	worker.workerName = workerName
	worker.instanceKey = instanceKey
	return worker
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.pollEvery)
	defer ticker.Stop()

	for {
		worker.recordHeartbeat(ctx)
		if err := worker.processOne(ctx); err != nil && ctx.Err() == nil {
			log.Printf("mail outbox delivery failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (worker *Worker) recordHeartbeat(ctx context.Context) {
	if worker.observer == nil || time.Since(worker.lastBeat) < 10*time.Second {
		return
	}
	if err := worker.observer.Heartbeat(
		ctx,
		worker.workerName,
		worker.instanceKey,
	); err != nil && ctx.Err() == nil {
		log.Printf("record mail worker heartbeat: %v", err)
		return
	}
	worker.lastBeat = time.Now()
}

func (worker *Worker) processOne(ctx context.Context) error {
	var job models.MailOutbox
	result := worker.database.WithContext(ctx).Raw(`
		UPDATE mail_outbox
		SET status = 'PROCESSING',
		    locked_at = now(),
		    attempts = attempts + 1,
		    updated_at = now()
		WHERE id = (
		    SELECT id
		    FROM mail_outbox
		    WHERE attempts < max_attempts
		      AND available_at <= now()
		      AND (
		          status = 'PENDING'
		          OR (
		              status = 'PROCESSING'
		              AND locked_at < now() - (? * interval '1 second')
		          )
		      )
		    ORDER BY available_at, created_at
		    FOR UPDATE SKIP LOCKED
		    LIMIT 1
		)
		RETURNING *
	`, staleJobAge.Seconds()).Scan(&job)
	if result.Error != nil {
		return fmt.Errorf("claim mail job: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil
	}

	if job.EncryptionKeyID != worker.box.KeyID() {
		return worker.failJob(ctx, job, fmt.Errorf("mail encryption key %q unavailable", job.EncryptionKeyID))
	}
	plaintext, err := worker.box.Open(
		job.PayloadCiphertext,
		mailAAD(job.Template, job.ToEmail),
	)
	if err != nil {
		return worker.failJob(ctx, job, err)
	}
	var payload MessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return worker.failJob(ctx, job, fmt.Errorf("decode mail payload: %w", err))
	}

	sendContext, cancel := context.WithTimeout(ctx, worker.sendTimeout)
	defer cancel()
	if err := worker.sender.SendMessage(sendContext, job.ToEmail, job.Template, payload); err != nil {
		return worker.failJob(ctx, job, err)
	}

	now := time.Now().UTC()
	result = worker.database.WithContext(ctx).Model(&models.MailOutbox{}).
		Where("id = ? AND status = ?", job.ID, constants.MailOutboxProcessing).
		Updates(map[string]any{
			"status":     constants.MailOutboxSent,
			"sent_at":    now,
			"locked_at":  nil,
			"last_error": nil,
			"updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("mark mail job sent: %w", result.Error)
	}
	if worker.observer != nil {
		if err := worker.observer.JobCompleted(
			ctx,
			worker.workerName,
			worker.instanceKey,
		); err != nil {
			log.Printf("record mail worker completion: %v", err)
		}
	}
	return nil
}

func (worker *Worker) failJob(
	ctx context.Context,
	job models.MailOutbox,
	deliveryError error,
) error {
	now := time.Now().UTC()
	status := constants.MailOutboxPending
	availableAt := now.Add(retryDelay(job.Attempts))
	if job.Attempts >= job.MaxAttempts {
		status = constants.MailOutboxFailed
		availableAt = now
	}
	errorText := deliveryError.Error()
	if len(errorText) > 500 {
		errorText = errorText[:500]
	}
	result := worker.database.WithContext(ctx).Model(&models.MailOutbox{}).
		Where("id = ? AND status = ?", job.ID, constants.MailOutboxProcessing).
		Updates(map[string]any{
			"status":       status,
			"available_at": availableAt,
			"locked_at":    nil,
			"last_error":   errorText,
			"updated_at":   now,
		})
	if result.Error != nil {
		return fmt.Errorf("record mail delivery failure: %w", result.Error)
	}
	return fmt.Errorf("deliver mail job %s: %w", job.ID, deliveryError)
}

func retryDelay(attempt int) time.Duration {
	delay := time.Minute
	for current := 1; current < attempt && delay < time.Hour; current++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func mailAAD(template, recipient string) []byte {
	return []byte("ev-cms-mail:" + template + ":" + strings.ToLower(strings.TrimSpace(recipient)))
}
