package messagebus

import (
	"encoding/json"
	"fmt"
)

// HostTools defines the tools exposed by the host platform to connected agents.
var HostTools = []map[string]interface{}{
	{
		"name":        "list_agents",
		"description": "List all agents registered on the A2A platform",
		"parameters": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		"name":        "send_to_agent",
		"description": "Send a message to another agent on the platform",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the target agent",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "The message content to send",
				},
			},
			"required": []string{"agent_name", "message"},
		},
	},
	{
		"name":        "get_agent_info",
		"description": "Get detailed information about a specific agent",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the agent to query",
				},
			},
			"required": []string{"agent_name"},
		},
	},
}

// HandleToolCall dispatches a tool call to the appropriate handler and returns the result.
func HandleToolCall(toolName string, params map[string]interface{}, hostURL string) string {
	switch toolName {
	case "list_agents":
		return handleListAgents(hostURL)
	case "send_to_agent":
		return handleSendToAgent(params, hostURL)
	case "get_agent_info":
		return handleGetAgentInfo(params, hostURL)
	default:
		return fmt.Sprintf(`{"error": "unknown tool: %s"}`, toolName)
	}
}

func handleListAgents(hostURL string) string {
	result := map[string]interface{}{
		"agents_url": fmt.Sprintf("%s/api/agents", hostURL),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func handleSendToAgent(params map[string]interface{}, hostURL string) string {
	agentName, _ := params["agent_name"].(string)
	message, _ := params["message"].(string)

	if agentName == "" || message == "" {
		data, _ := json.Marshal(map[string]string{"error": "agent_name and message are required"})
		return string(data)
	}

	result := map[string]interface{}{
		"status":     "queued",
		"agent_name": agentName,
		"message":    message,
		"api":        fmt.Sprintf("%s/api/agents/%s/send", hostURL, agentName),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func handleGetAgentInfo(params map[string]interface{}, hostURL string) string {
	agentName, _ := params["agent_name"].(string)

	if agentName == "" {
		data, _ := json.Marshal(map[string]string{"error": "agent_name is required"})
		return string(data)
	}

	result := map[string]interface{}{
		"agent_name": agentName,
		"info_url":   fmt.Sprintf("%s/api/agents/%s", hostURL, agentName),
	}
	data, _ := json.Marshal(result)
	return string(data)
}
