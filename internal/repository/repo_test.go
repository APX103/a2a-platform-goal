package repository

import (
	"fmt"
	"strings"
	"testing"

	"a2a-platform/internal/model"

	_ "modernc.org/sqlite"
)

// --- TaskRepository tests ---

func TestCreateAndGetTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTaskRepository(db)

	created, err := repo.Create(&model.TaskRecord{
		AgentName: "test-agent",
		ContextID: "ctx-123",
		State:     model.TaskStateSubmitted,
		DisplayID: "disp-001",
	})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero task ID")
	}
	if created.LocalTaskID == "" {
		t.Error("expected generated local_task_id")
	}

	got, err := repo.Get(created.LocalTaskID)
	if err != nil {
		t.Fatalf("Get task failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.LocalTaskID != created.LocalTaskID {
		t.Errorf("expected local_task_id '%s', got '%s'", created.LocalTaskID, got.LocalTaskID)
	}
	if got.AgentName != "test-agent" {
		t.Errorf("expected agent_name 'test-agent', got '%s'", got.AgentName)
	}
	if got.State != model.TaskStateSubmitted {
		t.Errorf("expected state SUBMITTED, got '%s'", got.State)
	}
}

func TestUpdateStateTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTaskRepository(db)

	created, err := repo.Create(&model.TaskRecord{
		AgentName: "test-agent",
		State:     model.TaskStateSubmitted,
	})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}

	err = repo.UpdateState(created.LocalTaskID, model.TaskStateWorking)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	got, err := repo.Get(created.LocalTaskID)
	if err != nil {
		t.Fatalf("Get task failed: %v", err)
	}
	if got.State != model.TaskStateWorking {
		t.Errorf("expected state WORKING, got '%s'", got.State)
	}
}

func TestListTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTaskRepository(db)

	// Create tasks with different agents and states
	_, err := repo.Create(&model.TaskRecord{
		AgentName: "agent-a",
		State:     model.TaskStateSubmitted,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = repo.Create(&model.TaskRecord{
		AgentName: "agent-a",
		State:     model.TaskStateWorking,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = repo.Create(&model.TaskRecord{
		AgentName: "agent-b",
		State:     model.TaskStateCompleted,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List all
	tasks, err := repo.List("", "")
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// List by agent name
	tasks, err = repo.List("agent-a", "")
	if err != nil {
		t.Fatalf("List by agent failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for agent-a, got %d", len(tasks))
	}

	// List by state
	tasks, err = repo.List("", model.TaskStateSubmitted)
	if err != nil {
		t.Fatalf("List by state failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task with state SUBMITTED, got %d", len(tasks))
	}

	// List by agent + state
	tasks, err = repo.List("agent-a", model.TaskStateWorking)
	if err != nil {
		t.Fatalf("List by agent+state failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

// --- MessageRepository tests ---

func TestCreateAndListMessages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a task first
	taskRepo := NewTaskRepository(db)
	task, err := taskRepo.Create(&model.TaskRecord{
		AgentName: "test-agent",
		State:     model.TaskStateWorking,
	})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}

	msgRepo := NewMessageRepository(db)

	// Create messages
	msg1, err := msgRepo.Create(&model.MessageRecord{
		TaskID:  task.ID,
		Role:    model.MessageRoleUser,
		Content: "Hello, agent!",
	})
	if err != nil {
		t.Fatalf("Create message 1 failed: %v", err)
	}
	if msg1.ID == 0 {
		t.Error("expected non-zero message ID")
	}

	_, err = msgRepo.Create(&model.MessageRecord{
		TaskID:  task.ID,
		Role:    model.MessageRoleAgent,
		Content: "Hello, user!",
	})
	if err != nil {
		t.Fatalf("Create message 2 failed: %v", err)
	}

	// List by task ID
	messages, err := msgRepo.ListByTaskID(task.ID)
	if err != nil {
		t.Fatalf("ListByTaskID failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != model.MessageRoleUser {
		t.Errorf("expected first role 'user', got '%s'", messages[0].Role)
	}
	if messages[1].Role != model.MessageRoleAgent {
		t.Errorf("expected second role 'agent', got '%s'", messages[1].Role)
	}
	if messages[0].Content != "Hello, agent!" {
		t.Errorf("expected content 'Hello, agent!', got '%s'", messages[0].Content)
	}
	if messages[1].Content != "Hello, user!" {
		t.Errorf("expected content 'Hello, user!', got '%s'", messages[1].Content)
	}
}

// --- TraceRepository tests ---

func TestRecordAndGetTimeline(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTraceRepository(db)

	taskID := "task-abc"

	_, err := repo.Record(&model.TraceEventRecord{
		TaskID:      taskID,
		ContextID:   "ctx-1",
		AgentName:   "agent-a",
		TargetAgent: "agent-b",
		EventType:   model.TraceEventSend,
		DataJSON:    `{"message": "hello"}`,
	})
	if err != nil {
		t.Fatalf("Record event 1 failed: %v", err)
	}

	_, err = repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		ContextID: "ctx-1",
		AgentName: "agent-b",
		EventType: model.TraceEventResponse,
		DataJSON:  `{"message": "hi back"}`,
	})
	if err != nil {
		t.Fatalf("Record event 2 failed: %v", err)
	}

	timeline, err := repo.GetTimeline(taskID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(timeline))
	}
	// Timeline should be ordered ASC by created_at
	if timeline[0].EventType != model.TraceEventSend {
		t.Errorf("expected first event type 'send', got '%s'", timeline[0].EventType)
	}
	if timeline[1].EventType != model.TraceEventResponse {
		t.Errorf("expected second event type 'response', got '%s'", timeline[1].EventType)
	}

	// Test GetDebugTrace
	debugTrace, err := repo.GetDebugTrace(taskID)
	if err != nil {
		t.Fatalf("GetDebugTrace failed: %v", err)
	}
	if !strings.Contains(debugTrace, "send") {
		t.Error("expected debug trace to contain 'send'")
	}
	if !strings.Contains(debugTrace, "response") {
		t.Error("expected debug trace to contain 'response'")
	}
	if !strings.Contains(debugTrace, `{"message": "hello"}`) {
		t.Error("expected debug trace to contain data JSON")
	}

	// Test GetDebugTrace for nonexistent task
	emptyTrace, err := repo.GetDebugTrace("nonexistent-task")
	if err != nil {
		t.Fatalf("GetDebugTrace for nonexistent failed: %v", err)
	}
	if emptyTrace != "(no trace yet)" {
		t.Errorf("expected '(no trace yet)', got '%s'", emptyTrace)
	}
}

func TestGetRecentByAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTraceRepository(db)

	// Record multiple events for different agents
	for i := 0; i < 5; i++ {
		_, err := repo.Record(&model.TraceEventRecord{
			TaskID:    fmt.Sprintf("task-%d", i),
			AgentName: "agent-a",
			EventType: model.TraceEventSend,
			DataJSON:  fmt.Sprintf(`{"index": %d}`, i),
		})
		if err != nil {
			t.Fatalf("Record event %d for agent-a failed: %v", i, err)
		}
	}

	// Record one event for a different agent
	_, err := repo.Record(&model.TraceEventRecord{
		TaskID:    "task-other",
		AgentName: "agent-b",
		EventType: model.TraceEventSend,
		DataJSON:  `{"index": 99}`,
	})
	if err != nil {
		t.Fatalf("Record event for agent-b failed: %v", err)
	}

	// Get recent 3 for agent-a
	recent, err := repo.GetRecentByAgent("agent-a", 3)
	if err != nil {
		t.Fatalf("GetRecentByAgent failed: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(recent))
	}

	// Should be ordered DESC (most recent first)
	if recent[0].DataJSON != `{"index": 4}` {
		t.Errorf("expected most recent to be index 4, got '%s'", recent[0].DataJSON)
	}
	if recent[2].DataJSON != `{"index": 2}` {
		t.Errorf("expected oldest in window to be index 2, got '%s'", recent[2].DataJSON)
	}

	// Verify all belong to agent-a
	for _, evt := range recent {
		if evt.AgentName != "agent-a" {
			t.Errorf("expected agent_name 'agent-a', got '%s'", evt.AgentName)
		}
	}
}
