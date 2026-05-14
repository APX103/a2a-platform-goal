# A2A Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a complete A2A (Agent-to-Agent) protocol platform in Go 1.21.13 with go-zero framework, SQLite persistence, SSE streaming, and pass all 53 E2E test cases.

**Architecture:** Host control plane with Registry (agent lifecycle), MessageBus (A2A message orchestration), Tracer (observability), and Proxy (transparent request forwarding). go-zero rest.Server for HTTP, ServiceContext for DI, custom handlers for SSE endpoints. SQLite via modernc.org/sqlite for zero-dependency persistence. Fake agent binary for E2E testing with Docker Compose.

**Tech Stack:** Go 1.21.13, go-zero (rest + goctl), SQLite (modernc.org/sqlite), Docker Compose, net/http for SSE

---

## Phase 1: Foundation

### Task 1: Initialize Project

**Files:**
- Create: `go.mod`, directory structure, `Makefile`
- Create: `etc/host.yaml`

- [ ] **Step 1: Create directory structure**

```bash
cd /Users/apx103/work/a2a_platform
mkdir -p cmd/host etc internal/{config,svc,handler,logic,model,repository,registry,messagebus,tracer,middleware} pkg/{a2a,httputil} e2e/fake_agent docs
```

- [ ] **Step 2: Initialize Go module and install dependencies**

```bash
cd /Users/apx103/work/a2a_platform
export PATH=$PATH:/Users/apx103/go-1.21.13/bin
go mod init a2a-platform
go get github.com/zeromicro/go-zero@latest
go get modernc.org/sqlite
go get github.com/google/uuid
```

- [ ] **Step 3: Install goctl**

```bash
go install github.com/zeromicro/go-zero/tools/goctl@latest
```

- [ ] **Step 4: Create etc/host.yaml**

```yaml
Host: "0.0.0.0"
Port: 7860
DataSource: "file:a2a_platform.db?cache=shared&_journal_mode=WAL"
Timeout: 30000
AgentRetryMax: 3
AgentRetryBaseDelay: 1
LogLevel: info
```

- [ ] **Step 5: Create Makefile**

```makefile
.PHONY: host fake-agent test e2e clean

host:
	go build -o bin/host ./cmd/host

fake-agent:
	go build -o bin/fake_agent ./e2e/fake_agent

test:
	go test ./... -v -count=1

e2e:
	go test ./e2e/... -v -count=1

clean:
	rm -rf bin a2a_platform.db
```

- [ ] **Step 6: Create internal/config/config.go**

```go
package config

type Config struct {
	Host              string `yaml:",default=0.0.0.0"`
	Port              int    `yaml:",default=7860"`
	DataSource        string `yaml:",default=file:a2a_platform.db?cache=shared&_journal_mode=WAL"`
	Timeout           int    `yaml:",default=30000"`
	AgentRetryMax     int    `yaml:",default=3"`
	AgentRetryBaseDelay int  `yaml:",default=1"`
	LogLevel          string `yaml:",default=info"`
}

func MustLoad(path string) *Config {
	var c Config
	conf.MustLoad(path, &c)
	return &c
}
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: initialize project structure and config"
```

---

### Task 2: Data Models

**Files:**
- Create: `internal/model/agent.go`
- Create: `internal/model/task.go`
- Create: `internal/model/message.go`
- Create: `internal/model/trace.go`
- Create: `internal/model/model_test.go`

- [ ] **Step 1: Write model tests**

```go
// internal/model/model_test.go
package model

import "testing"

func TestTaskStateValues(t *testing.T) {
	states := []TaskState{
		TaskStateSubmitted, TaskStateWorking, TaskStateInputRequired,
		TaskStateCompleted, TaskStateFailed, TaskStateError, TaskStateResponded,
	}
	for _, s := range states {
		if string(s) == "" {
			t.Errorf("empty state string for %v", s)
		}
	}
}

func TestAgentStatusValues(t *testing.T) {
	states := []AgentStatus{
		AgentStatusConnected, AgentStatusDisconnected, AgentStatusError,
	}
	for _, s := range states {
		if string(s) == "" {
			t.Errorf("empty status string for %v", s)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/apx103/work/a2a_platform && go test ./internal/model/ -v
```

- [ ] **Step 3: Write model implementations**

```go
// internal/model/agent.go
package model

import "time"

type AgentStatus string

const (
	AgentStatusConnected    AgentStatus = "connected"
	AgentStatusDisconnected AgentStatus = "disconnected"
	AgentStatusError        AgentStatus = "error"
)

type AgentRecord struct {
	ID           int64      `db:"id" json:"-"`
	Name         string     `db:"name" json:"name"`
	URL          string     `db:"url" json:"url"`
	Description  string     `db:"description" json:"description"`
	Version      string     `db:"version" json:"version"`
	AgentType    string     `db:"type" json:"type"`
	Status       AgentStatus `db:"status" json:"status"`
	SkillsJSON   string     `db:"skills_json" json:"-"`
	Skills       []string   `db:"-" json:"skills"`
	ErrorMessage string     `db:"error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}
```

```go
// internal/model/task.go
package model

import "time"

type TaskState string

const (
	TaskStateSubmitted    TaskState = "SUBMITTED"
	TaskStateWorking      TaskState = "WORKING"
	TaskStateInputRequired TaskState = "INPUT_REQUIRED"
	TaskStateCompleted    TaskState = "COMPLETED"
	TaskStateFailed       TaskState = "FAILED"
	TaskStateError        TaskState = "ERROR"
	TaskStateResponded    TaskState = "RESPONDED"
)

type TaskRecord struct {
	ID           int64     `db:"id" json:"-"`
	LocalTaskID  string    `db:"local_task_id" json:"local_task_id"`
	AgentName    string    `db:"agent_name" json:"agent_name"`
	ContextID    string    `db:"context_id" json:"context_id"`
	State        TaskState `db:"state" json:"state"`
	DisplayID    string    `db:"display_id" json:"display_id"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}
```

```go
// internal/model/message.go
package model

import "time"

type MessageRole string

const (
	MessageRoleUser MessageRole = "user"
	MessageRoleAgent MessageRole = "agent"
)

type MessageRecord struct {
	ID        int64      `db:"id" json:"-"`
	TaskID    int64      `db:"task_id" json:"-"`
	Role      MessageRole `db:"role" json:"role"`
	Content   string     `db:"content" json:"content"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}
```

```go
// internal/model/trace.go
package model

import "time"

type TraceEventType string

const (
	TraceEventSend         TraceEventType = "send"
	TraceEventTaskCreated  TraceEventType = "task_created"
	TraceEventTaskUpdate   TraceEventType = "task_update"
	TraceEventArtifact     TraceEventType = "artifact"
	TraceEventResponse     TraceEventType = "response"
	TraceEventError        TraceEventType = "error"
)

