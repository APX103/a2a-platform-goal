package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var port = flag.Int("port", 10003, "port to listen on")

func main() {
	flag.Parse()
	p := *port
	pStr := os.Getenv("PORT")
	if pStr != "" {
		fmt.Sscanf(pStr, "%d", &p)
	}
	if p == 0 {
		p = 10003
	}

	http.HandleFunc("/.well-known/agent.json", handleAgentCard)
	http.HandleFunc("/", handleMessage)

	fmt.Printf("LLM agent starting on :%d\n", p)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", p), nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "llm-agent",
		"description": "LLM-backed agent using mimo-v2.5-pro",
		"version":     "1.0.0",
		"url":         "http://" + r.Host,
		"capabilities": map[string]bool{
			"streaming": true,
		},
		"skills": []map[string]string{
			{"id": "chat", "name": "Chat", "description": "Chat with LLM"},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		handleAgentCard(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	userText := extractUserText(req)

	baseURL := envOr("LLM_BASE_URL", "https://token-plan-cn.xiaomimimo.com/v1")
	apiKey := os.Getenv("LLM_API_KEY")
	model := envOr("LLM_MODEL", "mimo-v2.5-pro")
	hostURL := os.Getenv("HOST_URL")

	if apiKey == "" {
		setSSEHeaders(w)
		writeSSE(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{{"kind": "text", "text": "Error: LLM_API_KEY environment variable is not set"}},
		})
		writeSSE(w, "status_update", map[string]string{"status": "COMPLETED"})
		return
	}

	setSSEHeaders(w)
	writeSSE(w, "status_update", map[string]string{"status": "WORKING"})
	flush(w)

	if hostURL != "" {
		handleToolAwareMessage(w, userText, hostURL, baseURL, apiKey, model)
	} else {
		handleStreamingMessage(w, userText, baseURL, apiKey, model)
	}

	writeSSE(w, "status_update", map[string]string{"status": "COMPLETED"})
}

// handleToolAwareMessage uses non-streaming LLM calls with tool definitions.
// Supports multi-round tool calling until the LLM gives a final answer.
func handleToolAwareMessage(w http.ResponseWriter, userText, hostURL, baseURL, apiKey, model string) {
	messages := []map[string]interface{}{
		{"role": "user", "content": userText},
	}

	tools := buildPlatformTools(hostURL)

	for round := 0; round < 5; round++ {
		llmResp, err := callLLMNonStreaming(baseURL, apiKey, model, messages, tools)
		if err != nil {
			writeSSE(w, "artifact_update", map[string]interface{}{
				"artifact": map[string]interface{}{
					"parts": []map[string]string{{"kind": "text", "text": fmt.Sprintf("LLM error: %v", err)}},
				},
			})
			break
		}

		if len(llmResp.ToolCalls) > 0 {
			// Add assistant message with tool_calls to conversation
			var toolCalls []map[string]interface{}
			for _, tc := range llmResp.ToolCalls {
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
			}
			asstMsg := map[string]interface{}{
				"role":       "assistant",
				"content":    llmResp.Content,
				"tool_calls": toolCalls,
			}
			if llmResp.ReasoningContent != "" {
				asstMsg["reasoning_content"] = llmResp.ReasoningContent
			}
			messages = append(messages, asstMsg)

			for _, tc := range llmResp.ToolCalls {
				toolDesc := fmt.Sprintf("Using tool: %s(%s)", tc.Function.Name, tc.Function.Arguments)
				writeSSE(w, "artifact_update", map[string]interface{}{
					"artifact": map[string]interface{}{
						"parts": []map[string]string{{"kind": "text", "text": toolDesc}},
					},
				})
				flush(w)

				result, toolErr := executeTool(hostURL, tc.Function.Name, tc.Function.Arguments)
				if toolErr != nil {
					result = fmt.Sprintf("Error: %v", toolErr)
				}

				writeSSE(w, "artifact_update", map[string]interface{}{
					"artifact": map[string]interface{}{
						"parts": []map[string]string{{"kind": "text", "text": fmt.Sprintf("Tool result: %s", result)}},
					},
				})
				flush(w)

				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id":  tc.ID,
					"content":      result,
					"name":         tc.Function.Name,
				})
			}
			continue
		}

		// No tool calls - final answer
		if llmResp.Content != "" {
			writeSSE(w, "message", map[string]interface{}{
				"role": "agent",
				"parts": []map[string]string{{"kind": "text", "text": llmResp.Content}},
			})
			flush(w)
		}
		return
	}

	// Max rounds exceeded
	writeSSE(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{{"kind": "text", "text": "Could not complete the request after multiple tool calls."}},
	})
	flush(w)
}

// handleStreamingMessage does direct streaming to the LLM (backward compatible, no tools).
func handleStreamingMessage(w http.ResponseWriter, userText, baseURL, apiKey, model string) {
	chatReq := map[string]interface{}{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": userText}},
		"stream":   true,
	}
	body, _ := json.Marshal(chatReq)

	llmReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeSSE(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{{"kind": "text", "text": "LLM request error: " + err.Error()}},
		})
		return
	}
	llmReq.Header.Set("Content-Type", "application/json")
	llmReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(llmReq)
	if err != nil {
		writeSSE(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{{"kind": "text", "text": "LLM error: " + err.Error()}},
		})
		return
	}
	defer resp.Body.Close()

	var responseText strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		choices, _ := chunk["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]interface{})
		if choice == nil {
			continue
		}
		delta, _ := choice["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}
		content, _ := delta["content"].(string)
		if content != "" {
			responseText.WriteString(content)
			writeSSE(w, "artifact_update", map[string]interface{}{
				"artifact": map[string]interface{}{
					"parts": []map[string]string{{"kind": "text", "text": content}},
				},
			})
			flush(w)
		}
	}

	writeSSE(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{{"kind": "text", "text": responseText.String()}},
	})
	flush(w)
}

