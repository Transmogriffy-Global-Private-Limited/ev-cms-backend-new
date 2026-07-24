package models

import (
	"time"

	"github.com/google/uuid"
)

type PlatformEvent struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType    string     `gorm:"type:varchar(150);not null;index" json:"type"`
	ActorUserID  *uuid.UUID `gorm:"type:uuid;index" json:"actor_user_id,omitempty"`
	ResourceType string     `gorm:"type:varchar(100);not null" json:"resource_type"`
	ResourceID   *string    `gorm:"type:varchar(255)" json:"resource_id,omitempty"`
	Data         JSONB      `gorm:"type:jsonb;not null;default:'{}'" json:"data"`
	OccurredAt   time.Time  `gorm:"not null;index" json:"occurred_at"`
	ExpiresAt    time.Time  `gorm:"not null;index" json:"-"`
}

type WorkerInstance struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkerName         string     `gorm:"type:varchar(100);not null;uniqueIndex:uq_worker_identity,priority:1" json:"name"`
	InstanceKey        string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_worker_identity,priority:2" json:"instance_key"`
	Required           bool       `gorm:"not null;default:true" json:"required"`
	ReportedStatus     string     `gorm:"type:varchar(20);not null;default:'HEALTHY'" json:"-"`
	StartedAt          time.Time  `gorm:"not null" json:"started_at"`
	LastHeartbeatAt    time.Time  `gorm:"not null;index" json:"last_heartbeat_at"`
	LastJobCompletedAt *time.Time `json:"last_job_completed_at,omitempty"`
	Metadata           JSONB      `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	UpdatedAt          time.Time  `gorm:"not null" json:"updated_at"`
}
