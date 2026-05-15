// +build !docker

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDiscovery_ListAgents registers 2 agents and verifies the list has 2 entries with proper fields.
func TestDiscovery_ListAgents(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "agent-one")
	env.StartFakeAgent(t, "sequence", "agent-two")

	agents, status := env.GetJSONArray(t, "/api/agents")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	// Verify each agent has expected fields
	for _, a := range agents {
		agent, ok := a.(map[string]interface{})
		if !ok {
			t.Fatalf("agent is not a map: %v", a)
		}

		// Check required fields exist
		if agent["name"] == nil {
			t.Error("agent missing 'name' field")
		}
		if agent["description"] == nil {
			t.Error("agent missing 'description' field")
		}
		if agent["version"] == nil {
			t.Error("agent missing 'version' field")
		}
		if agent["skills"] == nil {
			t.Error("agent missing 'skills' field")
		}
	}
}

// TestDiscovery_GetAgent retrieves a single agent by name and verifies fields.
func TestDiscovery_GetAgent(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "get-test-agent")

	agent, status := env.GetJSON(t, "/api/agents/get-test-agent")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if agent["name"] != "get-test-agent" {
		t.Errorf("expected name=get-test-agent, got %v", agent["name"])
	}
	if agent["description"] == nil {
		t.Error("missing description")
	}
	if agent["version"] == nil {
		t.Error("missing version")
	}
}

// TestDiscovery_GetNonexistentAgent verifies 404 with error JSON for a nonexistent agent.
func TestDiscovery_GetNonexistentAgent(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	result, status := env.GetJSON(t, "/api/agents/nonexistent")
	if status != 404 {
		t.Fatalf("expected status 404, got %d", status)
	}

	errMsg, ok := result["error"].(string)
	if !ok {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errMsg, "nonexistent") {
		t.Errorf("expected error to contain 'nonexistent', got %s", errMsg)
	}
}

// TestDiscovery_Capabilities verifies GET /api/capabilities returns expected structure.
func TestDiscovery_Capabilities(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	caps, status := env.GetJSON(t, "/api/capabilities")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if caps["name"] != "a2a-platform-host" {
		t.Errorf("expected name=a2a-platform-host, got %v", caps["name"])
	}
	if caps["description"] != "A2A protocol host platform" {
		t.Errorf("expected description='A2A protocol host platform', got %v", caps["description"])
	}
	if caps["version"] != "0.1.0" {
		t.Errorf("expected version=0.1.0, got %v", caps["version"])
	}

	capMap, ok := caps["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities is not a map")
	}
	if capMap["streaming"] != true {
		t.Errorf("expected streaming=true, got %v", capMap["streaming"])
	}

	tools, ok := caps["tools"].([]interface{})
	if !ok {
		t.Fatal("tools is not an array")
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tools")
	}
}

// TestDiscovery_CapabilitiesConsistency verifies capabilities structure is the same with 0 agents.
func TestDiscovery_CapabilitiesConsistency(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// No agents registered - get capabilities
	caps, status := env.GetJSON(t, "/api/capabilities")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if caps["name"] != "a2a-platform-host" {
		t.Errorf("expected name=a2a-platform-host, got %v", caps["name"])
	}
	if caps["version"] != "0.1.0" {
		t.Errorf("expected version=0.1.0, got %v", caps["version"])
	}

	// Verify tools exist even with 0 agents
	tools, ok := caps["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("expected non-empty tools even with 0 agents")
	}
}

// TestDiscovery_ListTasks sends 2 messages and verifies >= 2 tasks.
func TestDiscovery_ListTasks(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "task-agent")

	// Send 2 messages
	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "task-agent",
		"message":    "message 1",
	})
	if s1 != 200 {
		t.Fatalf("message 1: expected status 200, got %d", s1)
	}

	_, s2 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "task-agent",
		"message":    "message 2",
	})
	if s2 != 200 {
		t.Fatalf("message 2: expected status 200, got %d", s2)
	}

	// List all tasks
	tasks, status := env.GetJSONArray(t, "/api/tasks")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(tasks) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", len(tasks))
	}
}

