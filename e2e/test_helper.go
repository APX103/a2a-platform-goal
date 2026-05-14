package e2e

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"

	_ "modernc.org/sqlite"
)

// TestEnv holds the test Host server and its dependencies.
type TestEnv struct {
	Host      *httptest.Server
	HostURL   string
	DB        *sql.DB
	SvcCtx    *svc.ServiceContext
	FakeAgent *exec.Cmd
}

// fakeAgentBinary returns the path to the pre-built fake agent binary.
func fakeAgentBinary() string {
	// Look for the binary next to the test executable or at a known location.
	// For testing, we use the existing binary at the project root.
	cwd, _ := os.Getwd()
	// Try project root
	bin := cwd + "/../fake_agent"
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	return "e2e/fake_agent"
}

// SetupTestEnv creates a Host server backed by an in-memory SQLite database.
// It registers all the real handler routes on a plain http.ServeMux.
func SetupTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("ping in-memory db: %v", err)
	}

	// Initialize the database schema
	if err := repository.InitDB(db); err != nil {
		t.Fatalf("init db schema: %v", err)
	}

	c := &config.Config{
		DataSource:          ":memory:",
		AgentRetryMax:       3,
		AgentRetryBaseDelay: 0, // no retry delay for tests
	}

	svcCtx := svc.NewServiceContextWithDB(db, c)

	mux := http.NewServeMux()

	// Register all API routes with pathvar support
	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.GetAgents(svcCtx).ServeHTTP(w, r)
		case http.MethodPost:
			handler.AddAgent(svcCtx).ServeHTTP(w, r)
		}
	})
	mux.HandleFunc("/api/agents/register", handler.RegisterAgent(svcCtx))
	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		// Extract :name from path /api/agents/{name} or /api/agents/{name}/...
		remaining := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		if idx := strings.Index(remaining, "/"); idx >= 0 {
			remaining = remaining[:idx]
		}
		if remaining != "" {
			r = pathvar.WithVars(r, map[string]string{"name": remaining})
		}
		switch r.Method {
		case http.MethodGet:
			handler.GetAgent(svcCtx).ServeHTTP(w, r)
		case http.MethodDelete:
			handler.DeleteAgent(svcCtx).ServeHTTP(w, r)
		}
	})
	mux.HandleFunc("/api/capabilities", handler.GetCapabilities(svcCtx))
	mux.HandleFunc("/api/tasks", handler.GetTasks(svcCtx))
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		// Extract :id from path /api/tasks/{id}/trace
		remaining := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		if idx := strings.Index(remaining, "/"); idx >= 0 {
			taskID := remaining[:idx]
			r = pathvar.WithVars(r, map[string]string{"id": taskID})
		}
		handler.GetTaskTrace(svcCtx).ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/chat", handler.Chat(svcCtx))
	mux.HandleFunc("/agent/", func(w http.ResponseWriter, r *http.Request) {
		// Extract :name from path /agent/{name}
		remaining := strings.TrimPrefix(r.URL.Path, "/agent/")
		if idx := strings.Index(remaining, "/"); idx >= 0 {
			remaining = remaining[:idx]
		}
		if remaining != "" {
			r = pathvar.WithVars(r, map[string]string{"name": remaining})
		}
		handler.ProxyAgent(svcCtx).ServeHTTP(w, r)
	})

	server := httptest.NewServer(mux)

	return &TestEnv{
		Host:    server,
		HostURL: server.URL,
		DB:      db,
		SvcCtx:  svcCtx,
	}
}

// Teardown cleans up the test environment.
func (e *TestEnv) Teardown() {
	if e.FakeAgent != nil && e.FakeAgent.Process != nil {
		e.FakeAgent.Process.Kill()
		e.FakeAgent.Wait()
	}
	if e.Host != nil {
		e.Host.Close()
	}
	if e.DB != nil {
		e.DB.Close()
	}
}

// StartFakeAgent starts the fake agent binary on a random port.
// It returns the URL of the running fake agent.
func (e *TestEnv) StartFakeAgent(t *testing.T, mode string, name string) string {
	t.Helper()
	if name == "" {
		name = "fake-agent"
	}

	bin := fakeAgentBinary()
	cmd := exec.Command(bin, "-name", name)
	cmd.Env = append(os.Environ(), "PORT=0")

	// Capture output for debugging on failure
	var output strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake agent: %v (output: %s)", err, output.String())
	}

	e.FakeAgent = cmd

	// Wait briefly for startup
	time.Sleep(100 * time.Millisecond)

	// Read the port from stdout or find a way to get it
	// The fake agent prints: Fake agent 'name' starting on :PORT
	// We need to extract the port. Since PORT=0 won't work well with exec,
	// let's use a different approach: use httptest.NewServer for the fake agent.

	// Kill this process and use httptest instead
	cmd.Process.Kill()
	cmd.Wait()
	e.FakeAgent = nil

	// Use httptest approach - start a real HTTP server that mimics fake agent behavior
	return e.startFakeAgentHTTPTest(t, mode, name)
}

