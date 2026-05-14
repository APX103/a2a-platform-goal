package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/registry"

	_ "modernc.org/sqlite"
)

// testServer creates an httptest.Server with in-memory SQLite and test doubles.
func testServer(t *testing.T) (*httptest.Server, *testServiceContext) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal("open db:", err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatal("init db:", err)
	}

	agentRepo := repository.NewAgentRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	tr := newTestTracer(traceRepo)
	reg := registry.NewWithRetry(agentRepo, 1, 0)
	bus := newTestMessageBus(taskRepo, messageRepo, tr)
	bus.setRegistry(func(name string) *testClient {
		return nil
	})

	svcCtx := &testServiceContext{
		db:          db,
		agentRepo:   agentRepo,
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
		traceRepo:   traceRepo,
		registry:    reg,
		messageBus:  bus,
		tracer:      tr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testGetAgents(svcCtx)(w, r)
		case http.MethodPost:
			testAddAgent(svcCtx)(w, r)
		}
	})
	mux.HandleFunc("/api/agents/register", testRegisterAgent(svcCtx))
	mux.HandleFunc("/api/capabilities", testGetCapabilities(svcCtx))
	mux.HandleFunc("/api/tasks", testGetTasks(svcCtx))
	mux.HandleFunc("/api/tasks/", testGetTaskTrace(svcCtx))
	mux.HandleFunc("/api/agents/", testGetAgent(svcCtx))
	mux.HandleFunc("/api/chat", testChat(svcCtx))

	ts := httptest.NewServer(mux)
	return ts, svcCtx
}

// --- Test doubles ---

type testServiceContext struct {
	db          *sql.DB
	agentRepo   repository.AgentRepository
	taskRepo    repository.TaskRepository
	messageRepo repository.MessageRepository
	traceRepo   repository.TraceRepository
	registry    *registry.Registry
	messageBus  *testMessageBus
	tracer      *testTracer
}

type testTracer struct {
	events []string
}

func newTestTracer(repo repository.TraceRepository) *testTracer {
	return &testTracer{}
}

func (t *testTracer) record(event string) {
	t.events = append(t.events, event)
}

func (t *testTracer) getDebugTrace(taskID string) string {
	if len(t.events) == 0 {
		return "(no trace yet)"
	}
	return strings.Join(t.events, "\n")
}

type testMessageBus struct {
	taskRepo    repository.TaskRepository
	messageRepo repository.MessageRepository
	tracer      *testTracer
	getClient   func(name string) *testClient
}

type testClient struct{}

func newTestMessageBus(taskRepo repository.TaskRepository, messageRepo repository.MessageRepository, tr *testTracer) *testMessageBus {
	return &testMessageBus{
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
		tracer:      tr,
	}
}

func (mb *testMessageBus) setRegistry(fn func(name string) *testClient) {
	mb.getClient = fn
}

func (mb *testMessageBus) createTask(agentName string) (*model.TaskRecord, error) {
	task := &model.TaskRecord{
		AgentName: agentName,
		State:     model.TaskStateSubmitted,
	}
	return mb.taskRepo.Create(task)
}

// --- Test handlers ---

func testGetAgents(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents := svcCtx.registry.ListAgents()
		if agents == nil {
			writeJSON(w, http.StatusOK, []map[string]interface{}{})
			return
		}
		writeJSON(w, http.StatusOK, agents)
	}
}

func testGetAgent(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract name from path: /api/agents/{name}
		name := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		if name == "" || name == r.URL.Path {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent name is required"})
			return
		}
		agent, err := svcCtx.registry.GetAgent(name)
		if err != nil || agent == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Agent '" + name + "' not found"})
			return
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func testGetCapabilities(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":         "a2a-platform-host",
			"description":  "A2A protocol host platform",
			"version":      "0.1.0",
			"capabilities": map[string]interface{}{"streaming": true},
			"tools":        []map[string]interface{}{
				{"name": "list_agents", "description": "List all agents registered on the A2A platform"},
			},
		})
	}
}

