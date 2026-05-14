package tracer

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return db
}

func TestRecordSendAndResponse(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	taskID := "task-123"
	contextID := "ctx-456"
	agentName := "test-agent"

	tr.RecordSend(taskID, "hello world", contextID, agentName)
	tr.RecordResponse(taskID, "hi there", agentName)

	timeline, err := tr.GetTimeline(taskID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(timeline))
	}
	if timeline[0].EventType != model.TraceEventSend {
		t.Errorf("expected first event type 'send', got '%s'", timeline[0].EventType)
	}
	if timeline[1].EventType != model.TraceEventResponse {
		t.Errorf("expected second event type 'response', got '%s'", timeline[1].EventType)
	}

	// Check data contains content
	if !strings.Contains(timeline[0].DataJSON, "hello world") {
		t.Errorf("expected send data to contain 'hello world', got '%s'", timeline[0].DataJSON)
	}
	if !strings.Contains(timeline[1].DataJSON, "hi there") {
		t.Errorf("expected response data to contain 'hi there', got '%s'", timeline[1].DataJSON)
	}
}

func TestRecordError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	taskID := "task-error"

	tr.RecordError(taskID, fmt.Errorf("connection refused"))

	timeline, err := tr.GetTimeline(taskID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("expected 1 event, got %d", len(timeline))
	}
	if timeline[0].EventType != model.TraceEventError {
		t.Errorf("expected event type 'error', got '%s'", timeline[0].EventType)
	}
	if !strings.Contains(timeline[0].DataJSON, "connection refused") {
		t.Errorf("expected error data to contain 'connection refused', got '%s'", timeline[0].DataJSON)
	}
}

func TestRecordNilError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	taskID := "task-nil-err"

	tr.RecordError(taskID, nil)

	timeline, err := tr.GetTimeline(taskID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("expected 1 event, got %d", len(timeline))
	}
	if timeline[0].EventType != model.TraceEventError {
		t.Errorf("expected event type 'error', got '%s'", timeline[0].EventType)
	}
}

func TestGetDebugTrace(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	taskID := "task-debug"
	agentName := "debug-agent"

	tr.RecordSend(taskID, "ping", "ctx-1", agentName)
	tr.RecordTaskCreated(taskID, "server-99", "ctx-1")
	tr.RecordTaskUpdate(taskID, string(model.TaskStateSubmitted), string(model.TaskStateWorking))
	tr.RecordResponse(taskID, "pong", agentName)

	debug := tr.GetDebugTrace(taskID)
	if debug == "(no trace yet)" {
		t.Fatal("expected debug trace, got '(no trace yet)'")
	}
	if !strings.Contains(debug, "send") {
		t.Error("expected debug trace to contain 'send'")
	}
	if !strings.Contains(debug, "response") {
		t.Error("expected debug trace to contain 'response'")
	}
	if !strings.Contains(debug, "task_created") {
		t.Error("expected debug trace to contain 'task_created'")
	}
	if !strings.Contains(debug, "task_update") {
		t.Error("expected debug trace to contain 'task_update'")
	}

	// Nonexistent task
	empty := tr.GetDebugTrace("nonexistent-task")
	if empty != "(no trace yet)" {
		t.Errorf("expected '(no trace yet)' for nonexistent, got '%s'", empty)
	}
}

func TestGetRecentByAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	agentA := "agent-a"
	agentB := "agent-b"

	// Record 5 events for agent-a
	for i := 0; i < 5; i++ {
		tr.RecordSend("task-a-"+string(rune('0'+i)), "msg", "ctx", agentA)
	}

	// Record 2 events for agent-b
	tr.RecordSend("task-b-0", "msg", "ctx", agentB)
	tr.RecordSend("task-b-1", "msg", "ctx", agentB)

	// Get recent 3 for agent-a
	recent, err := tr.GetRecentByAgent(agentA, 3)
	if err != nil {
		t.Fatalf("GetRecentByAgent failed: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(recent))
	}

	// All should be for agent-a
	for _, evt := range recent {
		if evt.AgentName != agentA {
			t.Errorf("expected agent_name '%s', got '%s'", agentA, evt.AgentName)
		}
	}

	// Get all for agent-b
	allB, err := tr.GetRecentByAgent(agentB, 0)
	if err != nil {
		t.Fatalf("GetRecentByAgent failed: %v", err)
	}
	if len(allB) != 2 {
		t.Fatalf("expected 2 events for agent-b, got %d", len(allB))
	}
}

func TestFullEventSequenceOrdering(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tr := New(repository.NewTraceRepository(db))

	taskID := "task-seq"
	agentName := "seq-agent"

	tr.RecordTaskCreated(taskID, "server-1", "ctx")
	tr.RecordSend(taskID, "hello", "ctx", agentName)
	tr.RecordTaskUpdate(taskID, "SUBMITTED", "WORKING")
	tr.RecordArtifact(taskID, "some artifact text")
	tr.RecordResponse(taskID, "final answer", agentName)

	timeline, err := tr.GetTimeline(taskID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 5 {
		t.Fatalf("expected 5 events, got %d", len(timeline))
	}

	expected := []model.TraceEventType{
		model.TraceEventTaskCreated,
		model.TraceEventSend,
		model.TraceEventTaskUpdate,
		model.TraceEventArtifact,
		model.TraceEventResponse,
	}
	for i, evt := range timeline {
		if evt.EventType != expected[i] {
			t.Errorf("event %d: expected type '%s', got '%s'", i, expected[i], evt.EventType)
		}
	}
}
