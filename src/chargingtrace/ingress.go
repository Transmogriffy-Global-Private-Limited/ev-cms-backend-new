// Package chargingtrace owns the isolated HAL diagnostic ingestion boundary.
// It deliberately does not project, repair, or decide charging business state.
package chargingtrace

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Envelope struct {
	SchemaVersion          int          `json:"schema_version"`
	TraceID                uuid.UUID    `json:"trace_id"`
	EventID                uuid.UUID    `json:"event_id"`
	CPOID                  uuid.UUID    `json:"cpo_id"`
	CMSStartIntentID       *uuid.UUID   `json:"cms_start_intent_id"`
	CMSChargingSessionID   *uuid.UUID   `json:"cms_charging_session_id"`
	CMSCommandID           *uuid.UUID   `json:"cms_command_id"`
	HALTransactionID       *uuid.UUID   `json:"hal_transaction_id"`
	OCPPTransactionID      *int64       `json:"ocpp_transaction_id"`
	ChargerOCPPIdentity    string       `json:"charger_ocpp_identity"`
	OCPPConnectorNumber    int          `json:"ocpp_connector_number"`
	Source                 string       `json:"source"`
	Target                 string       `json:"target"`
	Category               string       `json:"category"`
	Protocol               string       `json:"protocol"`
	Phase                  string       `json:"phase"`
	Summary                string       `json:"summary"`
	OccurredAt             time.Time    `json:"occurred_at"`
	StateBefore            string       `json:"state_before"`
	StateAfter             string       `json:"state_after"`
	CorrelationID          string       `json:"correlation_id"`
	Data                   models.JSONB `json:"data"`
	ImmutableContentSHA256 string       `json:"immutable_content_sha256"`
}

type Error struct {
	Status        int
	Code, Message string
}

func (err *Error) Error() string { return err.Code }

type Ingestor struct {
	database *gorm.DB
	bearer   string
	now      func() time.Time
}

func NewIngestor(database *gorm.DB, bearer string) *Ingestor {
	return &Ingestor{database: database, bearer: strings.TrimSpace(bearer), now: func() time.Time { return time.Now().UTC() }}
}

