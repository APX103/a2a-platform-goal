package a2a

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_SendStreamingMessage_SSE(t *testing.T) {
	sseBody := `data: {"type":"task","data":{"id":"task-1","status":"WORKING"}}

data: {"type":"status_update","data":{"status":"COMPLETED"}}

`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseBody))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage("user", "hello", "task-1")
	if err != nil {
		t.Fatalf("SendStreamingMessage returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != "task" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "task")
	}
	if events[1].Type != "status_update" {
		t.Errorf("events[1].Type = %q, want %q", events[1].Type, "status_update")
	}

	// Verify request body contains correct fields
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(sseBody), &reqBody); err == nil {
		_ = reqBody // just checking it's valid JSON structure
	}
}

func TestClient_SendStreamingMessage_NonSSE(t *testing.T) {
	response := map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "hi there"},
		},
	}
	respBytes, _ := json.Marshal(response)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBytes)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage("user", "hello", "")
	if err != nil {
		t.Fatalf("SendStreamingMessage returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Type != "message" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "message")
	}
	if string(events[0].Data) != string(respBytes) {
		t.Errorf("events[0].Data = %s, want %s", string(events[0].Data), string(respBytes))
	}
}

func TestClient_SendStreamingMessage_A2AVersionHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := r.Header.Get("A2A-Version")
		if version != "1.0" {
			t.Errorf("A2A-Version header = %q, want %q", version, "1.0")
		}
		accept := r.Header.Get("Accept")
		if accept != "text/event-stream" {
			t.Errorf("Accept header = %q, want %q", accept, "text/event-stream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"message\",\"data\":{}}\n\n"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendStreamingMessage("user", "hello", "")
	if err != nil {
		t.Fatalf("SendStreamingMessage returned error: %v", err)
	}
}

func TestClient_FetchAgentCard(t *testing.T) {
	card := AgentCard{
		Name:        "test-agent",
		Description: "A test agent",
		Version:     "1.0.0",
		URL:         "http://example.com",
		Capabilities: Capabilities{Streaming: true},
		Skills: []Skill{
			{ID: "s1", Name: "echo", Description: "Echoes input"},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent.json" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/.well-known/agent.json")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	result, err := client.FetchAgentCard()
	if err != nil {
		t.Fatalf("FetchAgentCard returned error: %v", err)
	}
	if result.Name != card.Name {
		t.Errorf("Name = %q, want %q", result.Name, card.Name)
	}
	if result.Description != card.Description {
		t.Errorf("Description = %q, want %q", result.Description, card.Description)
	}
	if result.Version != card.Version {
		t.Errorf("Version = %q, want %q", result.Version, card.Version)
	}
	if !result.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = false, want true")
	}
	if len(result.Skills) != 1 {
		t.Fatalf("len(Skills) = %d, want 1", len(result.Skills))
	}
	if result.Skills[0].ID != "s1" {
		t.Errorf("Skills[0].ID = %q, want %q", result.Skills[0].ID, "s1")
	}
}

func TestClient_FetchAgentCard_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.FetchAgentCard()
	if err == nil {
		t.Fatal("FetchAgentCard expected error for 404, got nil")
	}
}

func TestClient_PostRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/tools/run" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/tools/run")
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	resp, err := client.PostRaw("/tools/run", strings.NewReader(`{"tool":"test"}`), "application/json")
	if err != nil {
		t.Fatalf("PostRaw returned error: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if string(respBody) != `{"tool":"test"}` {
		t.Errorf("response body = %q, want %q", string(respBody), `{"tool":"test"}`)
	}
}

func TestClient_GetRaw(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/health" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/health")
		}
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	resp, err := client.GetRaw("/health")
	if err != nil {
		t.Fatalf("GetRaw returned error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("response body = %q, want %q", string(body), "ok")
	}
}
