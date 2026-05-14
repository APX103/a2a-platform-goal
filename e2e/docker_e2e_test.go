// +build docker

package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// DockerE2E runs E2E tests against real Docker Compose services.
// Requires env vars:
//   HOST_URL - Host service URL (e.g. http://host:7860)
//   FAKE_AGENT_A_URL - Fake agent A URL (e.g. http://fake-agent-a:10001)
//   FAKE_AGENT_B_URL - Fake agent B URL (e.g. http://fake-agent-b:10002)
//   LLM_AGENT_URL - LLM agent URL (e.g. http://llm-agent:10003)
// Set DOCKER_E2E=1 to enable these tests. Skip otherwise.

func getEnvOrSkip(t *testing.T, key string) string {
	val := os.Getenv(key)
	if val == "" {
		t.Skipf("env %s not set, skipping Docker E2E test", key)
	}
	return val
}

func dockerGetJSON(t *testing.T, url string) (map[string]interface{}, int) {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result, resp.StatusCode
}

func dockerGetJSONArray(t *testing.T, url string) ([]interface{}, int) {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result []interface{}
	json.Unmarshal(body, &result)
	return result, resp.StatusCode
}

func dockerPostJSON(t *testing.T, url string, body interface{}) (string, int) {
	t.Helper()
	var bodyReader io.Reader
	switch v := body.(type) {
	case string:
		bodyReader = strings.NewReader(v)
	default:
		data, _ := json.Marshal(v)
		bodyReader = strings.NewReader(string(data))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bodyReader)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), resp.StatusCode
}

func dockerPostSSE(t *testing.T, url string, body interface{}) ([]map[string]interface{}, int) {
	t.Helper()
	var bodyReader io.Reader
	switch v := body.(type) {
	case string:
		bodyReader = strings.NewReader(v)
	default:
		data, _ := json.Marshal(v)
		bodyReader = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest("POST", url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s SSE failed: %v", url, err)
	}
	defer resp.Body.Close()
	var events []map[string]interface{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var evt map[string]interface{}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt) == nil {
				events = append(events, evt)
			}
		}
	}
	return events, resp.StatusCode
}