// --- Tool definitions and execution ---

func buildPlatformTools(hostURL string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "list_agents",
				"description": "List all agents registered on the A2A platform. Returns an array of agent objects with name, description, status, and skills.",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "get_agent_info",
				"description": "Get detailed information about a specific agent registered on the platform, including its name, description, version, status, and skills.",
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
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "send_to_agent",
				"description": fmt.Sprintf("Send a message to another agent through the A2A platform proxy. The platform will deliver your message and return the agent's response. Use this to communicate with other agents."),
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"agent_name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the target agent to send the message to",
						},
						"message": map[string]interface{}{
							"type":        "string",
							"description": "The message content to send to the target agent",
						},
					},
					"required": []string{"agent_name", "message"},
				},
			},
		},
	}
}

func executeTool(hostURL, toolName, argumentsJSON string) (string, error) {
	switch toolName {
	case "list_agents":
		return executeListAgents(hostURL)
	case "get_agent_info":
		return executeGetAgentInfo(hostURL, argumentsJSON)
	case "send_to_agent":
		return executeSendToAgent(hostURL, argumentsJSON)
	default:
		return fmt.Sprintf("Unknown tool: %s", toolName), nil
	}
}

func executeListAgents(hostURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(hostURL + "/api/agents")
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var agents []map[string]interface{}
	if err := json.Unmarshal(body, &agents); err != nil {
		return string(body), nil
	}

	if len(agents) == 0 {
		return "No agents registered on the platform.", nil
	}

	result := fmt.Sprintf("Found %d agents on the platform:\n", len(agents))
	for _, a := range agents {
		name, _ := a["name"].(string)
		desc, _ := a["description"].(string)
		status, _ := a["status"].(string)
		result += fmt.Sprintf("- %s: %s (status: %s)\n", name, desc, status)
	}
	return result, nil
}

func executeGetAgentInfo(hostURL, argumentsJSON string) (string, error) {
	var params struct {
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(hostURL + "/api/agents/" + params.AgentName)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(body), nil
}

func executeSendToAgent(hostURL, argumentsJSON string) (string, error) {
	var params struct {
		AgentName string `json:"agent_name"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if params.AgentName == "" {
		return "Error: agent_name is required", nil
	}
	if params.Message == "" {
		return "Error: message is required", nil
	}

	chatBody, _ := json.Marshal(map[string]string{
		"agent_name": params.AgentName,
		"message":    params.Message,
	})

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(hostURL+"/api/chat", "application/json", strings.NewReader(string(chatBody)))
	if err != nil {
		return "", fmt.Errorf("send message to %s failed: %w", params.AgentName, err)
	}
	defer resp.Body.Close()

	// Parse SSE response, find the "response" event
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var evt map[string]interface{}
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}
		evtType, _ := evt["type"].(string)
		if evtType == "response" {
			if d, ok := evt["data"].(map[string]interface{}); ok {
				text, _ := d["text"].(string)
				return text, nil
			}
		}
	}

	return "(agent did not return a response)", nil
}

// --- LLM API calls ---

type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type llmNonStreamingResponse struct {
	Content         string         `json:"content"`
	ReasoningContent string        `json:"reasoning_content"`
	ToolCalls       []llmToolCall `json:"tool_calls"`
}

func callLLMNonStreaming(baseURL, apiKey, model string, messages []map[string]interface{}, tools []map[string]interface{}) (*llmNonStreamingResponse, error) {
	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content         string         `json:"content"`
				ReasoningContent string         `json:"reasoning_content"`
				ToolCalls       []llmToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return &llmNonStreamingResponse{}, nil
	}

	return &llmNonStreamingResponse{
		Content:         result.Choices[0].Message.Content,
		ReasoningContent: result.Choices[0].Message.ReasoningContent,
		ToolCalls:       result.Choices[0].Message.ToolCalls,
	}, nil
}

// --- Helpers ---

func extractUserText(req map[string]interface{}) string {
	var userText string
	params, ok := req["params"].(map[string]interface{})
	if !ok {
		return ""
	}
	if messages, ok := params["messages"].([]interface{}); ok && len(messages) > 0 {
		lastMsg, _ := messages[len(messages)-1].(map[string]interface{})
		if parts, ok := lastMsg["parts"].([]interface{}); ok {
			for _, p := range parts {
				pm, _ := p.(map[string]interface{})
				if pm["kind"] == "text" {
					if t, ok := pm["text"].(string); ok {
						userText += t
					}
				}
			}
		}
	}
	if userText == "" {
		if message, ok := params["message"].(map[string]interface{}); ok {
			if parts, ok := message["parts"].([]interface{}); ok {
				for _, p := range parts {
					pm, _ := p.(map[string]interface{})
					if pm["kind"] == "text" {
						if t, ok := pm["text"].(string); ok {
							userText += t
						}
					}
				}
			}
		}
	}
	return userText
}

func writeSSE(w http.ResponseWriter, eventType string, data interface{}) {
	payload := map[string]interface{}{"type": eventType, "data": data}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	w.Write([]byte("data: " + string(jsonData) + "\n\n"))
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
