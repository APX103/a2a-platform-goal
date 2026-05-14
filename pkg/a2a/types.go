package a2a

import "encoding/json"

type AgentCard struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Version             string       `json:"version"`
	URL                 string       `json:"url"`
	Capabilities        Capabilities `json:"capabilities"`
	Skills              []Skill      `json:"skills"`
	SupportedInterfaces []string     `json:"supportedInterfaces,omitempty"`
}

type Capabilities struct {
	Streaming bool `json:"streaming"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

type Task struct {
	ID        string            `json:"id"`
	ContextID string            `json:"contextId,omitempty"`
	Status    TaskStatus        `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TaskStatus string

const (
	TaskStatusSubmitted     TaskStatus = "SUBMITTED"
	TaskStatusWorking       TaskStatus = "WORKING"
	TaskStatusInputRequired TaskStatus = "INPUT_REQUIRED"
	TaskStatusCompleted     TaskStatus = "COMPLETED"
	TaskStatusFailed        TaskStatus = "FAILED"
)

type Message struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type SendStreamingMessageParams struct {
	Message   Message `json:"message"`
	TaskID    string  `json:"taskId,omitempty"`
	ContextID string  `json:"contextId,omitempty"`
}

type SSEEventType string

const (
	SSEEventTask           SSEEventType = "task"
	SSEEventStatusUpdate   SSEEventType = "status_update"
	SSEEventArtifactUpdate SSEEventType = "artifact_update"
	SSEEventMessage        SSEEventType = "message"
)

type SSEEventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
