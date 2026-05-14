package model

import "testing"

func TestAgentStatusValues(t *testing.T) {
	tests := []struct {
		name   string
		status AgentStatus
		want   string
	}{
		{"connected", AgentStatusConnected, "connected"},
		{"disconnected", AgentStatusDisconnected, "disconnected"},
		{"error", AgentStatusError, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("AgentStatus = %q, want %q", tt.status, tt.want)
			}
		})
	}
}

func TestTaskStateValues(t *testing.T) {
	tests := []struct {
		name  string
		state TaskState
		want  string
	}{
		{"submitted", TaskStateSubmitted, "SUBMITTED"},
		{"working", TaskStateWorking, "WORKING"},
		{"input_required", TaskStateInputRequired, "INPUT_REQUIRED"},
		{"completed", TaskStateCompleted, "COMPLETED"},
		{"failed", TaskStateFailed, "FAILED"},
		{"error", TaskStateError, "ERROR"},
		{"responded", TaskStateResponded, "RESPONDED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("TaskState = %q, want %q", tt.state, tt.want)
			}
		})
	}
}

func TestMessageRoleValues(t *testing.T) {
	tests := []struct {
		name string
		role MessageRole
		want string
	}{
		{"user", MessageRoleUser, "user"},
		{"agent", MessageRoleAgent, "agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.role) != tt.want {
				t.Errorf("MessageRole = %q, want %q", tt.role, tt.want)
			}
		})
	}
}

func TestTraceEventTypeValues(t *testing.T) {
	tests := []struct {
		name     string
		eventype TraceEventType
		want     string
	}{
		{"send", TraceEventSend, "send"},
		{"task_created", TraceEventTaskCreated, "task_created"},
		{"task_update", TraceEventTaskUpdate, "task_update"},
		{"artifact", TraceEventArtifact, "artifact"},
		{"response", TraceEventResponse, "response"},
		{"error", TraceEventError, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.eventype) != tt.want {
				t.Errorf("TraceEventType = %q, want %q", tt.eventype, tt.want)
			}
		})
	}
}