type TraceEventRecord struct {
	ID           int64          `db:"id" json:"-"`
	TaskID       string         `db:"task_id" json:"task_id"`
	ContextID    string         `db:"context_id" json:"context_id"`
	AgentName    string         `db:"agent_name" json:"agent_name"`
	TargetAgent  string         `db:"target_agent" json:"target_agent,omitempty"`
	EventType    TraceEventType `db:"event_type" json:"event_type"`
	DataJSON     string         `db:"data_json" json:"-"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/model/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add data models for agents, tasks, messages, traces"
```

---

### Task 3: A2A Protocol Types

**Files:**
- Create: `pkg/a2a/types.go`
- Create: `pkg/a2a/types_test.go`

- [ ] **Step 1: Write failing tests for A2A types**

```go
// pkg/a2a/types_test.go
package a2a

import (
	"encoding/json"
	"testing"
)

func TestAgentCardUnmarshal(t *testing.T) {
	raw := `{
		"name": "test-agent",
		"description": "A test agent",
		"version": "1.0.0",
		"url": "http://localhost:10001",
		"capabilities": {"streaming": true},
		"skills": [{"id": "chat", "name": "Chat", "description": "Chat with agent"}]
	}`
	var card AgentCard
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatal(err)
	}
	if card.Name != "test-agent" {
		t.Errorf("expected name=test-agent, got %s", card.Name)
	}
	if len(card.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(card.Skills))
	}
}

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "SendStreamingMessage",
		Params:  SendStreamingMessageParams{Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "hello"}}}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	if decoded["method"] != "SendStreamingMessage" {
		t.Error("wrong method")
	}
}

func TestSSEEventTypes(t *testing.T) {
	types := []SSEEventType{SSEEventTask, SSEEventStatusUpdate, SSEEventArtifactUpdate, SSEEventMessage}
	for _, tt := range types {
		if string(tt) == "" {
			t.Errorf("empty SSE event type")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/a2a/ -v
```

- [ ] **Step 3: Implement A2A types**

```go
// pkg/a2a/types.go
package a2a

// AgentCard represents the /.well-known/agent.json response
type AgentCard struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Version             string       `json:"version"`
	URL                 string       `json:"url"`
	Capabilities        Capabilities `json:"capabilities"`
	Skills              []Skill      `json:"skills"`
	SupportedInterfaces []string     `json:"supportedInterfaces,omitempty"`
}

type Capabilities struct {
	Streaming bool `json:"streaming"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

// A2A Task model
type Task struct {
	ID        string    `json:"id"`
	ContextID string    `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TaskStatus string

const (
	TaskStatusSubmitted     TaskStatus = "SUBMITTED"
	TaskStatusWorking       TaskStatus = "WORKING"
	TaskStatusInputRequired TaskStatus = "INPUT_REQUIRED"
	TaskStatusCompleted     TaskStatus = "COMPLETED"
	TaskStatusFailed        TaskStatus = "FAILED"
)

// Message
type Message struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// JSON-RPC 2.0
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type SendStreamingMessageParams struct {
	Message   Message `json:"message"`
	TaskID    string  `json:"taskId,omitempty"`
	ContextID string  `json:"contextId,omitempty"`
}

// SSE Event types
type SSEEventType string

const (
	SSEEventTask          SSEEventType = "task"
	SSEEventStatusUpdate  SSEEventType = "status_update"
	SSEEventArtifactUpdate SSEEventType = "artifact_update"
	SSEEventMessage       SSEEventType = "message"
)

// SSE Event envelope (from A2A spec: {type, data})
type SSEEventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./pkg/a2a/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add A2A protocol types (AgentCard, JSON-RPC, SSE events)"
```

---

## Phase 2: Repository Layer

### Task 4: Database Initialization & Agent Repository

**Files:**
- Create: `internal/repository/db.go`
- Create: `internal/repository/agent_repo.go`
- Create: `internal/repository/agent_repo_test.go`

- [ ] **Step 1: Write agent repo tests (TDD)**

```go
// internal/repository/agent_repo_test.go
package repository

import (
	"database/sql"
	"testing"
	"a2a-platform/internal/model"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := InitDB(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAgentRepo_CreateAndGet(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewAgentRepo(db)

	record := &model.AgentRecord{
		Name:    "agent-a",
		URL:     "http://localhost:10001",
		AgentType: "helloworld",
		Status:  model.AgentStatusConnected,
	}
	if err := repo.Create(record); err != nil {
		t.Fatal(err)
	}
	if record.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := repo.GetByName("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "agent-a" {
		t.Errorf("expected agent-a, got %s", got.Name)
	}
	if got.Status != model.AgentStatusConnected {
		t.Errorf("expected connected, got %s", got.Status)
	}
}

func TestAgentRepo_Upsert(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewAgentRepo(db)

	record := &model.AgentRecord{Name: "agent-a", URL: "http://localhost:10001", Status: model.AgentStatusConnected}
	repo.Create(record)

	// Upsert with same name
	record2 := &model.AgentRecord{Name: "agent-a", URL: "http://localhost:10001", Status: model.AgentStatusError, ErrorMessage: "fail"}
	if err := repo.Upsert(record2); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByName("agent-a")
	if got.Status != model.AgentStatusError {
		t.Errorf("expected error, got %s", got.Status)
	}
}

func TestAgentRepo_List(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewAgentRepo(db)

	repo.Create(&model.AgentRecord{Name: "agent-a", URL: "http://localhost:10001", Status: model.AgentStatusConnected})
	repo.Create(&model.AgentRecord{Name: "agent-b", URL: "http://localhost:10002", Status: model.AgentStatusConnected})

	list, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestAgentRepo_UpdateStatus(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewAgentRepo(db)

	repo.Create(&model.AgentRecord{Name: "agent-a", URL: "http://localhost:10001", Status: model.AgentStatusConnected})
	if err := repo.UpdateStatus("agent-a", model.AgentStatusDisconnected, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByName("agent-a")
	if got.Status != model.AgentStatusDisconnected {
		t.Errorf("expected disconnected, got %s", got.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/repository/ -run TestAgent -v
```

- [ ] **Step 3: Implement db.go and agent_repo.go**

```go
// internal/repository/db.go
package repository

import "database/sql"

func InitDB(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		url TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		version TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'disconnected',
		skills_json TEXT NOT NULL DEFAULT '[]',
		error_message TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		local_task_id TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		context_id TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'SUBMITTED',
		display_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id),
		role TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS trace_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		context_id TEXT NOT NULL DEFAULT '',
		agent_name TEXT NOT NULL DEFAULT '',
		target_agent TEXT NOT NULL DEFAULT '',
		event_type TEXT NOT NULL,
		data_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_context_id ON tasks(context_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_agent_name ON tasks(agent_name);
	CREATE INDEX IF NOT EXISTS idx_messages_task_id ON messages(task_id);
	CREATE INDEX IF NOT EXISTS idx_traces_task_id ON trace_events(task_id);
	CREATE INDEX IF NOT EXISTS idx_traces_agent_name ON trace_events(agent_name);
	`
	_, err := db.Exec(schema)
	return err
}
```

```go
// internal/repository/agent_repo.go
package repository

import (
	"database/sql"
	"a2a-platform/internal/model"
)

type AgentRepo interface {
	Create(record *model.AgentRecord) error
	GetByName(name string) (*model.AgentRecord, error)
	List() ([]*model.AgentRecord, error)
	Upsert(record *model.AgentRecord) error
	UpdateStatus(name string, status model.AgentStatus, errorMessage string) error
	Delete(name string) error
}

type agentRepo struct {
	db *sql.DB
}

func NewAgentRepo(db *sql.DB) AgentRepo {
	return &agentRepo{db: db}
}

func (r *agentRepo) Create(record *model.AgentRecord) error {
	result, err := r.db.Exec(
		`INSERT INTO agents (name, url, description, version, type, status, skills_json, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Name, record.URL, record.Description, record.Version,
		record.AgentType, record.Status, record.SkillsJSON, record.ErrorMessage,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	record.ID = id
	return nil
}

func (r *agentRepo) GetByName(name string) (*model.AgentRecord, error) {
	record := &model.AgentRecord{}
	err := r.db.QueryRow(
		`SELECT id, name, url, description, version, type, status, skills_json, error_message, created_at, updated_at
		 FROM agents WHERE name = ?`, name,
	).Scan(&record.ID, &record.Name, &record.URL, &record.Description, &record.Version,
		&record.AgentType, &record.Status, &record.SkillsJSON, &record.ErrorMessage,
		&record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *agentRepo) List() ([]*model.AgentRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, name, url, description, version, type, status, skills_json, error_message, created_at, updated_at
		 FROM agents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*model.AgentRecord
	for rows.Next() {
		record := &model.AgentRecord{}
		if err := rows.Scan(&record.ID, &record.Name, &record.URL, &record.Description, &record.Version,
			&record.AgentType, &record.Status, &record.SkillsJSON, &record.ErrorMessage,
			&record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *agentRepo) Upsert(record *model.AgentRecord) error {
	_, err := r.db.Exec(
		`INSERT INTO agents (name, url, description, version, type, status, skills_json, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET url=excluded.url, description=excluded.description,
		 version=excluded.version, type=excluded.type, status=excluded.status,
		 skills_json=excluded.skills_json, error_message=excluded.error_message, updated_at=CURRENT_TIMESTAMP`,
		record.Name, record.URL, record.Description, record.Version,
		record.AgentType, record.Status, record.SkillsJSON, record.ErrorMessage,
	)
	return err
}

func (r *agentRepo) UpdateStatus(name string, status model.AgentStatus, errorMessage string) error {
	_, err := r.db.Exec(
		`UPDATE agents SET status=?, error_message=?, updated_at=CURRENT_TIMESTAMP WHERE name=?`,
		status, errorMessage, name)
	return err
}

func (r *agentRepo) Delete(name string) error {
	_, err := r.db.Exec(`DELETE FROM agents WHERE name=?`, name)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/repository/ -run TestAgent -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add SQLite schema and agent repository"
```

---

### Task 5: Task, Message, and Trace Repositories

**Files:**
- Create: `internal/repository/task_repo.go`
- Create: `internal/repository/message_repo.go`
- Create: `internal/repository/trace_repo.go`
- Create: `internal/repository/repo_test.go`

- [ ] **Step 1: Write repo tests**

```go
// internal/repository/repo_test.go
package repository

import (
	"database/sql"
	"testing"
	"a2a-platform/internal/model"
	"time"
)

func TestTaskRepo_CreateAndGet(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTaskRepo(db)

	task, err := repo.Create("agent-a", "ctx-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.LocalTaskID == "" {
		t.Fatal("expected non-empty task ID")
	}

	got, err := repo.Get(task.LocalTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentName != "agent-a" {
		t.Errorf("expected agent-a, got %s", got.AgentName)
	}
	if got.State != model.TaskStateSubmitted {
		t.Errorf("expected SUBMITTED, got %s", got.State)
	}
}

func TestTaskRepo_UpdateState(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTaskRepo(db)

	task, _ := repo.Create("agent-a", "ctx-1")
	repo.UpdateState(task.LocalTaskID, model.TaskStateWorking)

	got, _ := repo.Get(task.LocalTaskID)
	if got.State != model.TaskStateWorking {
		t.Errorf("expected WORKING, got %s", got.State)
	}
}

func TestTaskRepo_List(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTaskRepo(db)

	repo.Create("agent-a", "ctx-1")
	repo.Create("agent-b", "ctx-2")

	list, err := repo.List("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Errorf("expected >=2 tasks, got %d", len(list))
	}
}

func TestTaskRepo_ListByAgentName(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTaskRepo(db)

	repo.Create("agent-a", "ctx-1")
	repo.Create("agent-b", "ctx-2")

	list, _ := repo.List("agent-a", "")
	if len(list) != 1 {
		t.Errorf("expected 1 task for agent-a, got %d", len(list))
	}
}

func TestTaskRepo_ListByState(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTaskRepo(db)

	task, _ := repo.Create("agent-a", "ctx-1")
	repo.UpdateState(task.LocalTaskID, model.TaskStateResponded)

	list, _ := repo.List("", string(model.TaskStateResponded))
	if len(list) != 1 {
		t.Errorf("expected 1 responded task, got %d", len(list))
	}
}

func TestMessageRepo_CreateAndList(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	taskRepo := NewTaskRepo(db)
	task, _ := taskRepo.Create("agent-a", "ctx-1")
	msgRepo := NewMessageRepo(db)

	msgRepo.Create(task.ID, model.MessageRoleUser, "hello")
	msgRepo.Create(task.ID, model.MessageRoleAgent, "hi there")

	msgs, err := msgRepo.ListByTaskID(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestTraceRepo_RecordAndGetTimeline(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTraceRepo(db)

	repo.Record(&model.TraceEventRecord{
		TaskID: "task-1", AgentName: "sender->agent-a",
		EventType: model.TraceEventSend, DataJSON: `{"message":"hello"}`,
	})
	repo.Record(&model.TraceEventRecord{
		TaskID: "task-1", AgentName: "sender->agent-a",
		EventType: model.TraceEventResponse, DataJSON: `{"text":"hi"}`,
	})

	timeline, err := repo.GetTimeline("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 2 {
		t.Errorf("expected 2 events, got %d", len(timeline))
	}
}

func TestTraceRepo_GetRecentByAgent(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	repo := NewTraceRepo(db)

	now := time.Now().UTC()
	repo.Record(&model.TraceEventRecord{TaskID: "t1", AgentName: "sender->agent-a", EventType: model.TraceEventSend, CreatedAt: now})
	repo.Record(&model.TraceEventRecord{TaskID: "t2", AgentName: "sender->agent-a", EventType: model.TraceEventResponse, CreatedAt: now.Add(time.Second)})

	events, err := repo.GetRecentByAgent("sender->agent-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2, got %d", len(events))
	}
	// Test limit
	events2, _ := repo.GetRecentByAgent("sender->agent-a", 1)
	if len(events2) != 1 {
		t.Errorf("expected 1 with limit, got %d", len(events2))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/repository/ -run "TestTask|TestMessage|TestTrace" -v
```

- [ ] **Step 3: Implement task_repo.go**

```go
// internal/repository/task_repo.go
package repository

import (
	"database/sql"
	"a2a-platform/internal/model"
	"github.com/google/uuid"
	"strings"
)

type TaskRepo interface {
	Create(agentName, contextID string) (*model.TaskRecord, error)
	Get(localTaskID string) (*model.TaskRecord, error)
	UpdateState(localTaskID string, state model.TaskState) error
	List(agentName, state string) ([]*model.TaskRecord, error)
}

type taskRepo struct {
	db *sql.DB
}

func NewTaskRepo(db *sql.DB) TaskRepo {
	return &taskRepo{db: db}
}

func (r *taskRepo) Create(agentName, contextID string) (*model.TaskRecord, error) {
	localTaskID := uuid.New().String()
	result, err := r.db.Exec(
		`INSERT INTO tasks (local_task_id, agent_name, context_id, state)
		 VALUES (?, ?, ?, 'SUBMITTED')`,
		localTaskID, agentName, contextID,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &model.TaskRecord{
		ID: id, LocalTaskID: localTaskID, AgentName: agentName,
		ContextID: contextID, State: model.TaskStateSubmitted,
	}, nil
}

func (r *taskRepo) Get(localTaskID string) (*model.TaskRecord, error) {
	record := &model.TaskRecord{}
	err := r.db.QueryRow(
		`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
		 FROM tasks WHERE local_task_id=?`, localTaskID,
	).Scan(&record.ID, &record.LocalTaskID, &record.AgentName, &record.ContextID,
		&record.State, &record.DisplayID, &record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *taskRepo) UpdateState(localTaskID string, state model.TaskState) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET state=?, updated_at=CURRENT_TIMESTAMP WHERE local_task_id=?`,
		state, localTaskID)
	return err
}

func (r *taskRepo) List(agentName, state string) ([]*model.TaskRecord, error) {
	query := `SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at FROM tasks WHERE 1=1`
	var args []interface{}
	if agentName != "" {
		query += ` AND agent_name=?`
		args = append(args, agentName)
	}
	if state != "" {
		query += ` AND state=?`
		args = append(args, state)
	}
	query += ` ORDER BY id`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*model.TaskRecord
	for rows.Next() {
		rec := &model.TaskRecord{}
		if err := rows.Scan(&rec.ID, &rec.LocalTaskID, &rec.AgentName, &rec.ContextID,
			&rec.State, &rec.DisplayID, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}
```

- [ ] **Step 4: Implement message_repo.go**

```go
// internal/repository/message_repo.go
package repository

import (
	"database/sql"
	"a2a-platform/internal/model"
)

type MessageRepo interface {
	Create(taskID int64, role model.MessageRole, content string) error
	ListByTaskID(taskID int64) ([]*model.MessageRecord, error)
}

type messageRepo struct {
	db *sql.DB
}

func NewMessageRepo(db *sql.DB) MessageRepo {
	return &messageRepo{db: db}
}

func (r *messageRepo) Create(taskID int64, role model.MessageRole, content string) error {
	_, err := r.db.Exec(
		`INSERT INTO messages (task_id, role, content) VALUES (?, ?, ?)`,
		taskID, role, content)
	return err
}

func (r *messageRepo) ListByTaskID(taskID int64) ([]*model.MessageRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, role, content, created_at FROM messages WHERE task_id=? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*model.MessageRecord
	for rows.Next() {
		rec := &model.MessageRecord{}
		if err := rows.Scan(&rec.ID, &rec.TaskID, &rec.Role, &rec.Content, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}
```

- [ ] **Step 5: Implement trace_repo.go**

```go
// internal/repository/trace_repo.go
package repository

import (
	"database/sql"
	"a2a-platform/internal/model"
	"strings"
)

type TraceRepo interface {
	Record(event *model.TraceEventRecord) error
	GetTimeline(taskID string) ([]*model.TraceEventRecord, error)
	GetDebugTrace(taskID string) string
	GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error)
}

type traceRepo struct {
	db *sql.DB
}

func NewTraceRepo(db *sql.DB) TraceRepo {
	return &traceRepo{db: db}
}

func (r *traceRepo) Record(event *model.TraceEventRecord) error {
	_, err := r.db.Exec(
		`INSERT INTO trace_events (task_id, context_id, agent_name, target_agent, event_type, data_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		event.TaskID, event.ContextID, event.AgentName, event.TargetAgent,
		event.EventType, event.DataJSON)
	return err
}

func (r *traceRepo) GetTimeline(taskID string) ([]*model.TraceEventRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, context_id, agent_name, target_agent, event_type, data_json, created_at
		 FROM trace_events WHERE task_id=? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*model.TraceEventRecord
	for rows.Next() {
		e := &model.TraceEventRecord{}
		if err := rows.Scan(&e.ID, &e.TaskID, &e.ContextID, &e.AgentName, &e.TargetAgent,
			&e.EventType, &e.DataJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (r *traceRepo) GetDebugTrace(taskID string) string {
	events, err := r.GetTimeline(taskID)
	if err != nil || len(events) == 0 {
		return "(no trace yet)"
	}
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(e.CreatedAt.Format("15:04:05"))
		sb.WriteString(" **")
		sb.WriteString(strings.ToUpper(string(e.EventType)))
		sb.WriteString("**: ")
		sb.WriteString(e.DataJSON)
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r *traceRepo) GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, context_id, agent_name, target_agent, event_type, data_json, created_at
		 FROM trace_events WHERE agent_name=? ORDER BY created_at DESC LIMIT ?`, agentName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*model.TraceEventRecord
	for rows.Next() {
		e := &model.TraceEventRecord{}
		if err := rows.Scan(&e.ID, &e.TaskID, &e.ContextID, &e.AgentName, &e.TargetAgent,
			&e.EventType, &e.DataJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}
```

- [ ] **Step 6: Run all repo tests**

```bash
go test ./internal/repository/ -v
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: add task, message, and trace repositories"
```

---

## Phase 3: Core Services

### Task 6: SSE Parser/Writer

**Files:**
- Create: `pkg/a2a/sse.go`
- Create: `pkg/a2a/sse_test.go`

- [ ] **Step 1: Write SSE tests**

```go
// pkg/a2a/sse_test.go
package a2a

import (
	"io"
	"strings"
	"testing"
)

func TestParseSSEEvents(t *testing.T) {
	reader := strings.NewReader(strings.Join([]string{
		"data: {\"type\":\"status_update\",\"data\":{\"status\":\"WORKING\"}}\n\n",
		"data: {\"type\":\"message\",\"data\":{\"role\":\"agent\",\"parts\":[{\"kind\":\"text\",\"text\":\"hello\"}]}}\n\n",
		"data: {\"type\":\"status_update\",\"data\":{\"status\":\"COMPLETED\"}}\n\n",
	}, ""))

	events, err := ParseSSEStream(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Type != "message" {
		t.Errorf("expected message, got %s", events[1].Type)
	}
}

func TestParseSSEEvents_Malformed(t *testing.T) {
	reader := strings.NewReader("not valid sse data\n\n")
	events, err := ParseSSEStream(reader)
	if err != nil {
		t.Fatal("should not error on malformed data")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestWriteSSEData(t *testing.T) {
	var buf strings.Builder
	WriteSSEData(&buf, `{"type":"message","data":"test"}`)
	if !strings.Contains(buf.String(), "data: ") {
		t.Error("expected 'data: ' prefix")
	}
	if !strings.HasSuffix(buf.String(), "\n\n") {
		t.Error("expected double newline suffix")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/a2a/ -run TestParseSSE -v
```

- [ ] **Step 3: Implement SSE parser/writer**

```go
// pkg/a2a/sse.go
package a2a

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

func ParseSSEStream(r io.Reader) ([]SSEEventEnvelope, error) {
	var events []SSEEventEnvelope
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "" && len(dataLines) > 0 {
			// End of event
			raw := strings.Join(dataLines, "\n")
			var event SSEEventEnvelope
			if err := json.Unmarshal([]byte(raw), &event); err == nil {
				events = append(events, event)
			}
			dataLines = nil
		}
	}
	return events, scanner.Err()
}

func WriteSSEData(w io.Writer, data string) {
	w.Write([]byte("data: " + data + "\n\n"))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./pkg/a2a/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add SSE parser and writer for A2A protocol"
```

---

### Task 7: A2A HTTP Client

**Files:**
- Create: `pkg/a2a/client.go`
- Create: `pkg/a2a/client_test.go`

- [ ] **Step 1: Write client tests**

```go
// pkg/a2a/client_test.go
package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_SendStreamingMessage(t *testing.T) {
	// Create a fake SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("A2A-Version") != "1.0" {
			t.Error("expected A2A-Version: 1.0 header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"message\",\"data\":{\"role\":\"agent\",\"parts\":[{\"kind\":\"text\",\"text\":\"echo\"}]}}\n\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	events, err := client.SendStreamingMessage("user", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "message" {
		t.Errorf("expected message type, got %s", events[0].Type)
	}
}

func TestClient_FetchAgentCard(t *testing.T) {
	card := AgentCard{Name: "test-agent", Version: "1.0"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent.json" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(card)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	fetched, err := client.FetchAgentCard()
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Name != "test-agent" {
		t.Errorf("expected test-agent, got %s", fetched.Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/a2a/ -run TestClient -v
```

- [ ] **Step 3: Implement A2A client**

```go
// pkg/a2a/client.go
package a2a

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) FetchAgentCard() (*AgentCard, error) {
	url := fmt.Sprintf("%s/.well-known/agent.json", c.baseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch agent card: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch agent card: status %d", resp.StatusCode)
	}
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode agent card: %w", err)
	}
	return &card, nil
}

func (c *Client) SendStreamingMessage(role, text, taskID string) ([]SSEEventEnvelope, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "SendStreamingMessage",
		Params: SendStreamingMessageParams{
			Message: Message{Role: role, Parts: []Part{{Kind: "text", Text: text}}},
			TaskID:  taskID,
		},
	}
	body, _ := json.Marshal(req)

	url := fmt.Sprintf("%s/", c.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("A2A-Version", "1.0")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send streaming message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("send streaming message: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	if resp.Header.Get("Content-Type") == "text/event-stream" {
		return ParseSSEStream(resp.Body)
	}
	// Non-SSE response: wrap as single event
	respBody, _ := io.ReadAll(resp.Body)
	return []SSEEventEnvelope{{
		Type: "message",
		Data: json.RawMessage(respBody),
	}}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./pkg/a2a/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add A2A HTTP client (agent card, streaming message)"
```

---

### Task 8: Tracer Service

**Files:**
- Create: `internal/tracer/tracer.go`
- Create: `internal/tracer/tracer_test.go`

- [ ] **Step 1: Write tracer tests**

```go
// internal/tracer/tracer_test.go
package tracer

import (
	"database/sql"
	"encoding/json"
	"testing"
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"

	_ "modernc.org/sqlite"
)

func testTracer(t *testing.T) (*Tracer, *sql.DB) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatal(err)
	}
	return NewTracer(repository.NewTraceRepo(db)), db
}

func TestTracer_SendAndResponse(t *testing.T) {
	tr, db := testTracer(t)
	defer db.Close()

	taskID := "task-1"
	tr.RecordSend(taskID, "hello", "ctx-1", "sender->agent-a")
	tr.RecordResponse(taskID, "hi there", "message")

	timeline, err := tr.GetTimeline(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(timeline))
	}
	if timeline[0].EventType != model.TraceEventSend {
		t.Errorf("expected send, got %s", timeline[0].EventType)
	}
}

func TestTracer_DebugTrace(t *testing.T) {
	tr, db := testTracer(t)
	defer db.Close()

	trace := tr.GetDebugTrace("nonexistent")
	if trace != "(no trace yet)" {
		t.Errorf("expected '(no trace yet)', got %s", trace)
	}

	tr.RecordSend("task-1", "hello", "ctx-1", "sender->agent-a")
	trace = tr.GetDebugTrace("task-1")
	if trace == "(no trace yet)" {
		t.Error("expected non-empty trace")
	}
}

func TestTracer_GetRecentByAgent(t *testing.T) {
	tr, db := testTracer(t)
	defer db.Close()

	tr.RecordSend("t1", "msg1", "ctx-1", "sender->agent-a")
	tr.RecordResponse("t1", "resp1", "message")
	tr.RecordSend("t2", "msg2", "ctx-2", "sender->agent-a")

	events, err := tr.GetRecentByAgent("sender->agent-a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3, got %d", len(events))
	}

	events2, _ := tr.GetRecentByAgent("sender->agent-a", 1)
	if len(events2) != 1 {
		t.Errorf("expected 1 with limit, got %d", len(events2))
	}
}

func TestTracer_FullEventSequence(t *testing.T) {
	tr, db := testTracer(t)
	defer db.Close()

	taskID := "task-1"
	tr.RecordSend(taskID, "test", "ctx-1", "sender->agent-a")
	tr.RecordTaskCreated(taskID, "server-123", "ctx-1")
	tr.RecordTaskUpdate(taskID, "SUBMITTED", "WORKING")
	tr.RecordArtifact(taskID, "generated artifact")
	tr.RecordResponse(taskID, "final response", "message")

	events, _ := tr.GetTimeline(taskID)
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
	// Verify order
	expected := []model.TraceEventType{
		model.TraceEventSend, model.TraceEventTaskCreated, model.TraceEventTaskUpdate,
		model.TraceEventArtifact, model.TraceEventResponse,
	}
	for i, e := range events {
		if e.EventType != expected[i] {
			t.Errorf("event %d: expected %s, got %s", i, expected[i], e.EventType)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tracer/ -v
```

- [ ] **Step 3: Implement tracer**

```go
// internal/tracer/tracer.go
package tracer

import (
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"encoding/json"
	"fmt"
)

type Tracer struct {
	repo repository.TraceRepo
}

func NewTracer(repo repository.TraceRepo) *Tracer {
	return &Tracer{repo: repo}
}

func (t *Tracer) RecordSend(taskID, content, contextID, agentName string) {
	data, _ := json.Marshal(map[string]string{"message": content})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, ContextID: contextID, AgentName: agentName,
		EventType: model.TraceEventSend, DataJSON: string(data),
	})
}

func (t *Tracer) RecordTaskCreated(taskID, serverTaskID, contextID string) {
	data, _ := json.Marshal(map[string]string{"server_task_id": serverTaskID})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, ContextID: contextID,
		EventType: model.TraceEventTaskCreated, DataJSON: string(data),
	})
}

func (t *Tracer) RecordTaskUpdate(taskID, oldState, newState string) {
	data, _ := json.Marshal(map[string]string{"old_state": oldState, "new_state": newState})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, EventType: model.TraceEventTaskUpdate, DataJSON: string(data),
	})
}

func (t *Tracer) RecordArtifact(taskID, text string) {
	data, _ := json.Marshal(map[string]string{"artifact": text})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, EventType: model.TraceEventArtifact, DataJSON: string(data),
	})
}

func (t *Tracer) RecordResponse(taskID, text, source string) {
	data, _ := json.Marshal(map[string]string{"text": text, "source": source})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, EventType: model.TraceEventResponse, DataJSON: string(data),
	})
}

func (t *Tracer) RecordError(taskID string, err error) {
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	t.repo.Record(&model.TraceEventRecord{
		TaskID: taskID, EventType: model.TraceEventError, DataJSON: string(data),
	})
}

func (t *Tracer) GetTimeline(taskID string) ([]*model.TraceEventRecord, error) {
	return t.repo.GetTimeline(taskID)
}

func (t *Tracer) GetDebugTrace(taskID string) string {
	return t.repo.GetDebugTrace(taskID)
}

func (t *Tracer) GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error) {
	return t.repo.GetRecentByAgent(agentName, limit)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tracer/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add tracer service for A2A event recording"
```

---

### Task 9: Registry Service

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/registry_test.go`

- [ ] **Step 1: Write registry tests**

```go
// internal/registry/registry_test.go
package registry

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/pkg/a2a"

	_ "modernc.org/sqlite"
)

func testRegistry(t *testing.T) (*Registry, *sql.DB, *httptest.Server) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatal(err)
	}
	// Create fake agent server
	card := a2a.AgentCard{
		Name: "agent-a", Description: "Test Agent", Version: "1.0",
		Capabilities: a2a.Capabilities{Streaming: true},
		Skills: []a2a.Skill{{ID: "chat", Name: "Chat"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			json.NewEncoder(w).Encode(card)
		}
	}))
	reg := NewRegistry(
		repository.NewAgentRepo(db),
		3, // retry max
		1, // base delay
	)
	return reg, db, server
}

func TestRegistry_ConnectByURL(t *testing.T) {
	reg, db, server := testRegistry(t)
	defer db.Close()
	defer server.Close()

	conn, err := reg.ConnectByURL(server.URL, "helloworld")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	if conn.Card.Name != "agent-a" {
		t.Errorf("expected agent-a, got %s", conn.Card.Name)
	}

	// Should be in DB
	record, _ := reg.GetAgent("agent-a")
	if record == nil {
		t.Fatal("agent not in DB")
	}
	if record.Status != model.AgentStatusConnected {
		t.Errorf("expected connected, got %s", record.Status)
	}
}

func TestRegistry_Disconnect(t *testing.T) {
	reg, db, server := testRegistry(t)
	defer db.Close()
	defer server.Close()

	reg.ConnectByURL(server.URL, "helloworld")
	reg.Disconnect("agent-a")

	client := reg.GetClient("agent-a")
	if client != nil {
		t.Error("expected nil client after disconnect")
	}
	record, _ := reg.GetAgent("agent-a")
	if record.Status != model.AgentStatusDisconnected {
		t.Errorf("expected disconnected, got %s", record.Status)
	}
}

func TestRegistry_Reconnect(t *testing.T) {
	reg, db, server := testRegistry(t)
	defer db.Close()
	defer server.Close()

	reg.ConnectByURL(server.URL, "helloworld")
	reg.Disconnect("agent-a")
	conn, err := reg.ConnectByURL(server.URL, "helloworld")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected connection after reconnect")
	}
	if reg.GetClient("agent-a") == nil {
		t.Error("expected client after reconnect")
	}
}

func TestRegistry_ListAgents(t *testing.T) {
	reg, db, server := testRegistry(t)
	defer db.Close()
	defer server.Close()

	reg.ConnectByURL(server.URL, "helloworld")
	agents := reg.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestRegistry_DuplicateConnect(t *testing.T) {
	reg, db, server := testRegistry(t)
	defer db.Close()
	defer server.Close()

	reg.ConnectByURL(server.URL, "helloworld")
	// Second connect to same agent
	reg.ConnectByURL(server.URL, "helloworld")
	agents := reg.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent (no duplicate), got %d", len(agents))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/registry/ -v
```

- [ ] **Step 3: Implement registry**

```go
// internal/registry/registry.go
package registry

import (
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/pkg/a2a"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type AgentConnection struct {
	Name    string
	URL     string
	Card    *a2a.AgentCard
	Type    string
	Client  *a2a.Client
}

type Registry struct {
	repo        repository.AgentRepo
	connections map[string]*AgentConnection
	mu          sync.RWMutex
	retryMax    int
	baseDelay   time.Duration
}

func NewRegistry(repo repository.AgentRepo, retryMax int, baseDelay int) *Registry {
	return &Registry{
		repo:        repo,
		connections: make(map[string]*AgentConnection),
		retryMax:    retryMax,
		baseDelay:   time.Duration(baseDelay) * time.Second,
	}
}

func (r *Registry) ConnectByURL(url, agentType string) (*AgentConnection, error) {
	client := a2a.NewClient(url)
	var lastErr error
	for attempt := 0; attempt < r.retryMax; attempt++ {
		if attempt > 0 {
			delay := r.baseDelay * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
		}
		card, err := client.FetchAgentCard()
		if err != nil {
			lastErr = err
			continue
		}
		conn := &AgentConnection{
			Name:   card.Name,
			URL:    url,
			Card:   card,
			Type:   agentType,
			Client: client,
		}
		skillsJSON, _ := json.Marshal(card.Skills)
		record := &model.AgentRecord{
			Name:       card.Name,
			URL:        url,
			Description: card.Description,
			Version:    card.Version,
			AgentType:  agentType,
			Status:     model.AgentStatusConnected,
			SkillsJSON: string(skillsJSON),
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		// Check if already connected
		if existing, ok := r.connections[card.Name]; ok {
			return existing, nil
		}
		r.repo.Upsert(record)
		r.connections[card.Name] = conn
		return conn, nil
	}
	return nil, fmt.Errorf("connect agent after %d retries: %w", r.retryMax, lastErr)
}

func (r *Registry) Disconnect(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, name)
	r.repo.UpdateStatus(name, model.AgentStatusDisconnected, "")
}

func (r *Registry) GetClient(name string) *AgentConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[name]
}

func (r *Registry) GetAgent(name string) (*model.AgentRecord, error) {
	return r.repo.GetByName(name)
}

func (r *Registry) ListAgents() []*model.AgentRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	records, _ := r.repo.List()
	for _, rec := range records {
		if conn, ok := r.connections[rec.Name]; ok {
			rec.Description = conn.Card.Description
			rec.Version = conn.Card.Version
			rec.Status = model.AgentStatusConnected
		} else if rec.Status == model.AgentStatusConnected {
			rec.Status = model.AgentStatusDisconnected
		}
	}
	return records
}

func (r *Registry) UpsertAgent(record *model.AgentRecord) error {
	return r.repo.Upsert(record)
}

func (r *Registry) DeleteAgent(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, name)
	return r.repo.Delete(name)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/registry/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add registry service (connect, disconnect, list agents)"
```

---

### Task 10: MessageBus Service

**Files:**
- Create: `internal/messagebus/messagebus.go`
- Create: `internal/messagebus/tools.go`
- Create: `internal/messagebus/messagebus_test.go`

- [ ] **Step 1: Write MessageBus tests**

```go
// internal/messagebus/messagebus_test.go
package messagebus

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"

	_ "modernc.org/sqlite"
)

func testMessageBus(t *testing.T) (*MessageBus, *sql.DB, *httptest.Server) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			json.NewEncoder(w).Encode(a2a.AgentCard{Name: "agent-a", Version: "1.0"})
			return
		}
		// SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"status_update\",\"data\":{\"status\":\"WORKING\"}}\n\n"))
		w.Write([]byte("data: {\"type\":\"message\",\"data\":{\"role\":\"agent\",\"parts\":[{\"kind\":\"text\",\"text\":\"echo: hello\"}]}}\n\n"))
		w.Write([]byte("data: {\"type\":\"status_update\",\"data\":{\"status\":\"COMPLETED\"}}\n\n"))
	}))
	tr := tracer.NewTracer(repository.NewTraceRepo(db))
	bus := NewMessageBus(
		repository.NewTaskRepo(db),
		repository.NewMessageRepo(db),
		tr,
	)
	bus.SetRegistry(func(name string) *a2a.Client {
		if name == "agent-a" {
			return a2a.NewClient(server.URL)
		}
		return nil
	})
	return bus, db, server
}

func TestMessageBus_SendStreaming(t *testing.T) {
	bus, db, server := testMessageBus(t)
	defer db.Close()
	defer server.Close()

	task, events, err := bus.SendStreaming("sender", "agent-a", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.State != model.TaskStateResponded {
		t.Errorf("expected RESPONDED, got %s", task.State)
	}
	if len(events) == 0 {
		t.Error("expected events")
	}
}

func TestMessageBus_Send(t *testing.T) {
	bus, db, server := testMessageBus(t)
	defer db.Close()
	defer server.Close()

	resp, err := bus.Send("sender", "agent-a", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	// Should contain echo response
	if resp == "" {
		t.Error("expected non-empty response")
	}
}

func TestMessageBus_ContextContinuation(t *testing.T) {
	bus, db, server := testMessageBus(t)
	defer db.Close()
	defer server.Close()

	// First message
	bus.Send("sender", "agent-a", "msg1", "ctx-1")
	// Second message with same context
	bus.Send("sender", "agent-a", "msg2", "ctx-1")

	// Should reuse same task (check messages)
	tasks, _ := bus.taskRepo.List("agent-a", "")
	if len(tasks) < 1 {
		t.Error("expected at least 1 task")
	}
}

func TestMessageBus_HandleToolCall(t *testing.T) {
	bus, db, server := testMessageBus(t)
	defer db.Close()
	defer server.Close()

	// Test no host configured
	result := HandleToolCall("list_agents", nil, "")
	if result != "(no host configured)" {
		t.Errorf("expected '(no host configured)', got %s", result)
	}
}

func TestMessageBus_SendNonexistentAgent(t *testing.T) {
	bus, db, _ := testMessageBus(t)
	defer db.Close()

	_, _, err := bus.SendStreaming("sender", "nonexistent", "hello", "")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/messagebus/ -v
```

- [ ] **Step 3: Implement MessageBus**

```go
// internal/messagebus/messagebus.go
package messagebus

import (
	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"
	"encoding/json"
	"fmt"
	"strings"
)

type StreamingEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type MessageBus struct {
	taskRepo    repository.TaskRepo
	messageRepo repository.MessageRepo
	tracer      *tracer.Tracer
	getClient   func(name string) *a2a.Client
}

func NewMessageBus(taskRepo repository.TaskRepo, messageRepo repository.MessageRepo, tr *tracer.Tracer) *MessageBus {
	return &MessageBus{
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
		tracer:      tr,
	}
}

func (b *MessageBus) SetRegistry(getClient func(name string) *a2a.Client) {
	b.getClient = getClient
}

func (b *MessageBus) CreateTask(agentName string) (*model.TaskRecord, error) {
	return b.taskRepo.Create(agentName, "")
}

func (b *MessageBus) Send(sender, agentName, content, contextID string) (string, error) {
	task, events, err := b.SendStreaming(sender, agentName, content, contextID)
	if err != nil {
		return "", err
	}
	for _, e := range events {
		if e.Type == "response" {
			return e.Data.(string), nil
		}
	}
	return "(empty)", nil
}

func (b *MessageBus) SendStreaming(sender, agentName, content, contextID string) (*model.TaskRecord, []StreamingEvent, error) {
	client := b.getClient(agentName)
	if client == nil {
		return nil, nil, fmt.Errorf("agent '%s' not found", agentName)
	}

	agentNameKey := sender + "->" + agentName

	// Create or reuse task
	task, err := b.taskRepo.Create(agentName, contextID)
	if err != nil {
		return nil, nil, err
	}

	// Record user message
	b.messageRepo.Create(task.ID, model.MessageRoleUser, content)

	// Record send trace
	b.tracer.RecordSend(task.LocalTaskID, content, contextID, agentNameKey)

	var events []StreamingEvent
	var responseText string
	responseSource := ""

	defer func() {
		if responseText != "" {
			b.messageRepo.Create(task.ID, model.MessageRoleAgent, responseText)
			b.tracer.RecordResponse(task.LocalTaskID, responseText, responseSource)
		}
	}()

	// Send to agent via A2A client
	sseEvents, err := client.SendStreamingMessage("user", content, task.LocalTaskID)
	if err != nil {
		b.taskRepo.UpdateState(task.LocalTaskID, model.TaskStateError)
		b.tracer.RecordError(task.LocalTaskID, err)
		events = append(events, StreamingEvent{Type: "error", Data: err.Error()})
		return task, events, err
	}

	// Process SSE events
	for _, evt := range sseEvents {
		switch evt.Type {
		case "task":
			var taskData struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if json.Unmarshal(evt.Data, &taskData) == nil {
				b.tracer.RecordTaskCreated(task.LocalTaskID, taskData.ID, contextID)
				b.taskRepo.UpdateState(task.LocalTaskID, model.TaskState(model.TaskStateWorking))
				events = append(events, StreamingEvent{Type: "task_update", Data: taskData})
			}

		case "status_update":
			var statusData struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(evt.Data, &statusData) == nil {
				b.taskRepo.UpdateState(task.LocalTaskID, model.TaskState(statusData.Status))
				events = append(events, StreamingEvent{Type: "task_update", Data: statusData})
			}

		case "artifact_update":
			var artifactData struct {
				Artifact struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"artifact"`
			}
			if json.Unmarshal(evt.Data, &artifactData) == nil {
				artifactText := ""
				for _, p := range artifactData.Artifact.Parts {
					artifactText += p.Text
				}
				if artifactText != "" {
					b.tracer.RecordArtifact(task.LocalTaskID, artifactText)
					responseText = artifactText
					responseSource = "artifact"
				}
				events = append(events, StreamingEvent{Type: "artifact", Data: artifactText})
			}

		case "message":
			var msgData struct {
				Role  string `json:"role"`
				Parts []struct {
					Kind string `json:"kind"`
					Text string `json:"text"`
				} `json:"parts"`
			}
			if json.Unmarshal(evt.Data, &msgData) == nil {
				var textParts []string
				for _, p := range msgData.Parts {
					if p.Kind == "text" {
						textParts = append(textParts, p.Text)
					}
				}
				joined := strings.Join(textParts, "")
				if joined != "" {
					responseText = joined
					responseSource = "message"
				}
				events = append(events, StreamingEvent{Type: "response", Data: joined})
			}
		}
	}

	// Fallback for empty response
	if responseText == "" {
		responseText = "(empty)"
		responseSource = "fallback"
		events = append(events, StreamingEvent{Type: "response", Data: responseText})
	}

	b.taskRepo.UpdateState(task.LocalTaskID, model.TaskStateResponded)
	return task, events, nil
}
```

- [ ] **Step 4: Implement tools.go**

```go
// internal/messagebus/tools.go
package messagebus

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var HostTools = []map[string]interface{}{
	{
		"name":        "list_agents",
		"description": "List all registered agents on the host platform",
		"parameters":  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	},
	{
		"name":        "send_to_agent",
		"description": "Send a message to another agent via the host",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_name": map[string]interface{}{"type": "string", "description": "Target agent name"},
				"message":    map[string]interface{}{"type": "string", "description": "Message to send"},
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
				"agent_name": map[string]interface{}{"type": "string", "description": "Agent name to query"},
			},
			"required": []string{"agent_name"},
		},
	},
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func HandleToolCall(toolName string, params map[string]interface{}, hostURL string) string {
	if hostURL == "" {
		return "(no host configured)"
	}

	switch toolName {
	case "list_agents":
		resp, err := httpClient.Get(hostURL + "/api/agents")
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)

	case "send_to_agent":
		agentName, _ := params["agent_name"].(string)
		message, _ := params["message"].(string)
		reqBody, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0", "id": "tool-call",
			"method": "SendStreamingMessage",
			"params": map[string]interface{}{
				"message": map[string]interface{}{
					"role":  "user",
					"parts": []map[string]string{{"kind": "text", "text": message}},
				},
			},
		})
		resp, err := httpClient.Post(
			hostURL+"/agent/"+agentName,
			"application/json",
			io.NopCloser(strings.NewReader(string(reqBody))),
		)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)

	case "get_agent_info":
		agentName, _ := params["agent_name"].(string)
		resp, err := httpClient.Get(hostURL + "/api/agents/" + agentName)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)

	default:
		return fmt.Sprintf("Unknown tool: %s", toolName)
	}
}
```

Note: `tools.go` needs `import "strings"` added.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/messagebus/ -v
```

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add messagebus service (send, streaming, tools)"
```

---

## Phase 4: API Layer

### Task 11: go-zero Setup & ServiceContext

**Files:**
- Create: `host.api`
- Create: `internal/svc/servicecontext.go`
- Create: `cmd/host/main.go`

- [ ] **Step 1: Create host.api**

```api
syntax = "v1"