func testGetTasks(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentName := r.URL.Query().Get("agent_name")
		state := r.URL.Query().Get("state")
		tasks, err := svcCtx.taskRepo.List(agentName, model.TaskState(state))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func testGetTaskTrace(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract taskID from path: /api/tasks/{id}/trace
		path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[0] == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}
		taskID := parts[0]
		trace := svcCtx.tracer.getDebugTrace(taskID)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(trace))
	}
}

func testRegisterAgent(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL       string `json:"url"`
			AgentType string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if body.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		// Use URL as name for test simplicity
		record := &model.AgentRecord{
			Name:   body.URL,
			URL:    body.URL,
			Status: model.AgentStatusConnected,
		}
		if err := svcCtx.registry.UpsertAgent(record); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "connected", "name": record.Name, "url": record.URL})
	}
}

func testAddAgent(svcCtx *testServiceContext) http.HandlerFunc {
	return testRegisterAgent(svcCtx)
}

func testChat(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentName string `json:"agent_name"`
			Message   string `json:"message"`
			ContextID string `json:"context_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if body.AgentName == "" || body.Message == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_name and message are required"})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, canFlush := w.(http.Flusher)

		task, err := svcCtx.messageBus.createTask(body.AgentName)
		if err != nil {
			return
		}

		taskData, _ := json.Marshal(map[string]string{"local_task_id": task.LocalTaskID, "agent_name": task.AgentName})
		writeSSEData(w, string(taskData))
		if canFlush {
			flusher.Flush()
		}

		respData, _ := json.Marshal(map[string]string{"type": "response", "text": "echo: " + body.Message})
		writeSSEData(w, string(respData))
		if canFlush {
			flusher.Flush()
		}
	}
}

func writeSSEData(w io.Writer, data string) {
	w.Write([]byte("data: " + data + "\n\n"))
}

// --- Tests ---

func TestGetAgentsEmpty(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var agents []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

func TestGetAgentsAfterRegister(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"url": "http://test-agent:8080"})
	resp, err := http.Post(ts.URL+"/api/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var agents []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0]["name"] != "http://test-agent:8080" {
		t.Fatalf("expected name http://test-agent:8080, got %v", agents[0]["name"])
	}
}

func TestGetAgentExists(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"url": "http://test-agent:8080"})
	resp, err := http.Post(ts.URL+"/api/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/agents/http%3A%2F%2Ftest-agent%3A8080")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var agent map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		t.Fatal(err)
	}
	if agent["name"] != "http://test-agent:8080" {
		t.Fatalf("unexpected agent name: %v", agent["name"])
	}
}

func TestGetAgentNotFound(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/agents/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "Agent 'nonexistent' not found" {
		t.Fatalf("unexpected error: %v", body["error"])
	}
}

func TestGetCapabilities(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var caps map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&caps); err != nil {
		t.Fatal(err)
	}

	if caps["name"] != "a2a-platform-host" {
		t.Fatalf("expected name a2a-platform-host, got %v", caps["name"])
	}
	if caps["version"] != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %v", caps["version"])
	}

	capMap, ok := caps["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities is not a map")
	}
	if capMap["streaming"] != true {
		t.Fatalf("expected streaming=true, got %v", capMap["streaming"])
	}

	tools, ok := caps["tools"].([]interface{})
	if !ok {
		t.Fatal("tools is not an array")
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tools")
	}
}

func TestGetTasksEmpty(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tasks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var tasks []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestGetTasksWithCreatedTask(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{
		"agent_name": "test-agent",
		"message":    "hello",
	})
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/tasks?agent_name=test-agent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var tasks []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["agent_name"] != "test-agent" {
		t.Fatalf("expected agent_name test-agent, got %v", tasks[0]["agent_name"])
	}
}

func TestGetTaskTrace(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tasks/some-id/trace")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "(no trace yet)" {
		t.Fatalf("expected '(no trace yet)', got '%s'", string(body))
	}
}

func TestRegisterAgentMissingURL(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"type": "test"})
	resp, err := http.Post(ts.URL+"/api/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestChatSSE(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{
		"agent_name": "test-agent",
		"message":    "hello world",
	})
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	data, _ := io.ReadAll(resp.Body)
	content := string(data)

	if strings.Count(content, "data: ") < 2 {
		t.Fatalf("expected at least 2 SSE events, got: %s", content)
	}

	if !strings.Contains(content, "echo: hello world") {
		t.Fatalf("expected echo in response, got: %s", content)
	}
}

func TestChatMissingFields(t *testing.T) {
	ts, _ := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"agent_name": "test-agent"})
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestProxyAgent tests the SSE proxy handler with a fake upstream agent.
func TestProxyAgent(t *testing.T) {
	// Start a fake upstream agent
	var upstreamURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":    "fake-agent",
				"url":     upstreamURL,
				"version": "1.0",
				"capabilities": map[string]interface{}{
					"streaming": true,
				},
			})
			return
		}

		// Verify A2A-Version header was injected
		if r.Header.Get("A2A-Version") != "1.0" {
			t.Errorf("expected A2A-Version: 1.0 header, got %s", r.Header.Get("A2A-Version"))
		}
		// Verify hop-by-hop headers were stripped
		if r.Header.Get("Connection") != "" {
			t.Errorf("expected Connection header to be stripped")
		}
		if r.Header.Get("Transfer-Encoding") != "" {
			t.Errorf("expected Transfer-Encoding header to be stripped")
		}

		// Respond with SSE
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"type\":\"task\",\"data\":{\"id\":\"t1\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"type\":\"response\",\"data\":{\"text\":\"hello from agent\"}}\n\n")
		flusher.Flush()
	}))
	upstreamURL = upstream.URL
	defer upstream.Close()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatal(err)
	}

	agentRepo := repository.NewAgentRepository(db)
	traceRepo := repository.NewTraceRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	tr := newTestTracer(traceRepo)
	reg := registry.NewWithRetry(agentRepo, 1, 0)
	bus := newTestMessageBus(taskRepo, messageRepo, tr)
	bus.setRegistry(func(name string) *testClient { return nil })

	// Register the fake agent via ConnectByURL (fetches agent card from upstream)
	_, err = reg.ConnectByURL(upstream.URL, "", "")
	if err != nil {
		t.Fatalf("connect by url: %v", err)
	}

	svcCtx := &testServiceContext{
		db:          db,
		agentRepo:   agentRepo,
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
		traceRepo:   traceRepo,
		registry:    reg,
		messageBus:  bus,
		tracer:      tr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/agent/", testProxyAgent(svcCtx))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"message": "hello"})
	resp, err := http.Post(ts.URL+"/agent/fake-agent", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	content := string(data)

	if !strings.Contains(content, "hello from agent") {
		t.Fatalf("expected 'hello from agent' in response, got: %s", content)
	}
	if !strings.Contains(content, "data:") {
		t.Fatalf("expected SSE data in response, got: %s", content)
	}
}

// testProxyAgent is the proxy handler adapted for test (uses testServiceContext).
func testProxyAgent(svcCtx *testServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentName := strings.TrimPrefix(r.URL.Path, "/agent/")

		conn := svcCtx.registry.GetClient(agentName)
		if conn == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Agent '" + agentName + "' not connected"})
			return
		}

		targetURL := conn.URL
		remaining := strings.TrimPrefix(r.URL.Path, "/agent/"+agentName)
		if remaining == "" {
			remaining = "/"
		}
		targetURL += remaining
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		var bodyReader io.Reader = r.Body
		proxyReq, err := http.NewRequest(r.Method, targetURL, bodyReader)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request: " + err.Error()})
			return
		}

		hopByHop := map[string]bool{
			"connection": true, "keep-alive": true, "proxy-authenticate": true,
			"proxy-authorization": true, "te": true, "trailers": true,
			"transfer-encoding": true, "upgrade": true, "host": true,
			"content-length": true, "accept-encoding": true,
		}
		for key, vals := range r.Header {
			if !hopByHop[strings.ToLower(key)] {
				for _, val := range vals {
					proxyReq.Header.Add(key, val)
				}
			}
		}

		if proxyReq.Header.Get("A2A-Version") == "" {
			proxyReq.Header.Set("A2A-Version", "1.0")
		}

		resp, err := http.DefaultTransport.RoundTrip(proxyReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "proxy request failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		for key, vals := range resp.Header {
			w.Header()[key] = vals
		}
		w.WriteHeader(resp.StatusCode)

		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
	}
}