// startFakeAgentHTTPTest starts a fake agent as an httptest.Server.
// This approach is more reliable than running a subprocess.
func (e *TestEnv) startFakeAgentHTTPTest(t *testing.T, mode string, name string) string {
	t.Helper()
	if name == "" {
		name = "fake-agent"
	}

	var agentURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Agent card endpoint
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
			card := map[string]interface{}{
				"name":        name,
				"description": "Fake agent for testing (" + mode + ")",
				"version":     "1.0.0",
				"url":         agentURL,
				"capabilities": map[string]bool{
					"streaming": true,
				},
				"skills": []map[string]string{
					{
						"id":          mode,
						"name":        mode,
						"description": "Test mode: " + mode,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}

		if r.Method == http.MethodPost {
			handleFakeAgentMode(w, r, mode)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	agentURL = server.URL

	// Store the server for cleanup (we'll close it in teardown)
	// For simplicity, we close it when Teardown is called via the test
	t.Cleanup(func() { server.Close() })

	// Register with Host using the base URL (ConnectByURL will append /.well-known/agent.json)
	e.RegisterAgent(t, agentURL)

	return agentURL
}

// RegisterAgent registers an agent URL with the Host.
func (e *TestEnv) RegisterAgent(t *testing.T, agentURL string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"url": agentURL})
	resp, err := http.Post(e.HostURL+"/api/agents/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("register agent request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("register agent failed: %d %s", resp.StatusCode, string(respBody))
	}
}

// RegisterAgentWithDB directly inserts an agent into the database.
func (e *TestEnv) RegisterAgentWithDB(t *testing.T, name, url, agentType, status string) {
	t.Helper()
	_ = e.SvcCtx.Registry.UpsertAgent(&model.AgentRecord{
		Name:      name,
		URL:       url,
		AgentType: agentType,
	})
	if status != "" {
		_ = e.SvcCtx.AgentRepo.UpdateStatus(name, model.AgentStatus(status), "")
	}
}

// GetJSON performs a GET request and returns the parsed JSON and status code.
func (e *TestEnv) GetJSON(t *testing.T, path string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := http.Get(e.HostURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result, resp.StatusCode
}

// GetJSONArray performs a GET request and returns a parsed JSON array and status code.
func (e *TestEnv) GetJSONArray(t *testing.T, path string) ([]interface{}, int) {
	t.Helper()
	resp, err := http.Get(e.HostURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result []interface{}
	json.Unmarshal(body, &result)
	return result, resp.StatusCode
}

// PostJSON performs a POST request with a JSON body and returns the response body and status code.
func (e *TestEnv) PostJSON(t *testing.T, path string, body interface{}) (string, int) {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(e.HostURL+path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), resp.StatusCode
}

// PostSSE performs a POST request with SSE accept header and parses SSE events.
func (e *TestEnv) PostSSE(t *testing.T, path string, body interface{}) ([]map[string]interface{}, int) {
	t.Helper()
	return e.PostSSEWithHeaders(t, path, body, map[string]string{
		"Accept": "text/event-stream",
	})
}

// PostSSEWithHeaders performs a POST request with custom headers and parses SSE events.
func (e *TestEnv) PostSSEWithHeaders(t *testing.T, path string, body interface{}, headers map[string]string) ([]map[string]interface{}, int) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", e.HostURL+path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST SSE %s: %v", path, err)
	}
	defer resp.Body.Close()

	events := ParseSSEEvents(resp.Body)
	return events, resp.StatusCode
}

// PostRaw performs a POST request with a raw body and returns response body and status code.
func (e *TestEnv) PostRaw(t *testing.T, path string, body string, contentType string, extraHeaders map[string]string) (string, int) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.HostURL+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST raw %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), resp.StatusCode
}

// GetRaw performs a GET request with custom headers and returns response body and status code.
func (e *TestEnv) GetRaw(t *testing.T, path string, headers map[string]string) (string, int) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.HostURL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET raw %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), resp.StatusCode
}