type (
    AgentInfo {
        Name         string   `json:"name"`
        Description  string   `json:"description"`
        URL          string   `json:"url"`
        Status       string   `json:"status"`
        Type         string   `json:"type"`
        Version      string   `json:"version"`
        Skills       []string `json:"skills"`
        ErrorMessage string   `json:"error_message,omitempty"`
    }

    TaskInfo {
        LocalTaskID string `json:"local_task_id"`
        AgentName   string `json:"agent_name"`
        State       string `json:"state"`
        DisplayID   string `json:"display_id"`
        CreatedAt   string `json:"created_at"`
    }

    RegisterRequest {
        Name   string   `json:"name"`
        Type   string   `json:"type"`
        Port   int      `json:"port"`
        Skills []Skill  `json:"skills,omitempty"`
        URL    string   `json:"url,omitempty"`
        Secret string   `json:"secret,omitempty"`
    }

    AddAgentRequest {
        URL string `json:"url"`
    }

    TraceResponse {
        TaskID string `json:"task_id"`
        Trace  string `json:"trace"`
    }

    Skill {
        ID          string   `json:"id"`
        Name        string   `json:"name"`
        Description string   `json:"description"`
        Tags        []string `json:"tags,omitempty"`
    }

    ErrorResponse {
        Error string `json:"error"`
    }
)

