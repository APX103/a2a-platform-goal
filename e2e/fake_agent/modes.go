package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// parseMode extracts the mode from the query string or returns "echo" as default.
func parseMode(r *http.Request) string {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		return "echo"
	}
	return mode
}

// parseName extracts the name from the query string or returns "fake-agent" as default.
func parseName(r *http.Request) string {
	name := r.URL.Query().Get("name")
	if name == "" {
		return "fake-agent"
	}
	return name
}

// handleAgentRequest handles POST requests with mode-specific behavior.
func handleAgentRequest(w http.ResponseWriter, r *http.Request, mode string) {
	switch {
	case mode == "echo":
		handleEcho(w, r)
	case mode == "sequence":
		handleSequence(w, r)
	case mode == "artifact":
		handleArtifact(w, r)
	case mode == "json_response":
		handleJSONResponse(w, r)
	case mode == "record_headers":
		handleRecordHeaders(w, r)
	case mode == "record_url":
		handleRecordURL(w, r)
	case mode == "full_events":
		handleFullEvents(w, r)
	case mode == "error":
		handleError(w, r)
	case strings.HasPrefix(mode, "tool_call"):
		handleToolCall(w, r, mode)
	case mode == "disconnect_mid_stream":
		handleDisconnectMidStream(w, r)
	case mode == "empty_response":
		handleEmptyResponse(w, r)
	case mode == "delayed_start":
		handleEcho(w, r)
	default:
		handleEcho(w, r)
	}
}

// handleEcho returns the input text in a single SSE message event.
func handleEcho(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// If body can't be parsed, echo an error message
		writeSSEEvent(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{
				{"kind": "text", "text": "error: could not parse request body"},
			},
		})
		return
	}

	// Extract text from the JSON-RPC params
	text := ""
	if params, ok := body["params"].(map[string]interface{}); ok {
		if messages, ok := params["messages"].([]interface{}); ok && len(messages) > 0 {
			if lastMsg, ok := messages[len(messages)-1].(map[string]interface{}); ok {
				if parts, ok := lastMsg["parts"].([]interface{}); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]interface{}); ok {
						if t, ok := part["text"].(string); ok {
							text = t
						}
					}
				}
			}
		}
	}

	writeSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": text},
		},
	})
}

// handleSequence returns ordered SSE events: status_update(WORKING) -> message("processed") -> status_update(COMPLETED).
func handleSequence(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "WORKING",
	})

	writeSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "processed"},
		},
	})

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "COMPLETED",
	})
}

// handleArtifact returns: status_update(WORKING) -> artifact_update(text) -> status_update(COMPLETED).
func handleArtifact(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "WORKING",
	})

	writeSSEEvent(w, "artifact_update", map[string]interface{}{
		"artifact": map[string]interface{}{
			"name": "generated_artifact",
			"parts": []map[string]string{
				{"kind": "text", "text": "generated artifact"},
			},
		},
	})

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "COMPLETED",
	})
}

// handleJSONResponse returns plain JSON (non-SSE) if the request does NOT accept text/event-stream.
func handleJSONResponse(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") {
		// If SSE is requested, fall through to echo behavior via SSE
		handleEcho(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"result": "ok",
	})
}

// handleRecordHeaders records all received request headers and returns them as JSON.
// Used to verify A2A-Version injection and hop-by-hop header stripping.
func handleRecordHeaders(w http.ResponseWriter, r *http.Request) {
	headers := make(map[string][]string)
	for key, vals := range r.Header {
		headers[key] = vals
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"headers": headers,
	})
}

// handleRecordURL records the full request URL path and query and returns them as JSON.
// Used to verify path/query forwarding.
func handleRecordURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"path":  r.URL.Path,
		"query": r.URL.RawQuery,
	})
}

// handleFullEvents returns complete event sequence: task -> status_update -> artifact_update -> message.
func handleFullEvents(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	writeSSEEvent(w, "task", map[string]interface{}{
		"id":   "test-task-001",
		"state": "SUBMITTED",
	})

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "WORKING",
	})

	writeSSEEvent(w, "artifact_update", map[string]interface{}{
		"artifact": map[string]interface{}{
			"name": "full_artifact",
			"parts": []map[string]string{
				{"kind": "text", "text": "artifact from full events"},
			},
		},
	})

	writeSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "done"},
		},
	})
}

// handleError returns HTTP 500 error.
func handleError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "intentional test error",
	})
}

// handleToolCall receives a tool_name parameter (via query or mode like "tool_call:list_agents").
// Makes an HTTP call back to the Host to execute the tool, then embeds the result in the SSE response.
func handleToolCall(w http.ResponseWriter, r *http.Request, mode string) {
	setSSEHeaders(w)

	// Extract tool name from mode (e.g. "tool_call:list_agents" -> "list_agents")
	toolName := ""
	if idx := strings.Index(mode, ":"); idx >= 0 {
		toolName = mode[idx+1:]
	}
	if toolName == "" {
		toolName = r.URL.Query().Get("tool_name")
	}

	// Get host URL from header or query
	hostURL := r.Header.Get("X-Host-URL")
	if hostURL == "" {
		hostURL = r.URL.Query().Get("host_url")
	}

	var toolResult string
	if hostURL != "" && toolName != "" {
		// Make an HTTP call back to the host
		targetURL := fmt.Sprintf("%s/tool/%s", hostURL, toolName)
		resp, err := http.Get(targetURL)
		if err != nil {
			toolResult = fmt.Sprintf("error calling host: %v", err)
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			toolResult = string(body)
		}
	} else {
		toolResult = fmt.Sprintf("no host_url or tool_name provided (tool=%s, host=%s)", toolName, hostURL)
	}

	writeSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": fmt.Sprintf("tool_call result: %s", toolResult)},
		},
	})
}

// handleDisconnectMidStream sends 1 SSE event then closes the connection.
func handleDisconnectMidStream(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	writeSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "disconnecting..."},
		},
	})

	// Connection will close when this handler returns
}

// handleEmptyResponse returns only status_update events (WORKING -> COMPLETED), no message or artifact.
func handleEmptyResponse(w http.ResponseWriter, r *http.Request) {
	setSSEHeaders(w)

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "WORKING",
	})

	writeSSEEvent(w, "status_update", map[string]string{
		"state": "COMPLETED",
	})
}
