package model

import "time"

type TaskState string

const (
	TaskStateSubmitted     TaskState = "SUBMITTED"
	TaskStateWorking       TaskState = "WORKING"
	TaskStateInputRequired TaskState = "INPUT_REQUIRED"
	TaskStateCompleted     TaskState = "COMPLETED"
	TaskStateFailed        TaskState = "FAILED"
	TaskStateError         TaskState = "ERROR"
	TaskStateResponded     TaskState = "RESPONDED"
)

type TaskRecord struct {
	ID          int64     `db:"id" json:"-"`
	LocalTaskID string    `db:"local_task_id" json:"local_task_id"`
	AgentName   string    `db:"agent_name" json:"agent_name"`
	ContextID   string    `db:"context_id" json:"context_id"`
	State       TaskState `db:"state" json:"state"`
	DisplayID   string    `db:"display_id" json:"display_id"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}