func (ingestor *Ingestor) Accept(ctx context.Context, bearer string, envelope Envelope) error {
	if ingestor == nil || ingestor.bearer == "" || len(bearer) != len(ingestor.bearer) || subtle.ConstantTimeCompare([]byte(bearer), []byte(ingestor.bearer)) != 1 {
		return &Error{Status: 401, Code: "hal_trace_authentication_required", Message: "Trace service authentication is required."}
	}
	if err := validateEnvelope(envelope); err != nil {
		return err
	}
	if ingestor.database == nil {
		return &Error{Status: 500, Code: "hal_trace_storage_unavailable", Message: "The trace event could not be processed."}
	}
	digest, err := Digest(envelope)
	if err != nil {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(envelope.ImmutableContentSHA256)) != 1 {
		return &Error{Status: 409, Code: "hal_trace_integrity_conflict", Message: "The trace event immutable content conflicts."}
	}
	return ingestor.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.ChargingTraceEvent
		err := tx.First(&existing, "id = ?", envelope.EventID).Error
		if err == nil {
			if subtle.ConstantTimeCompare([]byte(existing.ImmutableContentSHA256), []byte(digest)) == 1 {
				return nil
			}
			return &Error{Status: 409, Code: "hal_trace_event_conflict", Message: "The trace event identity conflicts."}
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		root, err := ingestor.adoptRoot(tx, envelope)
		if err != nil {
			return err
		}
		if err := tx.Create(&models.ChargingTraceEvent{ID: envelope.EventID, TraceID: root.TraceID, CPOID: root.CPOID, SessionID: root.CMSChargingSessionID, Source: envelope.Source, Target: envelope.Target, Category: envelope.Category, Protocol: envelope.Protocol, Phase: envelope.Phase, Summary: envelope.Summary, OccurredAt: envelope.OccurredAt.UTC(), RecordedAt: ingestor.now(), StateBefore: envelope.StateBefore, StateAfter: envelope.StateAfter, CorrelationID: envelope.CorrelationID, Data: sanitize(envelope.Data), ImmutableContentSHA256: digest}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (ingestor *Ingestor) adoptRoot(tx *gorm.DB, envelope Envelope) (models.ChargingTrace, error) {
	var root models.ChargingTrace
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&root, "trace_id = ?", envelope.TraceID).Error
	if err == gorm.ErrRecordNotFound {
		root = models.ChargingTrace{TraceID: envelope.TraceID, CPOID: envelope.CPOID, CMSStartIntentID: envelope.CMSStartIntentID, CMSChargingSessionID: envelope.CMSChargingSessionID, CMSCommandID: envelope.CMSCommandID, HALTransactionID: envelope.HALTransactionID, OCPPTransactionID: envelope.OCPPTransactionID, ChargerOCPPIdentity: strings.TrimSpace(envelope.ChargerOCPPIdentity), OCPPConnectorNumber: envelope.OCPPConnectorNumber, CreatedAt: ingestor.now(), UpdatedAt: ingestor.now()}
		if err := tx.Create(&root).Error; err != nil {
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) && pgError.Code == "23505" {
				return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
			}
			return models.ChargingTrace{}, err
		}
		return root, nil
	}
	if err != nil {
		return models.ChargingTrace{}, err
	}
	if root.CPOID != envelope.CPOID {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	updates := map[string]any{"updated_at": ingestor.now()}
	if err := monotonicUUID(root.CMSStartIntentID, envelope.CMSStartIntentID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.CMSChargingSessionID, envelope.CMSChargingSessionID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.CMSCommandID, envelope.CMSCommandID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicUUID(root.HALTransactionID, envelope.HALTransactionID); err != nil {
		return models.ChargingTrace{}, err
	}
	if err := monotonicInt64(root.OCPPTransactionID, envelope.OCPPTransactionID); err != nil {
		return models.ChargingTrace{}, err
	}
	for _, field := range []struct {
		current, incoming any
		column            string
	}{{root.CMSStartIntentID, envelope.CMSStartIntentID, "cms_start_intent_id"}, {root.CMSChargingSessionID, envelope.CMSChargingSessionID, "cms_charging_session_id"}, {root.CMSCommandID, envelope.CMSCommandID, "cms_command_id"}, {root.HALTransactionID, envelope.HALTransactionID, "hal_transaction_id"}, {root.OCPPTransactionID, envelope.OCPPTransactionID, "ocpp_transaction_id"}} {
		if field.current == nil && field.incoming != nil {
			updates[field.column] = field.incoming
		}
	}
	if root.ChargerOCPPIdentity == "" {
		updates["charger_ocpp_identity"] = strings.TrimSpace(envelope.ChargerOCPPIdentity)
	} else if root.ChargerOCPPIdentity != envelope.ChargerOCPPIdentity {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	if root.OCPPConnectorNumber == 0 {
		updates["ocpp_connector_number"] = envelope.OCPPConnectorNumber
	} else if root.OCPPConnectorNumber != envelope.OCPPConnectorNumber {
		return models.ChargingTrace{}, &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	if err := tx.Model(&root).Updates(updates).Error; err != nil {
		return models.ChargingTrace{}, err
	}
	if err := tx.First(&root, "trace_id = ?", envelope.TraceID).Error; err != nil {
		return models.ChargingTrace{}, err
	}
	return root, nil
}

func monotonicUUID(current, incoming *uuid.UUID) error {
	if current == nil || incoming == nil {
		return nil
	}
	if *current != *incoming {
		return &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	return nil
}
func monotonicInt64(current, incoming *int64) error {
	if current == nil || incoming == nil {
		return nil
	}
	if *current != *incoming {
		return &Error{Status: 409, Code: "hal_trace_root_conflict", Message: "The trace identity conflicts."}
	}
	return nil
}
func validateEnvelope(e Envelope) error {
	if e.SchemaVersion != 1 || e.TraceID == uuid.Nil || e.EventID == uuid.Nil || e.CPOID == uuid.Nil || e.OCPPConnectorNumber < 1 || e.OccurredAt.IsZero() || !validText(e.ChargerOCPPIdentity, 255) || !validText(e.Source, 32) || !validText(e.Target, 32) || !validText(e.Category, 48) || !validText(e.Protocol, 24) || !validText(e.Phase, 24) || !validText(e.Summary, 200) || len(e.StateBefore) > 64 || len(e.StateAfter) > 64 || len(e.CorrelationID) > 128 || len(e.ImmutableContentSHA256) != 64 || e.Data == nil {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if e.OCPPTransactionID != nil && *e.OCPPTransactionID <= 0 {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if !validActor(e.Source) || !validActor(e.Target) || !validPhase(e.Phase) {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	if len(e.Data) > 16 {
		return &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."}
	}
	return nil
}
func validText(v string, n int) bool { return strings.TrimSpace(v) != "" && len(v) <= n }
func validActor(v string) bool       { return v == "APP" || v == "CMS" || v == "HAL" || v == "CHARGER" }
func validPhase(v string) bool {
	return v == "STARTING" || v == "CHARGING" || v == "STOPPING" || v == "POST_STOP"
}
func sanitize(input models.JSONB) models.JSONB {
	output := models.JSONB{}
	for _, key := range []string{"action", "result", "status", "transaction_id", "connector_id", "meter_wh", "reason", "error_class"} {
		if value, ok := input[key]; ok {
			output[key] = value
		}
	}
	return output
}
func Digest(envelope Envelope) (string, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	var immutable map[string]any
	if err := json.Unmarshal(raw, &immutable); err != nil {
		return "", err
	}
	delete(immutable, "immutable_content_sha256")
	raw, err = json.Marshal(immutable)
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
