package model

import "time"

type AgentStatus string

const (
	AgentStatusConnected    AgentStatus = "connected"
	AgentStatusDisconnected AgentStatus = "disconnected"
	AgentStatusError        AgentStatus = "error"
)

type AgentRecord struct {
	ID           int64       `db:"id" json:"-"`
	Name         string      `db:"name" json:"name"`
	URL          string      `db:"url" json:"url"`
	Description  string      `db:"description" json:"description"`
	Version      string      `db:"version" json:"version"`
	AgentType    string      `db:"type" json:"type"`
	Status       AgentStatus `db:"status" json:"status"`
	SkillsJSON   string      `db:"skills_json" json:"-"`
	Skills       []string    `db:"-" json:"skills"`
	ErrorMessage string      `db:"error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time   `db:"updated_at" json:"updated_at"`
}