// TestDiscovery_FilterTasksByAgent filters tasks by agent_name.
func TestDiscovery_FilterTasksByAgent(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "filter-agent-a")
	env.StartFakeAgent(t, "echo", "filter-agent-b")

	// Send message to agent-a
	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "filter-agent-a",
		"message":    "hello a",
	})
	if s1 != 200 {
		t.Fatalf("message a: expected status 200, got %d", s1)
	}

	// Send message to agent-b
	_, s2 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "filter-agent-b",
		"message":    "hello b",
	})
	if s2 != 200 {
		t.Fatalf("message b: expected status 200, got %d", s2)
	}

	// Filter by agent-a
	tasksA, status := env.GetJSONArray(t, "/api/tasks?agent_name=filter-agent-a")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(tasksA) < 1 {
		t.Fatalf("expected at least 1 task for filter-agent-a, got %d", len(tasksA))
	}

	// Verify all tasks belong to filter-agent-a
	for _, task := range tasksA {
		tm, ok := task.(map[string]interface{})
		if !ok {
			continue
		}
		if tm["agent_name"] != "filter-agent-a" {
			t.Errorf("expected agent_name=filter-agent-a, got %v", tm["agent_name"])
		}
	}
}

// TestDiscovery_FilterTasksByState filters tasks by state.
func TestDiscovery_FilterTasksByState(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "state-agent")

	// Send message (will create RESPONDED task via chat)
	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "state-agent",
		"message":    "test message",
	})
	if s1 != 200 {
		t.Fatalf("message: expected status 200, got %d", s1)
	}

	// Filter by RESPONDED state
	tasks, status := env.GetJSONArray(t, "/api/tasks?state=RESPONDED&agent_name=state-agent")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(tasks) < 1 {
		t.Fatalf("expected at least 1 RESPONDED task, got %d", len(tasks))
	}

	// Filter by ERROR state (should be empty)
	errorTasks, status2 := env.GetJSONArray(t, "/api/tasks?state=ERROR&agent_name=state-agent")
	if status2 != 200 {
		t.Fatalf("expected status 200 for ERROR filter, got %d", status2)
	}

	if len(errorTasks) != 0 {
		t.Errorf("expected 0 ERROR tasks, got %d", len(errorTasks))
	}
}

// TestDiscovery_TaskTrace sends a message and verifies trace has send+response entries.
func TestDiscovery_TaskTrace(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "trace-agent")

	// Send message
	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "trace-agent",
		"message":    "trace test",
	})
	if s1 != 200 {
		t.Fatalf("message: expected status 200, got %d", s1)
	}

	// Get tasks to find the task ID
	tasks, status := env.GetJSONArray(t, "/api/tasks?agent_name=trace-agent")
	if status != 200 {
		t.Fatalf("get tasks: expected status 200, got %d", status)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least 1 task")
	}

	// Get task ID
	task, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatal("task is not a map")
	}
	taskID, ok := task["local_task_id"].(string)
	if !ok || taskID == "" {
		t.Fatal("missing local_task_id")
	}

	// Get trace
	trace, traceStatus := env.GetRaw(t, "/api/tasks/"+taskID+"/trace", nil)
	if traceStatus != 200 {
		t.Fatalf("get trace: expected status 200, got %d", traceStatus)
	}

	// Verify trace contains send and response events
	if !strings.Contains(trace, "send") {
		t.Errorf("expected trace to contain 'send' event, got: %s", trace)
	}
	if !strings.Contains(trace, "response") {
		t.Errorf("expected trace to contain 'response' event, got: %s", trace)
	}
}

// TestDiscovery_NonexistentTaskTrace verifies "(no trace yet)" for nonexistent task.
func TestDiscovery_NonexistentTaskTrace(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	trace, status := env.GetRaw(t, "/api/tasks/nonexistent-id/trace", nil)
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if !strings.Contains(trace, "(no trace yet)") {
		t.Errorf("expected '(no trace yet)', got: %s", trace)
	}
}

