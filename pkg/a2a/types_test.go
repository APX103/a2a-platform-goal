package a2a

import (
	"encoding/json"
	"testing"
)

func TestAgentCardUnmarshal(t *testing.T) {
	input := `{
		"name": "test-agent",
		"description": "A test agent",
		"version": "1.0.0",
		"url": "http://localhost:8080",
		"capabilities": {"streaming": true},
		"skills": [
			{
				"id": "s1",
				"name": "echo",
				"description": "Echoes input",
				"tags": ["test", "echo"],
				"examples": ["hello"]
			}
		],
		"supportedInterfaces": ["jsonrpc"]
	}`

	var card AgentCard
	if err := json.Unmarshal([]byte(input), &card); err != nil {
		t.Fatalf("failed to unmarshal AgentCard: %v", err)
	}

	if card.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", card.Name, "test-agent")
	}
	if card.Description != "A test agent" {
		t.Errorf("Description = %q, want %q", card.Description, "A test agent")
	}
	if card.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", card.Version, "1.0.0")
	}
	if card.URL != "http://localhost:8080" {
		t.Errorf("URL = %q, want %q", card.URL, "http://localhost:8080")
	}
	if !card.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = false, want true")
	}
	if len(card.Skills) != 1 {
		t.Fatalf("len(Skills) = %d, want 1", len(card.Skills))
	}
	if card.Skills[0].Name != "echo" {
		t.Errorf("Skills[0].Name = %q, want %q", card.Skills[0].Name, "echo")
	}
	if len(card.Skills[0].Tags) != 2 {
		t.Errorf("len(Skills[0].Tags) = %d, want 2", len(card.Skills[0].Tags))
	}
	if len(card.Skills[0].Examples) != 1 {
		t.Errorf("len(Skills[0].Examples) = %d, want 1", len(card.Skills[0].Examples))
	}
	if len(card.SupportedInterfaces) != 1 {
		t.Errorf("len(SupportedInterfaces) = %d, want 1", len(card.SupportedInterfaces))
	}
}

func TestAgentCardOmitEmpty(t *testing.T) {
	input := `{
		"name": "minimal",
		"description": "",
		"version": "0.1",
		"url": "http://localhost",
		"capabilities": {"streaming": false},
		"skills": []
	}`

	var card AgentCard
	if err := json.Unmarshal([]byte(input), &card); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if card.SupportedInterfaces != nil {
		t.Errorf("SupportedInterfaces should be nil when omitted, got %v", card.SupportedInterfaces)
	}
}

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "123",
		Method:  "message/send",
		Params: SendStreamingMessageParams{
			Message: Message{
				Role: "user",
				Parts: []Part{
					{Kind: "text", Text: "hello"},
				},
			},
			TaskID: "task-1",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal JSONRPCRequest: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal back: %v", err)
	}

	if decoded["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", decoded["jsonrpc"])
	}
	if decoded["id"] != "123" {
		t.Errorf("id = %v, want 123", decoded["id"])
	}
	if decoded["method"] != "message/send" {
		t.Errorf("method = %v, want message/send", decoded["method"])
	}
}

func TestSSEEventTypeValues(t *testing.T) {
	tests := []struct {
		name     string
		eventype SSEEventType
		want     string
	}{
		{"task", SSEEventTask, "task"},
		{"status_update", SSEEventStatusUpdate, "status_update"},
		{"artifact_update", SSEEventArtifactUpdate, "artifact_update"},
		{"message", SSEEventMessage, "message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.eventype) != tt.want {
				t.Errorf("SSEEventType = %q, want %q", tt.eventype, tt.want)
			}
		})
	}
}

func TestTaskStatusValues(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   string
	}{
		{"submitted", TaskStatusSubmitted, "SUBMITTED"},
		{"working", TaskStatusWorking, "WORKING"},
		{"input_required", TaskStatusInputRequired, "INPUT_REQUIRED"},
		{"completed", TaskStatusCompleted, "COMPLETED"},
		{"failed", TaskStatusFailed, "FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("TaskStatus = %q, want %q", tt.status, tt.want)
			}
		})
	}
}

func TestTaskMarshal(t *testing.T) {
	task := Task{
		ID:        "task-123",
		ContextID: "ctx-456",
		Status:    TaskStatusWorking,
		Metadata: map[string]string{
			"key": "value",
		},
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal Task: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal back: %v", err)
	}

	if decoded["id"] != "task-123" {
		t.Errorf("id = %v, want task-123", decoded["id"])
	}
	if decoded["contextId"] != "ctx-456" {
		t.Errorf("contextId = %v, want ctx-456", decoded["contextId"])
	}
	if decoded["status"] != "WORKING" {
		t.Errorf("status = %v, want WORKING", decoded["status"])
	}
}

func TestSSEEventEnvelope(t *testing.T) {
	envelope := SSEEventEnvelope{
		Type: string(SSEEventTask),
		Data: json.RawMessage(`{"id":"task-1","status":"COMPLETED"}`),
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal SSEEventEnvelope: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal back: %v", err)
	}

	if decoded["type"] != "task" {
		t.Errorf("type = %v, want task", decoded["type"])
	}
}
