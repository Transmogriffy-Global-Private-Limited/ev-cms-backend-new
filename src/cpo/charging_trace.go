package cpo

import (
	"context"
	"sort"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
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
}
type ChargingTraceResponse struct {
	TraceID           uuid.UUID                `json:"trace_id"`
	SessionID         *uuid.UUID               `json:"session_id,omitempty"`
	HALTransactionID  *uuid.UUID               `json:"hal_transaction_id,omitempty"`
	OCPPTransactionID *int64                   `json:"ocpp_transaction_id,omitempty"`
	Events            []ChargingTraceEventView `json:"events"`
	CMSSource         string                   `json:"cms_source"`
	HALSource         string                   `json:"hal_source"`
	NextOccurredAt    *time.Time               `json:"next_occurred_at,omitempty"`
	NextEventID       *uuid.UUID               `json:"next_event_id,omitempty"`
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
			var intent models.ChargingStartIntent
			if err := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).First(&intent).Error; err != nil {
				return ChargingTraceResponse{}, chargingTraceNotFound()
			}
		}
	}
	rows := make([]models.ChargingTraceEvent, 0, limit+1)
	eventQuery := service.database.WithContext(ctx).Where("cpo_id = ? AND trace_id = ?", *principal.CPOID, traceID).Order("occurred_at DESC, id DESC")
	if before != nil && beforeID != nil {
		eventQuery = eventQuery.Where("(occurred_at, id) < (?, ?)", *before, *beforeID)
	}
	if err := eventQuery.Limit(limit + 1).Find(&rows).Error; err != nil {
		return ChargingTraceResponse{}, err
	}
	response := ChargingTraceResponse{TraceID: traceID, Events: make([]ChargingTraceEventView, 0, len(rows)), CMSSource: "AVAILABLE", HALSource: "NOT_REQUESTED"}
	if session.ID != uuid.Nil {
		response.SessionID = &session.ID
		response.HALTransactionID = session.HALTransactionID
		value := session.TransactionID
		response.OCPPTransactionID = &value
	}
	for _, row := range rows {
		response.Events = append(response.Events, ChargingTraceEventView{ID: row.ID.String(), TraceID: row.TraceID.String(), Source: row.Source, Target: row.Target, Category: row.Category, Protocol: row.Protocol, Phase: row.Phase, Summary: row.Summary, OccurredAt: row.OccurredAt, RecordedAt: row.RecordedAt, StateBefore: row.StateBefore, StateAfter: row.StateAfter, CorrelationID: row.CorrelationID, Data: row.Data})
	}
	if service.halOperations != nil {
		response.HALSource = "UNAVAILABLE"
		cursorTime, cursorID := time.Time{}, uuid.Nil
		if before != nil && beforeID != nil {
			cursorTime, cursorID = *before, *beforeID
		}
		if hal, err := service.halOperations.GetTrace(ctx, traceID, cursorTime, cursorID, limit+1); err == nil && hal.Trace.TraceID == traceID && hal.Trace.CPOID == *principal.CPOID {
			response.HALSource = "AVAILABLE"
			for _, event := range hal.Events {
				response.Events = append(response.Events, ChargingTraceEventView{ID: event.EventID.String(), TraceID: event.TraceID.String(), Source: event.Source, Target: event.Target, Category: event.Category, Protocol: event.Protocol, Phase: event.Phase, Summary: event.Summary, OccurredAt: event.OccurredAt, RecordedAt: event.RecordedAt, StateBefore: event.StateBefore, StateAfter: event.StateAfter, CorrelationID: event.CorrelationID, Data: models.JSONB(event.Data)})
			}
			sort.SliceStable(response.Events, func(i, j int) bool {
				if response.Events[i].OccurredAt.Equal(response.Events[j].OccurredAt) {
					return response.Events[i].ID > response.Events[j].ID
				}
				return response.Events[i].OccurredAt.After(response.Events[j].OccurredAt)
			})
		}
	}
	more := len(response.Events) > limit
	if more {
		response.Events = response.Events[:limit]
	}
	if more && len(response.Events) > 0 {
		last := response.Events[len(response.Events)-1]
		lastID, parseErr := uuid.Parse(last.ID)
		if parseErr == nil && lastID != uuid.Nil {
			response.NextOccurredAt = &last.OccurredAt
			response.NextEventID = &lastID
		}
	}
	return response, nil
}

func chargingTraceNotFound() *auth.APIError {
	return &auth.APIError{Status: 404, Code: "charging_trace_not_found", Message: "The charging trace was not found."}
}