@server (
    prefix: /api
    middleware: A2AVersionMiddleware
)
service host-api {
    @handler GetAgents
    get /agents returns ([]AgentInfo)

    @handler GetAgent
    get /agents/:name returns (AgentInfo)

    @handler GetCapabilities
    get /capabilities

    @handler GetTasks
    get /tasks returns ([]TaskInfo)

    @handler GetTaskTrace
    get /tasks/:id/trace returns (TraceResponse)

    @handler RegisterAgent
    post /agents/register

    @handler AddAgent
    post /agents returns (AgentInfo)

    @handler DeleteAgent
    delete /agents/:name
}

@server (
    middleware: A2AVersionMiddleware
)
service host-api {
    @handler ProxyHandler
    post /agent/:name

    @handler ChatHandler
    post /api/chat
}
```

- [ ] **Step 2: Run goctl to generate code**

```bash
cd /Users/apx103/work/a2a_platform
goctl api go -api host.api -dir . --style goZero
```

- [ ] **Step 3: Create ServiceContext**

```go
// internal/svc/servicecontext.go
package svc

import (
	"a2a-platform/internal/config"
	"a2a-platform/internal/messagebus"
	"a2a-platform/internal/model"
	"a2a-platform/internal/registry"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"
	"database/sql"
	_ "modernc.org/sqlite"
)