// ParseSSEEvents reads SSE events from a reader and returns them as maps.
func ParseSSEEvents(r io.Reader) []map[string]interface{} {
	var events []map[string]interface{}
	scanner := bufio.NewScanner(r)
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

// --- Fake agent mode handlers (inlined for E2E tests since we can't import package main) ---

func handleFakeAgentMode(w http.ResponseWriter, r *http.Request, mode string) {
	switch {
	case mode == "echo":
		fakeHandleEcho(w, r)
	case mode == "sequence":
		fakeHandleSequence(w, r)
	case mode == "artifact":
		fakeHandleArtifact(w, r)
	case mode == "json_response":
		fakeHandleJSONResponse(w, r)
	case mode == "record_headers":
		fakeHandleRecordHeaders(w, r)
	case mode == "record_url":
		fakeHandleRecordURL(w, r)
	case mode == "full_events":
		fakeHandleFullEvents(w, r)
	case mode == "error":
		fakeHandleError(w, r)
	case strings.HasPrefix(mode, "tool_call"):
		fakeHandleToolCall(w, r, mode)
	case mode == "disconnect_mid_stream":
		fakeHandleDisconnectMidStream(w, r)
	case mode == "empty_response":
		fakeHandleEmptyResponse(w, r)
	default:
		fakeHandleEcho(w, r)
	}
}

func fakeSetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func fakeWriteSSEEvent(w http.ResponseWriter, eventType string, data interface{}) {
	payload := map[string]interface{}{
		"type": eventType,
		"data": data,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	w.Write([]byte("data: " + string(jsonData) + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func fakeHandleEcho(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fakeWriteSSEEvent(w, "message", map[string]interface{}{
			"role": "agent",
			"parts": []map[string]string{
				{"kind": "text", "text": "error: could not parse request body"},
			},
		})
		return
	}

	text := ""
	if params, ok := body["params"].(map[string]interface{}); ok {
		// Try params.messages (plural) format first
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
		// Also try params.message (singular) format (used by A2A Client)
		if text == "" {
			if msg, ok := params["message"].(map[string]interface{}); ok {
				if parts, ok := msg["parts"].([]interface{}); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]interface{}); ok {
						if t, ok := part["text"].(string); ok {
							text = t
						}
					}
				}
			}
		}
	}

	fakeWriteSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": text},
		},
	})
}

func fakeHandleSequence(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "WORKING"})
	fakeWriteSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "processed"},
		},
	})
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "COMPLETED"})
}

func fakeHandleArtifact(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "WORKING"})
	fakeWriteSSEEvent(w, "artifact_update", map[string]interface{}{
		"artifact": map[string]interface{}{
			"name": "generated_artifact",
			"parts": []map[string]string{
				{"kind": "text", "text": "generated artifact"},
			},
		},
	})
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "COMPLETED"})
}

func fakeHandleJSONResponse(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") {
		fakeHandleEcho(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

func fakeHandleRecordHeaders(w http.ResponseWriter, r *http.Request) {
	headers := make(map[string][]string)
	for key, vals := range r.Header {
		headers[key] = vals
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"headers": headers})
}

func fakeHandleRecordURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"path":  r.URL.Path,
		"query": r.URL.RawQuery,
	})
}

func fakeHandleFullEvents(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)
	fakeWriteSSEEvent(w, "task", map[string]interface{}{
		"id":    "test-task-001",
		"state": "SUBMITTED",
	})
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "WORKING"})
	fakeWriteSSEEvent(w, "artifact_update", map[string]interface{}{
		"artifact": map[string]interface{}{
			"name": "full_artifact",
			"parts": []map[string]string{
				{"kind": "text", "text": "artifact from full events"},
			},
		},
	})
	fakeWriteSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "done"},
		},
	})
}

func fakeHandleError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": "intentional test error"})
}

func fakeHandleToolCall(w http.ResponseWriter, r *http.Request, mode string) {
	fakeSetSSEHeaders(w)

	toolName := ""
	if idx := strings.Index(mode, ":"); idx >= 0 {
		toolName = mode[idx+1:]
	}
	if toolName == "" {
		toolName = r.URL.Query().Get("tool_name")
	}

	hostURL := r.Header.Get("X-Host-URL")
	if hostURL == "" {
		hostURL = r.URL.Query().Get("host_url")
	}

	var toolResult string
	if hostURL != "" && toolName != "" {
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

	fakeWriteSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": fmt.Sprintf("tool_call result: %s", toolResult)},
		},
	})
}

func fakeHandleDisconnectMidStream(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)
	fakeWriteSSEEvent(w, "message", map[string]interface{}{
		"role": "agent",
		"parts": []map[string]string{
			{"kind": "text", "text": "disconnecting..."},
		},
	})
}

func fakeHandleEmptyResponse(w http.ResponseWriter, r *http.Request) {
	fakeSetSSEHeaders(w)
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "WORKING"})
	fakeWriteSSEEvent(w, "status_update", map[string]string{"state": "COMPLETED"})
}
