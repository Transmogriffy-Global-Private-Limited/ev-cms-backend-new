package cpo

import (
	"context"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ChargingTraceEventView struct {
	ID            string       `json:"id"`
	TraceID       string       `json:"trace_id"`
	Source        string       `json:"source"`
	Target        string       `json:"target"`
	Category      string       `json:"category"`
	Protocol      string       `json:"protocol"`
	Phase         string       `json:"phase"`
	Summary       string       `json:"summary"`
	OccurredAt    time.Time    `json:"occurred_at"`
	RecordedAt    time.Time    `json:"recorded_at"`
	StateBefore   string       `json:"state_before,omitempty"`
	StateAfter    string       `json:"state_after,omitempty"`
	CorrelationID string       `json:"correlation_id,omitempty"`
	Data          models.JSONB `json:"data"`
	// IngestionSequence is a CMS replay cursor, not an authoritative charging fact.
	IngestionSequence int64 `json:"-"`
}
type ChargingTraceResponse struct {
	TraceID             uuid.UUID                `json:"trace_id"`
	StartIntentID       *uuid.UUID               `json:"cms_start_intent_id,omitempty"`
	SessionID           *uuid.UUID               `json:"session_id,omitempty"`
	CMSCommandID        *uuid.UUID               `json:"cms_command_id,omitempty"`
	HALTransactionID    *uuid.UUID               `json:"hal_transaction_id,omitempty"`
	OCPPTransactionID   *int64                   `json:"ocpp_transaction_id,omitempty"`
	ChargerOCPPIdentity string                   `json:"charger_ocpp_identity,omitempty"`
	OCPPConnectorNumber int                      `json:"ocpp_connector_number,omitempty"`
	Events              []ChargingTraceEventView `json:"events"`
	SourcesPresent      []string                 `json:"sources_present"`
	ReplayCursor        int64                    `json:"replay_cursor"`
	NextOccurredAt      *time.Time               `json:"next_occurred_at,omitempty"`
	NextEventID         *uuid.UUID               `json:"next_event_id,omitempty"`
}

type ChargingTraceReplayPage struct {
	Events     []ChargingTraceEventView
	NextCursor int64
}

// ListChargingTraceReplay uses CMS ingestion sequence rather than occurred_at:
// replay is delivery-order recovery, while waterfall display stays chronological.
func (service *Service) ListChargingTraceReplay(ctx context.Context, principal auth.Principal, traceID uuid.UUID, after int64, limit int) (ChargingTraceReplayPage, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargingTraceReplayPage{}, err
	}
	if traceID == uuid.Nil || after < 0 {
		return ChargingTraceReplayPage{}, chargingTraceNotFound()
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var root models.ChargingTrace
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).First(&root).Error; err != nil {
		return ChargingTraceReplayPage{}, chargingTraceNotFound()
	}
	rows := make([]models.ChargingTraceEvent, 0, limit)
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ? AND ingestion_sequence > ?", *principal.CPOID, traceID, after).Order("ingestion_sequence ASC").Limit(limit).Find(&rows).Error; err != nil {
		return ChargingTraceReplayPage{}, err
	}
	page := ChargingTraceReplayPage{Events: make([]ChargingTraceEventView, 0, len(rows)), NextCursor: after}
	for _, row := range rows {
		page.Events = append(page.Events, ChargingTraceEventView{ID: row.ID.String(), TraceID: row.TraceID.String(), Source: row.Source, Target: row.Target, Category: row.Category, Protocol: row.Protocol, Phase: row.Phase, Summary: row.Summary, OccurredAt: row.OccurredAt, RecordedAt: row.RecordedAt, StateBefore: row.StateBefore, StateAfter: row.StateAfter, CorrelationID: row.CorrelationID, Data: row.Data, IngestionSequence: row.IngestionSequence})
		page.NextCursor = row.IngestionSequence
	}
	return page, nil
}