type ServiceContext struct {
	Config      *config.Config
	DB          *sql.DB
	AgentRepo   repository.AgentRepo
	TaskRepo    repository.TaskRepo
	MessageRepo repository.MessageRepo
	TraceRepo   repository.TraceRepo
	Registry    *registry.Registry
	MessageBus  *messagebus.MessageBus
	Tracer      *tracer.Tracer
}

func NewServiceContext(c *config.Config) *ServiceContext {
	db, err := sql.Open("sqlite", c.DataSource)
	if err != nil {
		panic("open db: " + err.Error())
	}
	if err := repository.InitDB(db); err != nil {
		panic("init db: " + err.Error())
	}

	agentRepo := repository.NewAgentRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	messageRepo := repository.NewMessageRepo(db)
	traceRepo := repository.NewTraceRepo(db)
	tr := tracer.NewTracer(traceRepo)
	reg := registry.NewRegistry(agentRepo, c.AgentRetryMax, c.AgentRetryBaseDelay)
	bus := messagebus.NewMessageBus(taskRepo, messageRepo, tr)
	bus.SetRegistry(func(name string) *a2a.Client {
		conn := reg.GetClient(name)
		if conn == nil {
			return nil
		}
		return conn.Client
	})

	return &ServiceContext{
		Config:      c,
		DB:          db,
		AgentRepo:   agentRepo,
		TaskRepo:    taskRepo,
		MessageRepo: messageRepo,
		TraceRepo:   traceRepo,
		Registry:    reg,
		MessageBus:  bus,
		Tracer:      tr,
	}
}
```

- [ ] **Step 4: Create cmd/host/main.go**

```go
package main