// TestDiscovery_GetErrorAgentDetail verifies that GET /api/agents/{name} returns
// the error_message field for an agent with status=error.
func TestDiscovery_GetErrorAgentDetail(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// Directly insert an error agent into the DB with an error_message,
	// without registering to in-memory connections.
	env.RegisterAgentWithDBAndError(t, "failing-agent", "http://localhost:9999", "test", "error",
		"connection refused after 3 retries")

	// Query the detail endpoint
	agent, status := env.GetJSON(t, "/api/agents/failing-agent")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if agent["name"] != "failing-agent" {
		t.Errorf("expected name=failing-agent, got %v", agent["name"])
	}
	if agent["status"] != "error" {
		t.Errorf("expected status=error, got %v", agent["status"])
	}
	if agent["error_message"] != "connection refused after 3 retries" {
		t.Errorf("expected error_message='connection refused after 3 retries', got %v", agent["error_message"])
	}
}

// TestDiscovery_ErrorAgentDisplay verifies an error-status agent appears in list.
func TestDiscovery_ErrorAgentDisplay(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// Create an agent with error status directly in DB
	env.RegisterAgentWithDB(t, "error-agent", "http://localhost:9999", "test", "error")

	agents, status := env.GetJSONArray(t, "/api/agents")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	found := false
	for _, a := range agents {
		agent, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if agent["name"] == "error-agent" {
			found = true
			if agent["status"] != "error" {
				t.Errorf("expected status=error, got %v", agent["status"])
			}
			break
		}
	}
	if !found {
		t.Error("error-agent not found in agent list")
	}
}

// TestDiscovery_DisconnectedAgentDisplay verifies a disconnected agent shows status=disconnected.
func TestDiscovery_DisconnectedAgentDisplay(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// Register a connected agent first
	env.StartFakeAgent(t, "echo", "disc-agent")

	// Verify it's connected
	agents1, _ := env.GetJSONArray(t, "/api/agents")
	for _, a := range agents1 {
		agent, _ := a.(map[string]interface{})
		if agent["name"] == "disc-agent" {
			if agent["status"] != "connected" {
				t.Errorf("expected initial status=connected, got %v", agent["status"])
			}
		}
	}

	// Disconnect the agent via DELETE
	_, delStatus := env.PostJSON(t, "/api/agents/disc-agent", nil)
	// DELETE requires a different method - use GetRaw
	// Actually, let's use DeleteAgent
	traceBody, _ := json.Marshal(nil)
	_, delStatus = env.PostRaw(t, "/api/agents/disc-agent", string(traceBody), "", map[string]string{})

	// The DELETE endpoint needs a proper DELETE request, which PostRaw doesn't do.
	// Instead, let's directly disconnect via registry
	env.SvcCtx.Registry.Disconnect("disc-agent")

	// Verify status is disconnected
	agents2, status := env.GetJSONArray(t, "/api/agents")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	found := false
	for _, a := range agents2 {
		agent, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if agent["name"] == "disc-agent" {
			found = true
			if agent["status"] != "disconnected" {
				t.Errorf("expected status=disconnected, got %v", agent["status"])
			}
			break
		}
	}
	if !found {
		t.Error("disc-agent not found after disconnect")
	}

	_ = delStatus // suppress unused warning
}

// TestDiscovery_FilterTasksByStateFailed verifies filtering by FAILED state returns empty.
func TestDiscovery_FilterTasksByStateFailed(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "fail-state-agent")

	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "fail-state-agent",
		"message":    "test",
	})
	if s1 != 200 {
		t.Fatalf("message: expected 200, got %d", s1)
	}

	tasks, status := env.GetJSONArray(t, "/api/tasks?state=FAILED&agent_name=fail-state-agent")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 FAILED tasks, got %d", len(tasks))
	}
}

