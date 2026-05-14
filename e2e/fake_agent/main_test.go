package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequest)
	return httptest.NewServer(mux)
}

// parseSSELines reads an SSE stream and returns the parsed "data:" payloads.
func parseSSELines(body io.Reader) []map[string]interface{} {
	var events []map[string]interface{}
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				events = append(events, event)
			}
		}
	}
	return events
}

func TestAgentCardEndpoint(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/.well-known/agent.json?mode=echo&name=test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
	}

	var card map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode agent card: %v", err)
	}
	if card["name"] != "test-agent" {
		t.Errorf("expected name=test-agent, got %v", card["name"])
	}
	desc, ok := card["description"].(string)
	if !ok || !strings.Contains(desc, "echo") {
		t.Errorf("expected description to contain 'echo', got %v", card["description"])
	}
	if card["version"] != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %v", card["version"])
	}
}

func TestAgentCardDefaultName(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/.well-known/agent.json?mode=sequence")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var card map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("failed to decode agent card: %v", err)
	}
	if card["name"] != "fake-agent" {
		t.Errorf("expected name=fake-agent, got %v", card["name"])
	}
}

func TestEchoMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{
		"jsonrpc": "2.0",
		"method": "tasks/send",
		"id": "1",
		"params": {
			"messages": [
				{
					"role": "user",
					"parts": [{"kind": "text", "text": "hello world"}]
				}
			]
		}
	}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=echo", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	events := parseSSELines(resp.Body)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	msg := events[0]
	if msg["type"] != "message" {
		t.Errorf("expected type=message, got %v", msg["type"])
	}

	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map")
	}
	if data["role"] != "agent" {
		t.Errorf("expected role=agent, got %v", data["role"])
	}

	parts, ok := data["parts"].([]interface{})
	if !ok {
		t.Fatalf("expected parts to be an array")
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	part, ok := parts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected part to be a map")
	}
	if part["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", part["text"])
	}
}

func TestSequenceMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=sequence", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	events := parseSSELines(resp.Body)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Event 1: status_update WORKING
	if events[0]["type"] != "status_update" {
		t.Errorf("expected event[0] type=status_update, got %v", events[0]["type"])
	}
	data0 := events[0]["data"].(map[string]interface{})
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
	data2 := events[2]["data"].(map[string]interface{})
	if data2["state"] != "COMPLETED" {
		t.Errorf("expected event[2] state=COMPLETED, got %v", data2["state"])
	}
}

func TestArtifactMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=artifact", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	events := parseSSELines(resp.Body)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Event 1: status_update WORKING
	if events[0]["type"] != "status_update" {
		t.Errorf("expected event[0] type=status_update, got %v", events[0]["type"])
	}

	// Event 2: artifact_update
	if events[1]["type"] != "artifact_update" {
		t.Errorf("expected event[1] type=artifact_update, got %v", events[1]["type"])
	}
	data1 := events[1]["data"].(map[string]interface{})
	artifact, ok := data1["artifact"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected artifact to be a map")
	}
	if artifact["name"] != "generated_artifact" {
		t.Errorf("expected artifact name=generated_artifact, got %v", artifact["name"])
	}

	// Event 3: status_update COMPLETED
	if events[2]["type"] != "status_update" {
		t.Errorf("expected event[2] type=status_update, got %v", events[2]["type"])
	}
}

func TestErrorMessage(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=error", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "intentional test error" {
		t.Errorf("expected error='intentional test error', got %v", errResp["error"])
	}
}

func TestJSONResponseMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	// Without SSE accept header, should return plain JSON
	req, _ := http.NewRequest("POST", server.URL+"/?mode=json_response", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", result["result"])
	}
}

func TestRecordHeadersMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/?mode=record_headers", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("X-Custom", "test-value")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	headers, ok := result["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected headers to be a map")
	}
	if _, found := headers["A2a-Version"]; !found {
		// Note: http.Header canonicalizes header names
		t.Errorf("expected A2A-Version in headers, got %v", headers)
	}
	if _, found := headers["X-Custom"]; !found {
		t.Errorf("expected X-Custom in headers, got %v", headers)
	}
}

func TestRecordURLMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/some/path?foo=bar&mode=record_url", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["path"] != "/some/path" {
		t.Errorf("expected path=/some/path, got %v", result["path"])
	}
	if !strings.Contains(result["query"], "foo=bar") {
		t.Errorf("expected query to contain foo=bar, got %v", result["query"])
	}
}

func TestEmptyResponseMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=empty_response", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	events := parseSSELines(resp.Body)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0]["type"] != "status_update" {
		t.Errorf("expected event[0] type=status_update, got %v", events[0]["type"])
	}
	data0 := events[0]["data"].(map[string]interface{})
	if data0["state"] != "WORKING" {
		t.Errorf("expected event[0] state=WORKING, got %v", data0["state"])
	}

	if events[1]["type"] != "status_update" {
		t.Errorf("expected event[1] type=status_update, got %v", events[1]["type"])
	}
	data1 := events[1]["data"].(map[string]interface{})
	if data1["state"] != "COMPLETED" {
		t.Errorf("expected event[1] state=COMPLETED, got %v", data1["state"])
	}
}

func TestDisconnectMidStreamMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=disconnect_mid_stream", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	events := parseSSELines(resp.Body)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0]["type"] != "message" {
		t.Errorf("expected type=message, got %v", events[0]["type"])
	}
}

func TestFullEventsMode(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	reqBody := `{"jsonrpc": "2.0", "method": "tasks/send", "id": "1", "params": {}}`

	req, _ := http.NewRequest("POST", server.URL+"/?mode=full_events", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	events := parseSSELines(resp.Body)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

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

func TestMethodNotAllowed(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	// PUT request should be rejected
	req, _ := http.NewRequest("PUT", server.URL+"/", strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}