import (
	"flag"
	"fmt"
	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/svc"
)

var configFile = flag.String("f", "etc/host.yaml", "the config file")

func main() {
	flag.Parse()
	c := config.MustLoad(*configFile)
	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(rest.RestConf{
		Host:    c.Host,
		Port:    c.Port,
		Timeout: c.Timeout,
	})
	defer server.Stop()

	handler.RegisterHandlers(server, svcCtx)

	// Register custom SSE handlers
	handler.RegisterCustomHandlers(server, svcCtx)

	fmt.Printf("A2A Host started on %s:%d\n", c.Host, c.Port)
	server.Start()
}
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add go-zero setup (api definition, service context, main)"
```

---

### Task 12: Discovery & Capabilities Handlers

**Files:**
- Create: `internal/logic/discovery.go`
- Create: `internal/logic/discovery_test.go`
- Modify: generated handler files

- [ ] **Step 1: Write discovery logic tests**

```go
// internal/logic/discovery_test.go
package logic

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"a2a-platform/internal/config"
	"a2a-platform/internal/svc"
	"a2a-platform/pkg/a2a"

	_ "modernc.org/sqlite"
)

func testServiceContext(t *testing.T) (*svc.ServiceContext, *sql.DB, *httptest.Server) {
	db, _ := sql.Open("sqlite", ":memory:")
	initDB(t, db)

	card := a2a.AgentCard{Name: "agent-a", Description: "Test", Version: "1.0",
		Capabilities: a2a.Capabilities{Streaming: true},
		Skills: []a2a.Skill{{ID: "chat", Name: "Chat"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			json.NewEncoder(w).Encode(card)
		}
	}))

	c := &config.Config{DataSource: "file::memory:?cache=shared", Port: 0}
	// Directly construct ServiceContext with in-memory DB
	svcCtx := constructServiceContext(db, server.URL, c)
	return svcCtx, db, server
}
```

- [ ] **Step 2: Implement discovery logic**

```go
// internal/logic/discovery.go
package logic

