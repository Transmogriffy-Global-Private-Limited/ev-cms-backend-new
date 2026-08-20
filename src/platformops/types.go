package platformops

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

type EventInput struct {
	Type         string
	ActorUserID  *uuid.UUID
	ResourceType string
	ResourceID   *string
	Data         models.JSONB
}

type EventQuery struct {
	AfterID int64
	Limit   int
	Type    string
}

type EventPage struct {
	Events     []models.PlatformEvent `json:"events"`
	NextCursor int64                  `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
}

type AuditQuery struct {
	Before      *time.Time
	BeforeID    *uuid.UUID
	Limit       int
	Action      string
	Entity      string
	ActorUserID *uuid.UUID
	CPOID       *uuid.UUID
}

type AuditPage struct {
	Records      []models.AuditLog `json:"records"`
	NextBefore   *time.Time        `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID        `json:"next_before_id,omitempty"`
	HasMore      bool              `json:"has_more"`
}

type WorkerView struct {
	ID                 uuid.UUID    `json:"id"`
	Name               string       `json:"name"`
	InstanceKey        string       `json:"instance_key"`
	Status             string       `json:"status"`
	Required           bool         `json:"required"`
	StartedAt          time.Time    `json:"started_at"`
	LastHeartbeatAt    time.Time    `json:"last_heartbeat_at"`
	LastJobCompletedAt *time.Time   `json:"last_job_completed_at,omitempty"`
	Metadata           models.JSONB `json:"metadata"`
}

type WorkerListResponse struct {
	Workers []WorkerView `json:"workers"`
}

// WorkerSpec is application wiring's expected-worker contract. A durable row
// is an observation of this contract, never the authority that defines it.
type WorkerSpec struct {
	Name        string
	InstanceKey string
	Required    bool
	Enabled     bool
}