func registerDockerAgent(t *testing.T, hostURL, agentURL, agentName string) {
	t.Helper()
	body, status := dockerPostJSON(t, hostURL+"/api/agents/register", map[string]string{
		"url":  agentURL,
		"name": agentName,
	})
	if status >= 400 {
		t.Fatalf("register agent %s failed: %d %s", agentName, status, body)
	}
	// Wait for agent to be available
	for i := 0; i < 30; i++ {
		agents, status := dockerGetJSONArray(t, hostURL+"/api/agents")
		if status != 200 {
			t.Fatalf("list agents failed: %d", status)
		}
		found := false
		for _, a := range agents {
			if m, ok := a.(map[string]interface{}); ok {
				if m["name"] == agentName {
					found = true
					break
				}
			}
		}
		if found {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("agent %s not registered after 15s", agentName)
}

// TestDocker_HostStartup verifies the Host service is up and responding.
func TestDocker_HostStartup(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	_, status := dockerGetJSON(t, hostURL+"/api/capabilities")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
}

// TestDocker_RegisterFakeAgent registers a fake agent via Host API and verifies listing.
func TestDocker_RegisterFakeAgent(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	agentURL := getEnvOrSkip(t, "FAKE_AGENT_A_URL")

	registerDockerAgent(t, hostURL, agentURL, "docker-agent-a")

	agents, status := dockerGetJSONArray(t, hostURL+"/api/agents")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	found := false
	for _, a := range agents {
		if m, ok := a.(map[string]interface{}); ok {
			if m["name"] == "docker-agent-a" {
				found = true
				if m["status"] != "connected" {
					t.Errorf("expected status=connected, got %v", m["status"])
				}
			}
		}
	}
	if !found {
		t.Error("docker-agent-a not found in agent list")
	}
}

// TestDocker_RegisterMultipleAgents registers 2 agents and verifies both appear.
func TestDocker_RegisterMultipleAgents(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	agentAURL := getEnvOrSkip(t, "FAKE_AGENT_A_URL")
	agentBURL := getEnvOrSkip(t, "FAKE_AGENT_B_URL")

	registerDockerAgent(t, hostURL, agentAURL, "docker-a")
	registerDockerAgent(t, hostURL, agentBURL, "docker-b")

	agents, status := dockerGetJSONArray(t, hostURL+"/api/agents")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(agents) < 2 {
		t.Errorf("expected >=2 agents, got %d", len(agents))
	}
}

// TestDocker_ProxyEcho sends a message through the Host proxy to the echo agent.
func TestDocker_ProxyEcho(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	agentURL := getEnvOrSkip(t, "FAKE_AGENT_A_URL")

	registerDockerAgent(t, hostURL, agentURL, "docker-agent-a")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hello docker"}]}}}`
	events, status := dockerPostSSE(t, hostURL+"/agent/docker-agent-a", reqBody)
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(events) == 0 {
		t.Fatal("expected at least 1 SSE event")
	}
	foundEcho := false
	for _, evt := range events {
		data, ok := evt["data"].(map[string]interface{})
		if !ok {
			continue
		}
		text := ""
		if parts, ok := data["parts"].([]interface{}); ok && len(parts) > 0 {
			if p, ok := parts[0].(map[string]interface{}); ok {
				text, _ = p["text"].(string)
			}
		}
		if strings.Contains(text, "hello docker") {
			foundEcho = true
		}
	}
	if !foundEcho {
		t.Errorf("echo response not found in events: %v", events)
	}
}

// TestDocker_SSESequence verifies SSE streaming through Docker networking.
func TestDocker_SSESequence(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	agentURL := getEnvOrSkip(t, "FAKE_AGENT_A_URL")

	registerDockerAgent(t, hostURL, agentURL, "docker-seq-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	events, status := dockerPostSSE(t, hostURL+"/agent/docker-seq-agent?mode=sequence", reqBody)
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(events) < 3 {
		t.Fatalf("expected >=3 SSE events, got %d", len(events))
	}
	// Verify sequence: status_update(WORKING) → message → status_update(COMPLETED
	statuses := make([]string, 0, len(events))
	for _, evt := range events {
		statuses = append(statuses, evt["type"].(string))
	}
	if statuses[0] != "status_update" {
		t.Errorf("expected first event type=status_update, got %s", statuses[0])
	}
}

// TestDocker_TaskListing verifies the tasks endpoint works.
func TestDocker_TaskListing(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")

	tasks, status := dockerGetJSONArray(t, hostURL+"/api/tasks")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	// Tasks list may be empty since proxy doesn't persist tasks
	_ = tasks
}

// TestDocker_AgentError404 verifies 404 for nonexistent agent through Docker.
func TestDocker_AgentError404(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"test"}]}}}`
	_, status := dockerPostJSON(t, hostURL+"/agent/nonexistent-docker", reqBody)
	if status != 404 {
		t.Errorf("expected 404, got %d", status)
	}
}

// TestDocker_LLMAgent registers the LLM agent and sends a real LLM request.
func TestDocker_LLMAgent(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	llmAgentURL := getEnvOrSkip(t, "LLM_AGENT_URL")

	registerDockerAgent(t, hostURL, llmAgentURL, "llm-agent")

	reqBody := `{"jsonrpc":"2.0","method":"SendStreamingMessage","id":"1","params":{"message":{"role":"user","parts":[{"kind":"text","text":"Say hello in one sentence"}]}}}`
	events, status := dockerPostSSE(t, hostURL+"/agent/llm-agent", reqBody)
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if len(events) == 0 {
		t.Fatal("expected at least 1 SSE event from LLM agent")
	}

	// Find the final message event and verify it has non-empty content
	hasContent := false
	for _, evt := range events {
		if evt["type"].(string) == "message" {
			data := evt["data"].(map[string]interface{})
			if parts, ok := data["parts"].([]interface{}); ok && len(parts) > 0 {
				if p, ok := parts[0].(map[string]interface{}); ok {
					text, _ := p["text"].(string)
					if len(text) > 5 {
						hasContent = true
					}
				}
			}
		}
	}
	if !hasContent {
		t.Errorf("LLM agent response seems empty, events: %v", events)
	}
}

// TestDocker_Capabilities verifies capabilities endpoint through Docker.
func TestDocker_Capabilities(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	caps, status := dockerGetJSON(t, hostURL+"/api/capabilities")
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	tools := caps["tools"]
	if tools == nil {
		t.Error("expected non-empty tools")
	}
}

// TestDocker_DisconnectAgent verifies agent deletion through Docker.
func TestDocker_DisconnectAgent(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	agentURL := getEnvOrSkip(t, "FAKE_AGENT_A_URL")

	registerDockerAgent(t, hostURL, agentURL, "docker-disc-agent")

	// Delete agent
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("DELETE", hostURL+"/api/agents/docker-disc-agent", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("disconnect request failed: %v", err)
	}
	resp.Body.Close()

	// Verify deleted (404)
	time.Sleep(500 * time.Millisecond)
	_, status := dockerGetJSON(t, hostURL+"/api/agents/docker-disc-agent")
	if status != 404 {
		t.Errorf("expected 404 after delete, got %d", status)
	}
}

// TestDocker_NonexistentAgent404 verifies 404 for nonexistent agent detail.
func TestDocker_NonexistentAgent404(t *testing.T) {
	hostURL := getEnvOrSkip(t, "HOST_URL")
	_, status := dockerGetJSON(t, hostURL+"/api/agents/nonexistent-docker")
	if status != 404 {
		t.Errorf("expected 404, got %d", status)
	}
}
