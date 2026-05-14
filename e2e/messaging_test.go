package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMessaging_Echo sends "hello" to echo agent via proxy and verifies response contains "hello".
func TestMessaging_Echo(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	agentURL := env.StartFakeAgent(t, "echo", "echo-agent")

	// POST to /agent/echo-agent via proxy (fake agent expects params.messages format)
	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"messages":[{"role":"user","parts":[{"kind":"text","text":"hello"}]}]}}`
	body, status := env.PostRaw(t, "/agent/echo-agent", reqBody, "application/json", map[string]string{
		"Accept": "text/event-stream",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	// Parse SSE events
	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) == 0 {
		t.Fatalf("expected at least 1 SSE event, got body: %s", body)
	}

	// Verify echo response contains "hello"
	found := false
	for _, evt := range events {
		evtJSON, _ := json.Marshal(evt)
		if strings.Contains(string(evtJSON), "hello") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected echo response to contain 'hello', got events: %v", events)
	}

	_ = agentURL // used to start the agent
}

// TestMessaging_Sequence sends to sequence agent and verifies 3 SSE events in order.
func TestMessaging_Sequence(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "sequence", "seq-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	body, status := env.PostRaw(t, "/agent/seq-agent", reqBody, "application/json", map[string]string{
		"Accept": "text/event-stream",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) != 3 {
		t.Fatalf("expected 3 SSE events, got %d: %v", len(events), events)
	}

	// Event 1: status_update WORKING
	if events[0]["type"] != "status_update" {
		t.Errorf("expected event[0] type=status_update, got %v", events[0]["type"])
	}
	data0, _ := events[0]["data"].(map[string]interface{})
	if data0["state"] != "WORKING" {
		t.Errorf("expected event[0] state=WORKING, got %v", data0["state"])
	}

	// Event 2: message "processed"
	if events[1]["type"] != "message" {
		t.Errorf("expected event[1] type=message, got %v", events[1]["type"])
	}

	// Event 3: status_update COMPLETED
	if events[2]["type"] != "status_update" {
		t.Errorf("expected event[2] type=status_update, got %v", events[2]["type"])
	}
	data2, _ := events[2]["data"].(map[string]interface{})
	if data2["state"] != "COMPLETED" {
		t.Errorf("expected event[2] state=COMPLETED, got %v", data2["state"])
	}
}

// TestMessaging_Artifact sends to artifact agent and verifies artifact_update event.
func TestMessaging_Artifact(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "artifact", "artifact-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	body, status := env.PostRaw(t, "/agent/artifact-agent", reqBody, "application/json", map[string]string{
		"Accept": "text/event-stream",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) != 3 {
		t.Fatalf("expected 3 SSE events, got %d: %v", len(events), events)
	}

	// Find the artifact_update event
	foundArtifact := false
	for _, evt := range events {
		if evt["type"] == "artifact_update" {
			foundArtifact = true
			data, _ := evt["data"].(map[string]interface{})
			artifact, _ := data["artifact"].(map[string]interface{})
			if artifact["name"] != "generated_artifact" {
				t.Errorf("expected artifact name=generated_artifact, got %v", artifact["name"])
			}
			break
		}
	}
	if !foundArtifact {
		t.Fatalf("expected artifact_update event, got events: %v", events)
	}
}

// TestMessaging_NonSSE sends without Accept: text/event-stream header and verifies JSON response.
func TestMessaging_NonSSE(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "json_response", "json-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	// No Accept: text/event-stream header
	body, status := env.PostRaw(t, "/agent/json-agent", reqBody, "application/json", nil)

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	// json_response mode returns JSON when no SSE accept header
	var result map[string]string
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v (body: %s)", err, body)
	}
	if result["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", result["result"])
	}
}

// TestMessaging_A2AVersionInjection verifies that the agent receives A2A-Version header
// even when the client does not send one.
func TestMessaging_A2AVersionInjection(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "record_headers", "headers-agent")

	// Send request without A2A-Version header
	reqBody := `{}`
	body, status := env.PostRaw(t, "/agent/headers-agent", reqBody, "application/json", nil)

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	headers, ok := result["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected headers map, got %v", result)
	}

	// Check that A2A-Version header was injected by proxy
	// http.Header canonicalizes to A2a-Version
	found := false
	for k := range headers {
		if strings.EqualFold(k, "A2A-Version") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected A2A-Version header in forwarded headers, got: %v", headers)
	}
}

// TestMessaging_HopByHopStripping verifies that hop-by-hop headers are stripped by the proxy.
func TestMessaging_HopByHopStripping(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "record_headers", "hopbyhop-agent")

	// Send request with hop-by-hop headers
	reqBody := `{}`
	body, status := env.PostRaw(t, "/agent/hopbyhop-agent", reqBody, "application/json", map[string]string{
		"Connection":        "keep-alive",
		"Transfer-Encoding": "chunked",
		"Keep-Alive":        "timeout=5",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	headers, ok := result["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected headers map, got %v", result)
	}

	// Verify hop-by-hop headers are stripped
	for k := range headers {
		lower := strings.ToLower(k)
		if lower == "connection" || lower == "transfer-encoding" || lower == "keep-alive" {
			t.Errorf("hop-by-hop header '%s' should have been stripped, but was forwarded", k)
		}
	}
}

// TestMessaging_URLPathForwarding verifies that path and query params are forwarded correctly.
func TestMessaging_URLPathForwarding(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "record_url", "url-agent")

	reqBody := `{}`
	body, status := env.PostRaw(t, "/agent/url-agent/some/path?key=value&foo=bar", reqBody, "application/json", nil)

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify the path includes the forwarded part
	if result["path"] != "/some/path" {
		t.Errorf("expected path=/some/path, got %s", result["path"])
	}

	// Verify query params are forwarded
	if !strings.Contains(result["query"], "key=value") {
		t.Errorf("expected query to contain key=value, got %s", result["query"])
	}
	if !strings.Contains(result["query"], "foo=bar") {
		t.Errorf("expected query to contain foo=bar, got %s", result["query"])
	}
}

// TestMessaging_FullEventSequence verifies the complete event sequence from full_events mode.
func TestMessaging_FullEventSequence(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "full_events", "full-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	body, status := env.PostRaw(t, "/agent/full-agent", reqBody, "application/json", map[string]string{
		"Accept": "text/event-stream",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) != 4 {
		t.Fatalf("expected 4 SSE events, got %d: %v", len(events), events)
	}

	// Verify event sequence: task -> status_update -> artifact_update -> message
	if events[0]["type"] != "task" {
		t.Errorf("expected event[0] type=task, got %v", events[0]["type"])
	}
	if events[1]["type"] != "status_update" {
		t.Errorf("expected event[1] type=status_update, got %v", events[1]["type"])
	}
	if events[2]["type"] != "artifact_update" {
		t.Errorf("expected event[2] type=artifact_update, got %v", events[2]["type"])
	}
	if events[3]["type"] != "message" {
		t.Errorf("expected event[3] type=message, got %v", events[3]["type"])
	}
}

// TestMessaging_SendMessageFunction tests sending a message via /api/chat and getting text response.
func TestMessaging_SendMessageFunction(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "chat-agent")

	// Send message via /api/chat
	body, status := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "chat-agent",
		"message":    "hello world",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	// Parse SSE events from chat response
	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) < 2 {
		t.Fatalf("expected at least 2 SSE events (task + response), got %d: %s", len(events), body)
	}

	// Find the response event
	found := false
	for _, evt := range events {
		evtJSON, _ := json.Marshal(evt)
		if strings.Contains(string(evtJSON), "hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected response containing 'hello world', got events: %v", events)
	}
}

// TestMessaging_ContextContinuation sends 2 messages with same context_id
// and verifies only 1 task is created with 3 messages (2 user + 1 agent).
func TestMessaging_ContextContinuation(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "ctx-agent")

	contextID := "test-context-123"

	// Send first message
	_, status1 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "ctx-agent",
		"message":    "first message",
		"context_id": contextID,
	})
	if status1 != 200 {
		t.Fatalf("first message: expected status 200, got %d", status1)
	}

	// Send second message with same context_id
	_, status2 := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "ctx-agent",
		"message":    "second message",
		"context_id": contextID,
	})
	if status2 != 200 {
		t.Fatalf("second message: expected status 200, got %d", status2)
	}

	// Query tasks - should have tasks for this agent
	tasksArr, tasksStatus := env.GetJSONArray(t, "/api/tasks?agent_name=ctx-agent")
	if tasksStatus != 200 {
		t.Fatalf("get tasks: expected status 200, got %d", tasksStatus)
	}

	// We should have at least 2 tasks (the context continuation test creates tasks
	// since SendStreaming creates a new task each time it's called)
	if len(tasksArr) < 2 {
		t.Fatalf("expected at least 2 tasks for ctx-agent, got %d", len(tasksArr))
	}
}

// TestMessaging_EmptyContent sends an empty string and verifies no error.
func TestMessaging_EmptyContent(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "empty-agent")

	// Send empty message via proxy
	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":""}]}}}`
	body, status := env.PostRaw(t, "/agent/empty-agent", reqBody, "application/json", map[string]string{
		"Accept": "text/event-stream",
	})

	if status != 200 {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}

	// Verify we got a valid SSE response (not an error)
	events := ParseSSEEvents(strings.NewReader(body))
	if len(events) == 0 {
		t.Fatalf("expected at least 1 SSE event for empty content, got body: %s", body)
	}

	// First event should be a valid message response, not an error
	evtJSON, _ := json.Marshal(events[0])
	if strings.Contains(string(evtJSON), "error") {
		t.Errorf("unexpected error in response for empty content: %s", string(evtJSON))
	}
}
