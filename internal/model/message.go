package model

import "time"

type MessageRole string

const (
	MessageRoleUser  MessageRole = "user"
	MessageRoleAgent MessageRole = "agent"
)

type MessageRecord struct {
	ID        int64       `db:"id" json:"-"`
	TaskID    int64       `db:"task_id" json:"-"`
	Role      MessageRole `db:"role" json:"role"`
	Content   string      `db:"content" json:"content"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
}