import (
	"a2a-platform/internal/svc"
	"a2a-platform/internal/model"
	"context"
	"encoding/json"
)

type GetAgentsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAgentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAgentsLogic {
	return &GetAgentsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetAgentsLogic) GetAgents() (interface{}, error) {
	records := l.svcCtx.Registry.ListAgents()
	var result []map[string]interface{}
	for _, r := range records {
		skills := parseSkills(r.SkillsJSON)
		agent := map[string]interface{}{
			"name":        r.Name,
			"description": r.Description,
			"url":         "/agent/" + r.Name,
			"status":      string(r.Status),
			"type":        r.AgentType,
			"version":     r.Version,
			"skills":      skills,
		}
		if r.ErrorMessage != "" {
			agent["error_message"] = r.ErrorMessage
		}
		result = append(result, agent)
	}
	return result, nil
}

type GetAgentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetAgentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAgentLogic {
	return &GetAgentLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetAgentLogic) GetAgent(name string) (interface{}, error) {
	record, err := l.svcCtx.Registry.GetAgent(name)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return map[string]string{"error": "Agent '" + name + "' not found"}, nil
	}
	skills := parseSkills(record.SkillsJSON)
	conn := l.svcCtx.Registry.GetClient(name)
	result := map[string]interface{}{
		"name":        record.Name,
		"description": record.Description,
		"url":         record.URL,
		"status":      string(record.Status),
		"type":        record.AgentType,
		"version":     record.Version,
		"skills":      skills,
	}
	if conn != nil {
		result["description"] = conn.Card.Description
		result["version"] = conn.Card.Version
	}
	if record.ErrorMessage != "" {
		result["error_message"] = record.ErrorMessage
	}
	return result, nil
}

type GetCapabilitiesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetCapabilitiesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCapabilitiesLogic {
	return &GetCapabilitiesLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetCapabilitiesLogic) GetCapabilities() (interface{}, error) {
	return map[string]interface{}{
		"protocol": "A2A JSON-RPC 2.0",
		"endpoints": map[string]string{
			"/api/agents":        "List all agents",
			"/api/agents/:name":  "Get agent details",
			"/api/capabilities":  "Platform capabilities",
			"/api/tasks":         "List tasks",
			"/agent/:name":       "Send message to agent",
			"/api/chat":          "Chat with agent (SSE)",
		},
		"message_format": "A2A JSON-RPC over HTTP with SSE streaming",
		"tool_definitions": messagebus.HostTools,
	}, nil
}

func parseSkills(skillsJSON string) []string {
	var skills []string
	if skillsJSON != "" && skillsJSON != "[]" {
		json.Unmarshal([]byte(skillsJSON), &skills)
	}
	return skills
}
```

- [ ] **Step 3: Wire up handlers in the generated handler files**

Map each generated handler to call the corresponding logic.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/logic/ -run TestDiscovery -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add discovery and capabilities handlers"
```

---

### Task 13: Task & Proxy Handlers

**Files:**
- Create: `internal/logic/tasks.go`
- Create: `internal/logic/proxy.go`
- Create: `internal/middleware/a2a_version.go`
- Create: `internal/middleware/a2a_version_test.go`

- [ ] **Step 1: Implement task handlers**

```go
// internal/logic/tasks.go
package logic

import (
	"a2a-platform/internal/svc"
	"context"
	"strconv"
)

type GetTasksLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTasksLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTasksLogic {
	return &GetTasksLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetTasksLogic) GetTasks(agentName, state string) (interface{}, error) {
	records, err := l.svcCtx.TaskRepo.List(agentName, state)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	for _, r := range records {
		result = append(result, map[string]interface{}{
			"local_task_id": r.LocalTaskID,
			"agent_name":    r.AgentName,
			"state":         string(r.State),
			"display_id":    r.DisplayID,
			"created_at":    r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

type GetTaskTraceLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTaskTraceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTaskTraceLogic {
	return &GetTaskTraceLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetTaskTraceLogic) GetTaskTrace(taskID string) (interface{}, error) {
	return map[string]interface{}{
		"task_id": taskID,
		"trace":   l.svcCtx.Tracer.GetDebugTrace(taskID),
	}, nil
}
```

- [ ] **Step 2: Implement proxy handler (SSE proxy)**

```go
// internal/logic/proxy.go
package logic

import (
	"a2a-platform/internal/svc"
	"a2a-platform/pkg/a2a"
	"context"
	"io"
	"net/http"
	"strings"
)

var hopByHopHeaders = []string{
	"host", "content-length", "connection", "accept-encoding", "transfer-encoding",
}

type ProxyLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewProxyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProxyLogic {
	return &ProxyLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ProxyLogic) Proxy(w http.ResponseWriter, r *http.Request, agentName string) {
	conn := l.svcCtx.Registry.GetClient(agentName)
	if conn == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Agent '` + agentName + `' not found"}`))
		return
	}

	// Copy request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build target URL
	targetPath := strings.TrimPrefix(r.URL.Path, "/agent/"+agentName)
	targetURL := conn.URL + "/" + strings.TrimPrefix(targetPath, "/")
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create proxy request
	targetReq, err := http.NewRequest(r.Method, targetURL, io.NopCloser(strings.NewReader(string(body))))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Copy headers, stripping hop-by-hop
	for k, vv := range r.Header {
		lower := strings.ToLower(k)
		skip := false
		for _, h := range hopByHopHeaders {
			if lower == h {
				skip = true
				break
			}
		}
		if !skip {
			for _, v := range vv {
				targetReq.Header.Add(k, v)
			}
		}
	}

	// Inject A2A-Version if missing
	if targetReq.Header.Get("A2A-Version") == "" {
		targetReq.Header.Set("A2A-Version", "1.0")
	}

	// Execute request
	resp, err := http.DefaultTransport.RoundTrip(targetReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}
```

- [ ] **Step 3: Implement A2A-Version middleware**

```go
// internal/middleware/a2a_version.go
package middleware

import (
	"net/http"
)

type A2AVersionMiddleware struct {
}

func NewA2AVersionMiddleware() *A2AVersionMiddleware {
	return &A2AVersionMiddleware{}
}

