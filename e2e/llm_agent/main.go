package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

	// Parse A2A JSON-RPC request
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	// Extract user text from params.message.parts or params.messages
	var userText string
	params, ok := req["params"].(map[string]interface{})
	if ok {
		// Try params.messages (plural) first
		if messages, ok := params["messages"].([]interface{}); ok && len(messages) > 0 {
			lastMsg, _ := messages[len(messages)-1].(map[string]interface{})
			if parts, ok := lastMsg["parts"].([]interface{}); ok {
				for _, p := range parts {
					pm, ok := p.(map[string]interface{})
					if ok && pm["kind"] == "text" {
						if t, ok := pm["text"].(string); ok {
							userText += t
						}
					}
				}
			}
		}
		// Also try params.message (singular)
		if userText == "" {
			if message, ok := params["message"].(map[string]interface{}); ok {
				if parts, ok := message["parts"].([]interface{}); ok {
					for _, p := range parts {
						pm, ok := p.(map[string]interface{})
						if ok && pm["kind"] == "text" {
							if t, ok := pm["text"].(string); ok {
								userText += t
							}
						}
					}
				}
			}
		}
	}

	// Read LLM config from env vars
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://token-plan-cn.xiaomimimo.com/v1"
	}
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		apiKey = "tp-cvg1l8m7kmdevg8xts5j9qt34i9tpvxdj5o57q668da3g5om"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "mimo-v2.5-pro"
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send working status
	writeSSE(w, "status_update", map[string]string{"status": "WORKING"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Build chat completion request
	chatReq := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": userText},
		},
		"stream": true,
	}
	body, _ := json.Marshal(chatReq)

	llmReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeSSE(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{{"kind": "text", "text": "LLM request build error: " + err.Error()}},
		})
		writeSSE(w, "status_update", map[string]string{"status": "COMPLETED"})
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
		writeSSE(w, "status_update", map[string]string{"status": "COMPLETED"})
		return
	}
	defer resp.Body.Close()

	// Parse SSE from LLM and forward
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
		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := delta["content"].(string)
		if ok && content != "" {
			responseText.WriteString(content)
			// Forward as artifact_update
			writeSSE(w, "artifact_update", map[string]interface{}{
				"artifact": map[string]interface{}{
					"parts": []map[string]string{{"kind": "text", "text": content}},
				},
			})
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}

	// Send final message
	writeSSE(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{{"kind": "text", "text": responseText.String()}},
	})
	writeSSE(w, "status_update", map[string]string{"status": "COMPLETED"})
}

func writeSSE(w http.ResponseWriter, eventType string, data interface{}) {
	payload := map[string]interface{}{"type": eventType, "data": data}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	w.Write([]byte("data: " + string(jsonData) + "\n\n"))
}
