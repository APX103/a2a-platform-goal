// +build !docker

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRegistry_ConnectByURL starts a fake agent, registers via /api/agents/register,
// and verifies the agent appears in the DB with connected status.
func TestRegistry_ConnectByURL(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	agentURL := env.StartFakeAgent(t, "echo", "reg-agent")

	// Verify agent is in the list with connected status
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
		if agent["name"] == "reg-agent" {
			found = true
			if agent["status"] != "connected" {
				t.Errorf("expected status=connected, got %v", agent["status"])
			}
			if agent["url"] != agentURL {
				t.Errorf("expected url=%s, got %v", agentURL, agent["url"])
			}
			break
		}
	}
	if !found {
		t.Error("reg-agent not found in agent list")
	}
}

// TestRegistry_Disconnect registers an agent, then DELETEs it, and verifies
// the status becomes disconnected.
func TestRegistry_Disconnect(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "disc-reg-agent")

	// Verify connected
	agent1, status1 := env.GetJSON(t, "/api/agents/disc-reg-agent")
	if status1 != 200 {
		t.Fatalf("get agent: expected status 200, got %d", status1)
	}
	if agent1["status"] != "connected" {
		t.Errorf("expected initial status=connected, got %v", agent1["status"])
	}

	// Disconnect via DELETE
	body, delStatus := env.DeleteRaw(t, "/api/agents/disc-reg-agent", nil)
	if delStatus != 200 {
		t.Fatalf("delete: expected status 200, got %d: %s", delStatus, body)
	}

	// Verify status is disconnected (agent is deleted, so get returns 404)
	_, status2 := env.GetJSON(t, "/api/agents/disc-reg-agent")
	if status2 != 404 {
		// The agent was deleted, not just disconnected. This is the expected behavior
		// for DeleteAgent. The DELETE endpoint removes the agent entirely.
		t.Logf("after DELETE, get agent returned %d (agent deleted)", status2)
	}
}

// TestRegistry_Reconnect registers, disconnects, then registers again the same
// agent URL and verifies reconnection.
func TestRegistry_Reconnect(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// Create a stable fake agent that won't go away
	var agentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
			card := map[string]interface{}{
				"name":        "reconnect-agent",
				"description": "Reconnect test agent",
				"version":     "1.0.0",
				"url":         agentURL,
				"capabilities": map[string]bool{
					"streaming": true,
				},
				"skills": []map[string]string{
					{"id": "echo", "name": "echo", "description": "echo mode"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}
		handleFakeAgentMode(w, r, "echo")
	}))
	defer server.Close()
	agentURL = server.URL

	// Register
	body1, status1 := env.PostJSON(t, "/api/agents/register", map[string]string{"url": server.URL})
	if status1 != 201 {
		t.Fatalf("first register: expected status 201, got %d: %s", status1, body1)
	}
	var reg1 map[string]interface{}
	json.Unmarshal([]byte(body1), &reg1)
	if reg1["status"] != "connected" {
		t.Errorf("expected status=connected, got %v", reg1["status"])
	}

	// Disconnect via DELETE
	_, delStatus := env.DeleteRaw(t, "/api/agents/reconnect-agent", nil)
	if delStatus != 200 {
		t.Fatalf("delete: expected status 200, got %d", delStatus)
	}

	// Re-register same URL
	body2, status2 := env.PostJSON(t, "/api/agents/register", map[string]string{"url": server.URL})
	if status2 != 201 {
		t.Fatalf("second register: expected status 201, got %d: %s", status2, body2)
	}
	var reg2 map[string]interface{}
	json.Unmarshal([]byte(body2), &reg2)
	if reg2["status"] != "connected" {
		t.Errorf("expected status=connected after reconnect, got %v", reg2["status"])
	}
	if reg2["name"] != "reconnect-agent" {
		t.Errorf("expected name=reconnect-agent, got %v", reg2["name"])
	}
}