func (m *A2AVersionMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/logic/ -v && go test ./internal/middleware/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add task handlers, proxy, and A2A-Version middleware"
```

---

### Task 14: Chat Handler (SSE)

**Files:**
- Create: `internal/logic/chat.go`

- [ ] **Step 1: Implement chat handler**

The chat handler accepts POST /api/chat with `{agent_name, message, context_id}` and returns SSE stream via MessageBus.SendStreaming.

```go
// internal/logic/chat.go
package logic

import (
	"a2a-platform/internal/svc"
	"a2a-platform/pkg/a2a"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type ChatLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewChatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ChatLogic {
	return &ChatLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ChatLogic) Chat(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		AgentName string `json:"agent_name"`
		Message   string `json:"message"`
		ContextID string `json:"context_id"`
	}
	json.Unmarshal(body, &req)

	if req.AgentName == "" || req.Message == "" {
		http.Error(w, "agent_name and message required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	task, events, err := l.svcCtx.MessageBus.SendStreaming("user", req.AgentName, req.Message, req.ContextID)
	if err != nil {
		a2a.WriteSSEData(w, `{"type":"error","data":"`+err.Error()+`"}`)
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	// Send task created event
	taskJSON, _ := json.Marshal(map[string]string{
		"local_task_id": task.LocalTaskID,
		"agent_name":    task.AgentName,
		"state":         string(task.State),
	})
	a2a.WriteSSEData(w, `{"type":"task_created","data":`+string(taskJSON)+`}`)
	if flusher != nil {
		flusher.Flush()
	}

	// Send all streaming events
	for _, evt := range events {
		evtJSON, _ := json.Marshal(evt)
		a2a.WriteSSEData(w, string(evtJSON))
		if flusher != nil {
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "feat: add SSE chat handler"
```

---

### Task 15: Registration Handler

**Files:**
- Create: `internal/logic/register.go`

- [ ] **Step 1: Implement registration handler**

```go
// internal/logic/register.go
package logic

import (
	"a2a-platform/internal/svc"
	"context"
	"encoding/json"
	"net"
	"net/http"
)

type RegisterLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *RegisterLogic) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string          `json:"name"`
		Type   string          `json:"type"`
		Port   int             `json:"port"`
		Skills json.RawMessage `json:"skills"`
		URL    string          `json:"url"`
		Secret string          `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Derive URL if not provided
	agentURL := req.URL
	if agentURL == "" {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "" {
			host = "127.0.0.1"
		}
		agentURL = "http://" + host + ":" + strconv.Itoa(req.Port)
	}

	// Connect to agent
	conn, err := l.svcCtx.Registry.ConnectByURL(agentURL, req.Type)
	if err != nil {
		// Mark as error
		l.svcCtx.Registry.UpsertAgent(&model.AgentRecord{
			Name: req.Name, URL: agentURL, AgentType: req.Type,
			Status: model.AgentStatusError, ErrorMessage: err.Error(),
		})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":         conn.Name,
		"url":          conn.URL,
		"status":       "connected",
		"re_registered": false,
	})
}
```

- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "feat: add agent registration handler"
```

---

## Phase 5: Fake Agent & E2E Testing

### Task 16: Fake Agent Binary

**Files:**
- Create: `e2e/fake_agent/main.go`
- Create: `e2e/fake_agent/modes.go`

- [ ] **Step 1: Implement fake agent modes**

The fake agent supports multiple modes via `?mode=xxx` query parameter:

```go
// e2e/fake_agent/modes.go
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AgentCard endpoint
func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/.well-known/agent.json" {
		w.WriteHeader(404)
		return
	}
	mode := r.URL.Query().Get("mode")
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "fake-agent"
	}
	card := map[string]interface{}{
		"name":        name,
		"description": "Fake agent for testing (" + mode + ")",
		"version":     "1.0.0",
		"url":         "http://" + r.Host,
		"capabilities": map[string]bool{"streaming": true},
		"skills":      []map[string]string{{"id": mode, "name": mode, "description": "Test mode: " + mode}},
	}
	json.NewEncoder(w).Encode(card)
}

// SSE response modes
func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/.well-known/agent.json" {
		handleAgentCard(w, r)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "echo"
	}

	// Read request
	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	acceptSSE := r.Header.Get("Accept") == "text/event-stream"
	if !acceptSSE && mode != "json_response" && mode != "error" {
		mode = "json_response"
	}

	switch mode {
	case "echo":
		handleEcho(w, req)
	case "sequence":
		handleSequence(w)
	case "artifact":
		handleArtifact(w)
	case "json_response":
		handleJSONResponse(w, req)
	case "error":
		handleError(w)
	case "record_headers":
		handleRecordHeaders(w, r)
	case "record_url":
		handleRecordURL(w, r)
	case "full_events":
		handleFullEvents(w)
	case "tool_call":
		handleToolCall(w, r)
	case "disconnect_mid_stream":
		handleDisconnect(w)
	case "empty_response":
		handleEmptyResponse(w)
	case "delayed_start":
		handleEcho(w, req)
	default:
		handleEcho(w, req)
	}
}
```

- [ ] **Step 2: Implement each mode function**

Implement echo, sequence, artifact, json_response, error, record_headers, record_url, full_events, tool_call, disconnect_mid_stream, empty_response modes.

- [ ] **Step 3: Implement main.go**

```go
// e2e/fake_agent/main.go
package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "10001"
	}
	fmt.Printf("Fake agent starting on :%s\n", port)
	http.HandleFunc("/", handleRequest)
	http.ListenAndServe(":"+port, nil)
}
```

- [ ] **Step 4: Build and verify**

```bash
go build -o bin/fake_agent ./e2e/fake_agent && ./bin/fake_agent &
curl http://localhost:10001/.well-known/agent.json
kill %1
```

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add fake agent binary with multiple test modes"
```

---

### Task 17: Docker Compose Setup

**Files:**
- Create: `e2e/Dockerfile.host`
- Create: `e2e/Dockerfile.agent`
- Create: `e2e/docker-compose.yml`

- [ ] **Step 1: Create Dockerfiles and docker-compose.yml**

The docker-compose starts Host + Fake Agent. E2E test binary runs from host machine connecting to the services.

```yaml
# e2e/docker-compose.yml
version: '3.8'
services:
  host:
    build:
      context: ..
      dockerfile: e2e/Dockerfile.host
    ports:
      - "${HOST_PORT:-7860}:7860"
    environment:
      - DATA_SOURCE=file:/data/a2a_platform.db?cache=shared&_journal_mode=WAL
    volumes:
      - host-data:/data

  fake-agent:
    build:
      context: ..
      dockerfile: e2e/Dockerfile.agent
    environment:
      - PORT=10001
      - MODE=echo
    ports:
      - "10001:10001"

  fake-agent-b:
    build:
      context: ..
      dockerfile: e2e/Dockerfile.agent
    environment:
      - PORT=10002
      - MODE=echo
    ports:
      - "10002:10002"

volumes:
  host-data:
```

- [ ] **Step 2: Test Docker Compose**

```bash
cd e2e && docker-compose up -d --build && docker-compose logs && docker-compose down
```

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: add Docker Compose setup for E2E testing"
```

---

### Task 18-24: E2E Tests (53 cases)

Each E2E test file corresponds to a category from the test document. Tests start the Host and fake agent programmatically (using httptest for Host, a real HTTP server for fake agent), then make HTTP requests and verify responses.

**Files:**
- Create: `e2e/test_helper.go` — shared fixtures
- Create: `e2e/messaging_test.go` — 11 cases
- Create: `e2e/discovery_test.go` — 12 cases
- Create: `e2e/tool_call_test.go` — 6 cases
- Create: `e2e/tracing_test.go` — 8 cases
- Create: `e2e/registry_test.go` — 6 cases
- Create: `e2e/error_test.go` — 8 cases

For each test:
1. Start Host with in-memory SQLite and random port
2. Start fake agent with specific mode on random port
3. Register agent with Host
4. Send HTTP request
5. Verify response

Each E2E test follows this pattern:

```go
func TestMessaging_Echo(t *testing.T) {
    // 1. Start fake agent
    agent := startFakeAgent(t, "echo")
    defer agent.Close()

    // 2. Start host
    host := startHost(t)
    defer host.Close()

    // 3. Register agent
    registerAgent(t, host.URL, agent.URL)

    // 4. Send message
    resp := postJSON(t, host.URL+"/agent/fake-agent", map[string]interface{}{
        "message": "hello",
    })

    // 5. Verify
    assertStatus(t, resp, 200)
    assertContains(t, resp.Body, "hello")
}
```

**Key implementation notes:**
- `startHost(t)` — builds and starts the Host server on a random port using `rest.MustNewServer`
- `startFakeAgent(t, mode)` — starts the fake agent binary on a random port
- `registerAgent(t, hostURL, agentURL)` — calls POST /api/agents/register or uses registry directly
- For SSE tests, read SSE stream line by line and parse `data:` fields
- For tool_call tests, the fake agent needs access to the Host URL to call tools back

See the test document for exact verification criteria per test case.

- [ ] **Implement all 53 E2E test cases across 7 test files**
- [ ] **Verify all E2E tests pass**

```bash
go test ./e2e/... -v -count=1
```

---

## Phase 6: Documentation

### Task 25: Usage Documentation (HTML)

**Files:**
- Create: `docs/usage.html`

- [ ] **Step 1: Write usage.html**

A complete HTML page documenting:
- How to build and run the Host
- How to register agents (static config, remote self-registration, dynamic add)
- API reference (all endpoints with examples)
- A2A protocol overview
- Configuration options
- Running E2E tests
- Architecture diagram

- [ ] **Step 2: Commit**

```bash
git add -A && git commit -m "docs: add HTML usage documentation"
```

---

## Self-Review Checklist

### Spec Coverage
- [x] Registry (connect/disconnect/list/get) → Task 9
- [x] MessageBus (send/send_streaming/create_task/handle_tool_call) → Task 10
- [x] Tracer (all event types, timeline, debug, per-agent) → Task 8
- [x] Proxy (A2A-Version injection, hop-by-hop stripping, path forwarding) → Task 13
- [x] Discovery (list/get/capabilities) → Task 12
- [x] Chat (SSE streaming) → Task 14
- [x] Registration → Task 15
- [x] SQLite persistence → Tasks 4-5
- [x] A2A protocol types → Task 3
- [x] SSE parser/writer → Task 6
- [x] Fake agent (all modes) → Task 16
- [x] 53 E2E tests → Tasks 18-24
- [x] Docker Compose → Task 17
- [x] HTML docs → Task 25

### Key Dependencies
- Task 10 depends on Tasks 5, 7, 8
- Task 9 depends on Tasks 3, 4
- Tasks 12-15 depend on Task 11
- Tasks 18-24 depend on Tasks 11-16
