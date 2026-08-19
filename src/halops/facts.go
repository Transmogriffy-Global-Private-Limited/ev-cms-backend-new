package halops

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FactEnvelope struct {
	FactID                 uuid.UUID       `json:"fact_id"`
	FactType               string          `json:"fact_type"`
	SchemaVersion          int             `json:"schema_version"`
	OccurredAt             time.Time       `json:"occurred_at"`
	Producer               string          `json:"producer"`
	ImmutableContentSHA256 string          `json:"immutable_content_sha256"`
	Payload                json.RawMessage `json:"payload"`
}

type FactProjector interface {
	ApplyHALFactProjection(tx *gorm.DB, envelope FactEnvelope, payload models.JSONB) error
}

type FactIngestor struct {
	database  *gorm.DB
	bearer    string
	projector FactProjector
	now       func() time.Time
}

func NewFactIngestor(database *gorm.DB, bearer string, projector FactProjector) *FactIngestor {
	return &FactIngestor{database: database, bearer: strings.TrimSpace(bearer), projector: projector, now: func() time.Time { return time.Now().UTC() }}
}

type FactError struct {
	Status        int
	Code, Message string
}

func (err *FactError) Error() string { return err.Message }

// FactProjectionError is an integration-boundary error: HAL supplied an
// authenticated, immutable fact, but CMS cannot apply its business projection.
// It deliberately carries only the stable public classification. The wrapped
// cause remains available to safe request diagnostics and is never returned to
// HAL.
type FactProjectionError struct {
	Status        int
	Code, Message string
	Cause         error
}

func (err *FactProjectionError) Error() string { return err.Message }
func (err *FactProjectionError) Unwrap() error { return err.Cause }

func NewFactProjectionError(status int, code, message string, cause error) *FactProjectionError {
	return &FactProjectionError{Status: status, Code: code, Message: message, Cause: cause}
}

// Accept validates immutable provider truth and records its receipt in the same
// transaction as projection application. Exact duplicates are intentionally a
// no-op; altered fact identity reuse is an integrity conflict.
func (ingestor *FactIngestor) Accept(ctx context.Context, bearer string, envelope FactEnvelope) error {
	if ingestor == nil || ingestor.projector == nil || ingestor.bearer == "" || len(bearer) != len(ingestor.bearer) || subtle.ConstantTimeCompare([]byte(bearer), []byte(ingestor.bearer)) != 1 {
		return &FactError{Status: 401, Code: "hal_fact_authentication_required", Message: "Service authentication is required."}
	}
	if envelope.FactID == uuid.Nil || envelope.SchemaVersion != 1 || envelope.Producer != "ocpp-hal-go-new" || strings.TrimSpace(envelope.FactType) == "" || len(envelope.Payload) == 0 || !validFactDigest(envelope.ImmutableContentSHA256) || envelope.OccurredAt.IsZero() {
		return &FactError{Status: 400, Code: "invalid_hal_fact", Message: "The HAL fact envelope is invalid."}
	}
	var payload models.JSONB
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return &FactError{Status: 400, Code: "invalid_hal_fact", Message: "The HAL fact payload is invalid."}
	}
	digest, err := CanonicalFactDigest(envelope)
	if err != nil {
		return fmt.Errorf("canonicalize HAL fact: %w", err)
	}
	if digest != envelope.ImmutableContentSHA256 {
		return &FactError{Status: 409, Code: "fact_integrity_violation", Message: "The HAL fact digest does not match its immutable content."}
	}
	return ingestor.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior models.HALFactReceipt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "fact_id = ?", envelope.FactID).Error; err == nil {
			if prior.Digest == digest {
				return nil
			}
			return &FactError{Status: 409, Code: "fact_integrity_violation", Message: "The HAL fact ID was reused with altered content."}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := ingestor.projector.ApplyHALFactProjection(tx, envelope, payload); err != nil {
			return err
		}
		return tx.Create(&models.HALFactReceipt{FactID: envelope.FactID, FactType: envelope.FactType, Digest: digest, OccurredAt: envelope.OccurredAt, Payload: payload, ProcessedAt: ingestor.now()}).Error
	})
}

func CanonicalFactDigest(envelope FactEnvelope) (string, error) {
	var payload any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return "", err
	}
	raw, err := json.Marshal(map[string]any{"fact_id": envelope.FactID.String(), "fact_type": envelope.FactType, "schema_version": envelope.SchemaVersion, "occurred_at": envelope.OccurredAt.UTC(), "producer": envelope.Producer, "payload": payload})
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func validFactDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
