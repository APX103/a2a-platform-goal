package model

import "time"

type TraceEventType string

const (
	TraceEventSend        TraceEventType = "send"
	TraceEventTaskCreated TraceEventType = "task_created"
	TraceEventTaskUpdate  TraceEventType = "task_update"
	TraceEventArtifact    TraceEventType = "artifact"
	TraceEventResponse    TraceEventType = "response"
	TraceEventError       TraceEventType = "error"
)

type TraceEventRecord struct {
	ID          int64          `db:"id" json:"-"`
	TaskID      string         `db:"task_id" json:"task_id"`
	ContextID   string         `db:"context_id" json:"context_id"`
	AgentName   string         `db:"agent_name" json:"agent_name"`
	TargetAgent string         `db:"target_agent" json:"target_agent,omitempty"`
	EventType   TraceEventType `db:"event_type" json:"event_type"`
	DataJSON    string         `db:"data_json" json:"-"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
}