// GetChargingTrace returns diagnostic evidence scoped by a real CPO session
// or trace identity. It does not compute, repair, or infer authoritative
// charging state from evidence rows.
func (service *Service) GetChargingTrace(ctx context.Context, principal auth.Principal, sessionID, traceID uuid.UUID, before *time.Time, beforeID *uuid.UUID, limit int) (ChargingTraceResponse, error) {
	if err := requireCPOContext(principal); err != nil {
		return ChargingTraceResponse{}, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := service.database.WithContext(ctx).Where("cpo_id = ?", *principal.CPOID)
	var session models.ChargingSession
	if sessionID != uuid.Nil {
		if err := query.First(&session, "id = ?", sessionID).Error; err != nil {
			return ChargingTraceResponse{}, chargingTraceNotFound()
		}
		if session.TraceID == nil {
			return ChargingTraceResponse{}, chargingTraceNotFound()
		}
		traceID = *session.TraceID
	} else {
		if traceID == uuid.Nil {
			return ChargingTraceResponse{}, chargingTraceNotFound()
		}
		if err := query.Where("trace_id = ?", traceID).First(&session).Error; err != nil {
			var root models.ChargingTrace
			if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).First(&root).Error; err != nil {
				var intent models.ChargingStartIntent
				if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).First(&intent).Error; err != nil {
					return ChargingTraceResponse{}, chargingTraceNotFound()
				}
			}
		}
	}
	var root models.ChargingTrace
	if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).First(&root).Error; err != nil && err != gorm.ErrRecordNotFound {
		return ChargingTraceResponse{}, err
	}
	rows := make([]models.ChargingTraceEvent, 0, limit+1)
	eventQuery := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).Order("occurred_at DESC, id DESC")
	if before != nil && beforeID != nil {
		eventQuery = eventQuery.Where("(occurred_at, id) < (?, ?)", *before, *beforeID)
	}
	if err := eventQuery.Limit(limit + 1).Find(&rows).Error; err != nil {
		return ChargingTraceResponse{}, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	response := chargingTraceResponseFromRoot(traceID, root)
	response.Events = make([]ChargingTraceEventView, 0, len(rows))
	response.SourcesPresent = []string{}
	if session.ID != uuid.Nil {
		response.SessionID = &session.ID
		response.HALTransactionID = session.HALTransactionID
		value := session.TransactionID
		response.OCPPTransactionID = &value
	}
	sources := map[string]struct{}{}
	var replayCursor int64
	for _, row := range rows {
		sources[row.Source] = struct{}{}
		if row.IngestionSequence > replayCursor {
			replayCursor = row.IngestionSequence
		}
		response.Events = append(response.Events, ChargingTraceEventView{ID: row.ID.String(), TraceID: row.TraceID.String(), Source: row.Source, Target: row.Target, Category: row.Category, Protocol: row.Protocol, Phase: row.Phase, Summary: row.Summary, OccurredAt: row.OccurredAt, RecordedAt: row.RecordedAt, StateBefore: row.StateBefore, StateAfter: row.StateAfter, CorrelationID: row.CorrelationID, Data: row.Data, IngestionSequence: row.IngestionSequence})
	}
	for _, source := range []string{"APP", "CMS", "HAL", "CHARGER"} {
		if _, ok := sources[source]; ok {
			response.SourcesPresent = append(response.SourcesPresent, source)
		}
	}
	if more && len(response.Events) > 0 {
		last := response.Events[len(response.Events)-1]
		lastID, parseErr := uuid.Parse(last.ID)
		if parseErr == nil && lastID != uuid.Nil {
			response.NextOccurredAt = &last.OccurredAt
			response.NextEventID = &lastID
		}
	}
	response.ReplayCursor = replayCursor
	return response, nil
}

func chargingTraceResponseFromRoot(traceID uuid.UUID, root models.ChargingTrace) ChargingTraceResponse {
	response := ChargingTraceResponse{TraceID: traceID}
	if root.TraceID == uuid.Nil {
		return response
	}
	response.StartIntentID = root.CMSStartIntentID
	response.SessionID = root.CMSChargingSessionID
	response.CMSCommandID = root.CMSCommandID
	response.HALTransactionID = root.HALTransactionID
	response.OCPPTransactionID = root.OCPPTransactionID
	response.ChargerOCPPIdentity = root.ChargerOCPPIdentity
	response.OCPPConnectorNumber = root.OCPPConnectorNumber
	return response
}

func chargingTraceNotFound() *auth.APIError {
	return &auth.APIError{Status: 404, Code: "charging_trace_not_found", Message: "The charging trace was not found."}
}
