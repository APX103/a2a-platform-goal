// +build !docker

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"a2a-platform/internal/messagebus"
)

// TestToolCall_ListAgents verifies that list_agents tool call returns agent list info.
func TestToolCall_ListAgents(t *testing.T) {
	result := messagebus.HandleToolCall("list_agents", nil, "http://localhost:8080")

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["agents_url"] == nil {
		t.Error("expected agents_url in result")
	}
	if !strings.Contains(parsed["agents_url"].(string), "/api/agents") {
		t.Errorf("expected agents_url to contain /api/agents, got %v", parsed["agents_url"])
	}
}

// TestToolCall_SendToAgent verifies that send_to_agent tool call returns proper response.
func TestToolCall_SendToAgent(t *testing.T) {
	params := map[string]interface{}{
		"agent_name": "echo-agent",
		"message":    "hello from agent",
	}
	result := messagebus.HandleToolCall("send_to_agent", params, "http://localhost:8080")

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["status"] != "queued" {
		t.Errorf("expected status=queued, got %v", parsed["status"])
	}
	if parsed["agent_name"] != "echo-agent" {
		t.Errorf("expected agent_name=echo-agent, got %v", parsed["agent_name"])
	}
	if parsed["message"] != "hello from agent" {
		t.Errorf("expected message='hello from agent', got %v", parsed["message"])
	}
	if parsed["api"] == nil {
		t.Error("expected api field in result")
	}
}

// TestToolCall_GetAgentInfo verifies that get_agent_info tool call returns agent info.
func TestToolCall_GetAgentInfo(t *testing.T) {
	params := map[string]interface{}{
		"agent_name": "test-agent",
	}
	result := messagebus.HandleToolCall("get_agent_info", params, "http://localhost:8080")

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["agent_name"] != "test-agent" {
		t.Errorf("expected agent_name=test-agent, got %v", parsed["agent_name"])
	}
	if parsed["info_url"] == nil {
		t.Error("expected info_url in result")
	}
}

// TestToolCall_UnknownTool verifies that calling an unknown tool returns an error.
func TestToolCall_UnknownTool(t *testing.T) {
	result := messagebus.HandleToolCall("unknown_tool", nil, "http://localhost:8080")

	if !strings.Contains(result, "unknown tool") {
		t.Errorf("expected error about unknown tool, got: %s", result)
	}
	if !strings.Contains(result, "unknown_tool") {
		t.Errorf("expected error to mention tool name, got: %s", result)
	}
}

// TestToolCall_SendToNonexistent verifies error when sending to nonexistent agent.
func TestToolCall_SendToNonexistent(t *testing.T) {
	params := map[string]interface{}{
		"agent_name": "nonexistent-agent",
		"message":    "hello",
	}
	result := messagebus.HandleToolCall("send_to_agent", params, "http://localhost:8080")

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// The tool call just queues the message - it doesn't validate the agent exists
	// So it should return the queued status with the nonexistent agent name
	if parsed["status"] != "queued" {
		t.Errorf("expected status=queued, got %v", parsed["status"])
	}
	if parsed["agent_name"] != "nonexistent-agent" {
		t.Errorf("expected agent_name=nonexistent-agent, got %v", parsed["agent_name"])
	}
}

// TestToolCall_NoHostConfigured verifies that calling HandleToolCall with empty hostURL
// still works (tools construct URLs from hostURL).
func TestToolCall_NoHostConfigured(t *testing.T) {
	result := messagebus.HandleToolCall("list_agents", nil, "")

	// With empty hostURL, the tool should still return a result (with empty URL)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// list_agents should return agents_url with empty host
	if parsed["agents_url"] == nil {
		t.Error("expected agents_url in result even with empty hostURL")
	}
}
