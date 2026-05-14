package tracer

import (
	"encoding/json"
	"fmt"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
)

// Tracer records trace events for A2A interactions and provides query methods.
type Tracer struct {
	repo repository.TraceRepository
}

// New creates a new Tracer backed by the given TraceRepository.
func New(repo repository.TraceRepository) *Tracer {
	return &Tracer{repo: repo}
}

// RecordSend records a "send" event where a message is sent to an agent.
func (t *Tracer) RecordSend(taskID, content, contextID, agentName string) {
	data, _ := json.Marshal(map[string]string{"content": content})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		ContextID: contextID,
		AgentName: agentName,
		EventType: model.TraceEventSend,
		DataJSON:  string(data),
	})
}

// RecordTaskCreated records a "task_created" event when a new task is created.
func (t *Tracer) RecordTaskCreated(taskID, serverTaskID, contextID string) {
	data, _ := json.Marshal(map[string]string{"server_task_id": serverTaskID})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		ContextID: contextID,
		EventType: model.TraceEventTaskCreated,
		DataJSON:  string(data),
	})
}

// RecordTaskUpdate records a "task_update" event when a task state changes.
func (t *Tracer) RecordTaskUpdate(taskID, oldState, newState string) {
	data, _ := json.Marshal(map[string]string{
		"old_state": oldState,
		"new_state": newState,
	})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		EventType: model.TraceEventTaskUpdate,
		DataJSON:  string(data),
	})
}

// RecordArtifact records an "artifact" event when an artifact is produced.
func (t *Tracer) RecordArtifact(taskID, text string) {
	data, _ := json.Marshal(map[string]string{"text": text})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		EventType: model.TraceEventArtifact,
		DataJSON:  string(data),
	})
}

// RecordResponse records a "response" event when an agent responds.
func (t *Tracer) RecordResponse(taskID, text, source string) {
	data, _ := json.Marshal(map[string]string{
		"text":   text,
		"source": source,
	})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		EventType: model.TraceEventResponse,
		DataJSON:  string(data),
	})
}

// RecordError records an "error" event when an error occurs.
func (t *Tracer) RecordError(taskID string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	data, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = t.repo.Record(&model.TraceEventRecord{
		TaskID:    taskID,
		EventType: model.TraceEventError,
		DataJSON:  string(data),
	})
}

// GetTimeline returns all trace events for a task ordered by time.
func (t *Tracer) GetTimeline(taskID string) ([]*model.TraceEventRecord, error) {
	return t.repo.GetTimeline(taskID)
}

// GetDebugTrace returns a formatted debug trace string for a task.
func (t *Tracer) GetDebugTrace(taskID string) string {
	s, err := t.repo.GetDebugTrace(taskID)
	if err != nil {
		return fmt.Sprintf("(error: %v)", err)
	}
	return s
}

// GetRecentByAgent returns the most recent trace events for an agent.
func (t *Tracer) GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error) {
	return t.repo.GetRecentByAgent(agentName, limit)
}