// TestDiscovery_CapabilitiesFullStructure verifies all fields in capabilities response.
func TestDiscovery_CapabilitiesFullStructure(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	caps, status := env.GetJSON(t, "/api/capabilities")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}

	// Verify required top-level fields
	requiredFields := []string{"name", "description", "version", "capabilities", "tools"}
	for _, field := range requiredFields {
		if caps[field] == nil {
			t.Errorf("capabilities missing field '%s'", field)
		}
	}

	// Verify capabilities.streaming is bool
	capMap, ok := caps["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities.capabilities is not a map")
	}
	if _, ok := capMap["streaming"].(bool); !ok {
		t.Errorf("expected streaming to be bool, got %T", capMap["streaming"])
	}

	// Verify tools is non-empty array with items that have name and description
	tools, ok := caps["tools"].([]interface{})
	if !ok {
		t.Fatal("tools is not an array")
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tools")
	}
	for i, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			t.Errorf("tool[%d] is not an object", i)
			continue
		}
		if toolMap["name"] == nil {
			t.Errorf("tool[%d] missing 'name' field", i)
		}
		if toolMap["description"] == nil {
			t.Errorf("tool[%d] missing 'description' field", i)
		}
	}
}

// TestDiscovery_A2AVersionHeaderValue verifies the A2A-Version header injected by proxy is "1.0".
func TestDiscovery_A2AVersionHeaderValue(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// We need to register a record_headers agent, but the built-in StartFakeAgent
	// doesn't support that directly for proxy use. Instead, we register and use
	// the MessageBus tool call path.
	// Actually, let's use a direct approach: register a custom agent server that
	// records headers and returns them.
	var headersAgentURL string
	var capturedHeaders map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
			card := map[string]interface{}{
				"name":        "headers-agent",
				"description": "Records headers",
				"version":     "1.0.0",
				"url":         headersAgentURL,
				"capabilities": map[string]bool{"streaming": true},
				"skills":      []map[string]string{{"id": "record_headers", "name": "record_headers", "description": "records headers"}},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}
		if r.Method == http.MethodPost {
			// Record headers
			capturedHeaders = make(map[string][]string)
			for k, v := range r.Header {
				capturedHeaders[k] = v
			}
			// Return SSE echo response
			fakeHandleEcho(w, r)
			return
		}
	}))
	defer server.Close()
	headersAgentURL = server.URL

	env.RegisterAgent(t, server.URL)

	// Send message through the chat endpoint (which goes through proxy)
	_, status := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "headers-agent",
		"message":    "check version header",
	})
	if status != 200 {
		t.Fatalf("chat: expected 200, got %d", status)
	}

	if capturedHeaders == nil {
		t.Fatal("agent did not receive any request")
	}

	a2aVersion := capturedHeaders["A2a-Version"]
	if len(a2aVersion) == 0 {
		// Try lowercase
		a2aVersion = capturedHeaders["a2a-version"]
	}
	if len(a2aVersion) == 0 {
		t.Errorf("expected A2A-Version header, got headers: %v", capturedHeaders)
	} else if a2aVersion[0] != "1.0" {
		t.Errorf("expected A2A-Version=1.0, got %s", a2aVersion[0])
	}
}

// TestDiscovery_TaskFields verifies tasks returned by the API have expected fields.
func TestDiscovery_TaskFields(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "task-fields-agent")

	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "task-fields-agent",
		"message":    "test",
	})
	if s1 != 200 {
		t.Fatalf("message: expected 200, got %d", s1)
	}

	tasks, status := env.GetJSONArray(t, "/api/tasks?agent_name=task-fields-agent")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least 1 task")
	}

	task, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatal("task is not a map")
	}

	requiredFields := []string{"local_task_id", "agent_name", "state", "created_at"}
	for _, field := range requiredFields {
		if task[field] == nil {
			t.Errorf("task missing required field '%s'", field)
		}
	}
}

// TestDiscovery_ContextContinuation verifies context_id is persisted on tasks.
func TestDiscovery_ContextContinuationContextID(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "ctx-persist-agent")

	contextID := "persistent-ctx-abc"

	_, s1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "ctx-persist-agent",
		"message":    "msg with context",
		"context_id": contextID,
	})
	if s1 != 200 {
		t.Fatalf("message: expected 200, got %d", s1)
	}

	tasks, status := env.GetJSONArray(t, "/api/tasks?agent_name=ctx-persist-agent")
	if status != 200 {
		t.Fatalf("get tasks: expected 200, got %d", status)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least 1 task")
	}

	task, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatal("task is not a map")
	}

	if task["context_id"] != contextID {
		t.Errorf("expected context_id=%s, got %v", contextID, task["context_id"])
	}
}