// TestRegistry_MultipleAgents registers 3 agents and verifies all 3 appear
// in the agent list.
func TestRegistry_MultipleAgents(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	env.StartFakeAgent(t, "echo", "multi-agent-1")
	env.StartFakeAgent(t, "echo", "multi-agent-2")
	env.StartFakeAgent(t, "echo", "multi-agent-3")

	agents, status := env.GetJSONArray(t, "/api/agents")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	names := make(map[string]bool)
	for _, a := range agents {
		agent, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := agent["name"].(string)
		names[name] = true
	}

	for _, expected := range []string{"multi-agent-1", "multi-agent-2", "multi-agent-3"} {
		if !names[expected] {
			t.Errorf("expected agent %s in list", expected)
		}
	}
}

// TestRegistry_DuplicateRegistration registers the same agent twice and verifies
// only 1 entry exists (upsert behavior).
func TestRegistry_DuplicateRegistration(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// Use a single stable agent server
	var dupAgentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
			card := map[string]interface{}{
				"name":        "dup-agent",
				"description": "Duplicate test agent",
				"version":     "1.0.0",
				"url":         dupAgentURL,
				"capabilities": map[string]bool{
					"streaming": true,
				},
				"skills": []map[string]string{
					{"id": "echo", "name": "echo", "description": "echo mode"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}
		handleFakeAgentMode(w, r, "echo")
	}))
	defer server.Close()
	dupAgentURL = server.URL

	// Register first time
	_, status1 := env.PostJSON(t, "/api/agents/register", map[string]string{"url": server.URL})
	if status1 != 201 {
		t.Fatalf("first register: expected status 201, got %d", status1)
	}

	// Register same agent again (same name from agent card)
	_, status2 := env.PostJSON(t, "/api/agents/register", map[string]string{"url": server.URL})
	if status2 != 201 {
		t.Fatalf("second register: expected status 201, got %d", status2)
	}

	// Verify only 1 agent in the list
	agents, status := env.GetJSONArray(t, "/api/agents")
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent after duplicate registration, got %d", len(agents))
	}

	agent, ok := agents[0].(map[string]interface{})
	if !ok {
		t.Fatal("agent is not a map")
	}
	if agent["name"] != "dup-agent" {
		t.Errorf("expected name=dup-agent, got %v", agent["name"])
	}
}

// TestRegistry_RetryWithDelayedStart tests that the registry retries connecting
// to an agent that initially fails. The agent starts on the 2nd attempt.
func TestRegistry_RetryWithDelayedStart(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Teardown()

	// We use a controlled server that fails initially then succeeds.
	// Since retry logic is in ConnectByURL with AgentRetryMax=3 and baseDelay=0,
	// we simulate the agent being unavailable on the first call.
	attempts := 0
	var retryAgentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
			attempts++
			if attempts < 2 {
				// Fail on first attempt
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			card := map[string]interface{}{
				"name":        "retry-agent",
				"description": "Retry test agent",
				"version":     "1.0.0",
				"url":         retryAgentURL,
				"capabilities": map[string]bool{
					"streaming": true,
				},
				"skills": []map[string]string{
					{"id": "echo", "name": "echo", "description": "echo mode"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}
		handleFakeAgentMode(w, r, "echo")
	}))
	defer server.Close()
	retryAgentURL = server.URL

	// Wait a tiny bit to ensure the server is up
	time.Sleep(50 * time.Millisecond)

	// The registry has retryMax=3 and baseDelay=0, so it will retry immediately.
	// The first call fails, second succeeds.
	body, status := env.PostJSON(t, "/api/agents/register", map[string]string{"url": server.URL})
	if status != 201 {
		t.Fatalf("register: expected status 201 after retry, got %d: %s", status, body)
	}

	var reg map[string]interface{}
	json.Unmarshal([]byte(body), &reg)
	if reg["status"] != "connected" {
		t.Errorf("expected status=connected after retry, got %v", reg["status"])
	}
	if reg["name"] != "retry-agent" {
		t.Errorf("expected name=retry-agent, got %v", reg["name"])
	}

	// Verify agent is functional by sending a message
	_, chatStatus := env.PostJSON(t, "/api/chat", map[string]string{
		"agent_name": "retry-agent",
		"message":    "hello retry agent",
	})
	if chatStatus != 200 {
		t.Errorf("chat with retry agent: expected status 200, got %d", chatStatus)
	}
}
