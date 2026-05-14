package messagebus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return db
}

// newStreamingServer creates a fake agent server that responds with SSE events.
// Events are formatted as SSEEventEnvelope: {"type": "...", "data": {...}}.
func newStreamingServer(t *testing.T, responseText string) *httptest.Server {
	t.Helper()
	if responseText == "" {
		responseText = "Hello from the agent!"
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		// task event
		taskEnvelope := map[string]interface{}{
			"type": "task",
			"data": map[string]interface{}{
				"id":        "server-task-123",
				"contextId": "ctx-server",
				"status": map[string]string{
					"state": "SUBMITTED",
				},
			},
		}
		taskJSON, _ := json.Marshal(taskEnvelope)
		w.Write([]byte(fmt.Sprintf("data: %s\n\n", taskJSON)))

		// status_update event
		statusEnvelope := map[string]interface{}{
			"type": "status_update",
			"data": map[string]string{
				"state": "WORKING",
			},
		}
		statusJSON, _ := json.Marshal(statusEnvelope)
		w.Write([]byte(fmt.Sprintf("data: %s\n\n", statusJSON)))

		// message event (response)
		msgEnvelope := map[string]interface{}{
			"type": "message",
			"data": map[string]interface{}{
				"role": "agent",
				"parts": []map[string]string{
					{"kind": "text", "text": responseText},
				},
			},
		}
		msgJSON, _ := json.Marshal(msgEnvelope)
		w.Write([]byte(fmt.Sprintf("data: %s\n\n", msgJSON)))
	}))
}

// newNonSSEServer creates a fake agent that responds with JSON (non-SSE).
func newNonSSEServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		msgData := map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{
				{"kind": "text", "text": "Plain JSON response"},
			},
		}
		json.NewEncoder(w).Encode(msgData)
	}))
}

// newMessageBusWithServer creates a MessageBus with a registry getter pointing
// the given agent name to the test server URL.
func newMessageBusWithServer(t *testing.T, server *httptest.Server, agentName string) (*MessageBus, *sql.DB) {
	t.Helper()
	db := setupTestDB(t)

	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	tr := tracer.New(traceRepo)

	mb := NewMessageBus(taskRepo, messageRepo, tr)
	mb.SetRegistry(func(name string) *a2a.Client {
		if name == agentName {
			return a2a.NewClient(server.URL)
		}
		return nil
	})

	return mb, db
}

func TestSendStreaming(t *testing.T) {
	server := newStreamingServer(t, "Hello from the agent!")
	defer server.Close()

	mb, db := newMessageBusWithServer(t, server, "test-agent")
	defer db.Close()

	task, events, err := mb.SendStreaming("user", "test-agent", "Hi there", "ctx-1")
	if err != nil {
		t.Fatalf("SendStreaming failed: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.AgentName != "test-agent" {
		t.Errorf("expected agent_name 'test-agent', got '%s'", task.AgentName)
	}
	if task.State != model.TaskStateResponded {
		t.Errorf("expected state 'RESPONDED', got '%s'", task.State)
	}

	if len(events) == 0 {
		t.Fatal("expected non-empty events")
	}

	// Check that response event contains the text
	var foundResponse bool
	for _, evt := range events {
		if evt.Type == "response" {
			foundResponse = true
			if data, ok := evt.Data.(map[string]interface{}); ok {
				if text, ok := data["text"].(string); ok {
					if text != "Hello from the agent!" {
						t.Errorf("expected response text 'Hello from the agent!', got '%s'", text)
					}
				}
			}
		}
	}
	if !foundResponse {
		t.Error("expected to find response event")
	}

	// Check trace
	debugTrace := mb.GetDebugTrace(task.LocalTaskID)
	if !strings.Contains(debugTrace, "send") {
		t.Error("expected debug trace to contain 'send'")
	}
	if !strings.Contains(debugTrace, "response") {
		t.Error("expected debug trace to contain 'response'")
	}

	// Check messages in DB
	messages, err := mb.messageRepo.ListByTaskID(task.ID)
	if err != nil {
		t.Fatalf("ListByTaskID failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (user + agent), got %d", len(messages))
	}
	if messages[0].Role != model.MessageRoleUser {
		t.Errorf("expected first message role 'user', got '%s'", messages[0].Role)
	}
	if messages[1].Role != model.MessageRoleAgent {
		t.Errorf("expected second message role 'agent', got '%s'", messages[1].Role)
	}
}

func TestSend(t *testing.T) {
	server := newStreamingServer(t, "Agent reply")
	defer server.Close()

	mb, db := newMessageBusWithServer(t, server, "chat-agent")
	defer db.Close()

	text, err := mb.Send("user", "chat-agent", "Hello", "ctx-2")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if text != "Agent reply" {
		t.Errorf("expected 'Agent reply', got '%s'", text)
	}
}

func TestNonSSEAgent(t *testing.T) {
	server := newNonSSEServer(t)
	defer server.Close()

	mb, db := newMessageBusWithServer(t, server, "json-agent")
	defer db.Close()

	task, events, err := mb.SendStreaming("user", "json-agent", "ping", "")
	if err != nil {
		t.Fatalf("SendStreaming failed: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.State != model.TaskStateResponded {
		t.Errorf("expected state 'RESPONDED', got '%s'", task.State)
	}
	if len(events) == 0 {
		t.Fatal("expected non-empty events")
	}
}

func TestNonexistentAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	tr := tracer.New(traceRepo)

	mb := NewMessageBus(taskRepo, messageRepo, tr)
	mb.SetRegistry(func(name string) *a2a.Client {
		return nil
	})

	_, _, err := mb.SendStreaming("user", "nonexistent", "hello", "ctx")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' error, got '%s'", err.Error())
	}
}

func TestRegistryNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	tr := tracer.New(traceRepo)

	mb := NewMessageBus(taskRepo, messageRepo, tr)
	// Don't call SetRegistry

	_, _, err := mb.SendStreaming("user", "agent", "hello", "ctx")
	if err == nil {
		t.Fatal("expected error when registry not configured")
	}
	if !strings.Contains(err.Error(), "registry not configured") {
		t.Errorf("expected 'registry not configured' error, got '%s'", err.Error())
	}
}

func TestContextContinuation(t *testing.T) {
	server := newStreamingServer(t, "Second reply")
	defer server.Close()

	mb, db := newMessageBusWithServer(t, server, "ctx-agent")
	defer db.Close()

	// First message
	task1, events1, err := mb.SendStreaming("user", "ctx-agent", "First message", "ctx-continue")
	if err != nil {
		t.Fatalf("First SendStreaming failed: %v", err)
	}
	if task1 == nil {
		t.Fatal("expected non-nil task")
	}
	if len(events1) == 0 {
		t.Fatal("expected non-empty events")
	}

	// Verify the first event is a task event
	if events1[0].Type != "task" {
		t.Errorf("expected first event type 'task', got '%s'", events1[0].Type)
	}

	// Second message with same context
	task2, events2, err := mb.SendStreaming("user", "ctx-agent", "Follow up", "ctx-continue")
	if err != nil {
		t.Fatalf("Second SendStreaming failed: %v", err)
	}
	if task2 == nil {
		t.Fatal("expected non-nil task for second message")
	}

	if len(events2) == 0 {
		t.Fatal("expected non-empty events for second message")
	}

	// Both tasks should exist
	tasks, err := mb.ListTasks("ctx-agent", "")
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify task IDs are different
	if tasks[0].LocalTaskID == tasks[1].LocalTaskID {
		t.Error("expected different task IDs for two sends")
	}
}

func TestHandleToolCallListAgents(t *testing.T) {
	result := HandleToolCall("list_agents", map[string]interface{}{}, "http://localhost:8080")
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if url, ok := data["agents_url"].(string); !ok || url != "http://localhost:8080/api/agents" {
		t.Errorf("expected agents_url, got %v", data["agents_url"])
	}
}

func TestHandleToolCallSendToAgent(t *testing.T) {
	result := HandleToolCall("send_to_agent", map[string]interface{}{
		"agent_name": "target-agent",
		"message":    "hello",
	}, "http://localhost:8080")

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["status"] != "queued" {
		t.Errorf("expected status 'queued', got '%v'", data["status"])
	}
	if data["agent_name"] != "target-agent" {
		t.Errorf("expected agent_name 'target-agent', got '%v'", data["agent_name"])
	}
}

func TestHandleToolCallGetAgentInfo(t *testing.T) {
	result := HandleToolCall("get_agent_info", map[string]interface{}{
		"agent_name": "info-agent",
	}, "http://localhost:8080")

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["agent_name"] != "info-agent" {
		t.Errorf("expected agent_name 'info-agent', got '%v'", data["agent_name"])
	}
}

func TestHandleToolCallUnknown(t *testing.T) {
	result := HandleToolCall("nonexistent_tool", map[string]interface{}{}, "http://localhost:8080")
	if !strings.Contains(result, "unknown tool") {
		t.Errorf("expected 'unknown tool' error, got '%s'", result)
	}
}

func TestHostToolsDefinitions(t *testing.T) {
	if len(HostTools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(HostTools))
	}

	names := make(map[string]bool)
	for _, tool := range HostTools {
		name, _ := tool["name"].(string)
		names[name] = true
	}

	for _, expected := range []string{"list_agents", "send_to_agent", "get_agent_info"} {
		if !names[expected] {
			t.Errorf("expected tool '%s' not found", expected)
		}
	}
}
