package messagebus

import (
	"encoding/json"
	"fmt"
	"strings"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/internal/tracer"
	"a2a-platform/pkg/a2a"
)

// StreamingEvent represents a single event from a streaming A2A interaction.
type StreamingEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// MessageBus orchestrates A2A communication between agents.
type MessageBus struct {
	taskRepo    repository.TaskRepository
	messageRepo repository.MessageRepository
	tracer      *tracer.Tracer
	getClient   func(name string) *a2a.Client
}

// NewMessageBus creates a new MessageBus with the given repositories and tracer.
func NewMessageBus(taskRepo repository.TaskRepository, messageRepo repository.MessageRepository, tr *tracer.Tracer) *MessageBus {
	return &MessageBus{
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
		tracer:      tr,
	}
}

// SetRegistry sets the function to look up a client for a connected agent.
func (mb *MessageBus) SetRegistry(getClient func(name string) *a2a.Client) {
	mb.getClient = getClient
}

// CreateTask creates a new task record for the given agent and returns it.
func (mb *MessageBus) CreateTask(agentName string) (*model.TaskRecord, error) {
	task := &model.TaskRecord{
		AgentName: agentName,
		State:     model.TaskStateSubmitted,
	}

	created, err := mb.taskRepo.Create(task)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	return created, nil
}

// Send sends a message to an agent and returns the response text.
// It collects the full streaming exchange and returns only the final response.
func (mb *MessageBus) Send(sender, agentName, content, contextID string) (string, error) {
	task, events, err := mb.SendStreaming(sender, agentName, content, contextID)
	if err != nil {
		return "", err
	}

	// Extract response text from events
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == "response" {
			if data, ok := events[i].Data.(map[string]interface{}); ok {
				if text, ok := data["text"].(string); ok {
					return text, nil
				}
			}
		}
	}

	// Fallback: return task local ID so caller knows a task was created
	return fmt.Sprintf("(task %s created, no response)", task.LocalTaskID), nil
}

// SendStreaming sends a streaming message to an agent and returns the task,
// streaming events, and any error.
func (mb *MessageBus) SendStreaming(sender, agentName, content, contextID string) (*model.TaskRecord, []StreamingEvent, error) {
	if mb.getClient == nil {
		return nil, nil, fmt.Errorf("registry not configured")
	}

	client := mb.getClient(agentName)
	if client == nil {
		return nil, nil, fmt.Errorf("agent '%s' not connected", agentName)
	}

	// Create task
	task, err := mb.CreateTask(agentName)
	if err != nil {
		return nil, nil, fmt.Errorf("send streaming: %w", err)
	}

	taskID := task.LocalTaskID

	// Record user message
	_, err = mb.messageRepo.Create(&model.MessageRecord{
		TaskID:  task.ID,
		Role:    model.MessageRoleUser,
		Content: content,
	})
	if err != nil {
		return task, nil, fmt.Errorf("record user message: %w", err)
	}

	// Record send trace
	mb.tracer.RecordSend(taskID, content, contextID, agentName)

	// Call agent
	events, err := client.SendStreamingMessage("user", content, taskID)
	if err != nil {
		mb.tracer.RecordError(taskID, err)
		_ = mb.taskRepo.UpdateState(taskID, model.TaskStateError)
		return task, nil, fmt.Errorf("send streaming message: %w", err)
	}

	var streamingEvents []StreamingEvent
	var responseText string
	var serverTaskID string

	// Parse SSE events
	for _, evt := range events {
		switch evt.Type {
		case "task":
			var taskData struct {
				ID        string `json:"id"`
				ContextID string `json:"contextId"`
				Status    struct {
					State string `json:"state"`
				} `json:"status"`
			}
			if err := json.Unmarshal(evt.Data, &taskData); err == nil {
				serverTaskID = taskData.ID
				mb.tracer.RecordTaskCreated(taskID, serverTaskID, contextID)
				if taskData.Status.State != "" {
					mb.tracer.RecordTaskUpdate(taskID, string(model.TaskStateSubmitted), taskData.Status.State)
				}

				// Update context_id if server provided one
				if taskData.ContextID != "" && contextID == "" {
					contextID = taskData.ContextID
				}

				streamingEvents = append(streamingEvents, StreamingEvent{
					Type: "task",
					Data: map[string]interface{}{
						"server_task_id": serverTaskID,
						"state":          taskData.Status.State,
					},
				})
			}

		case "status_update":
			var statusData struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(evt.Data, &statusData); err == nil {
				oldState := string(task.State)
				newState := statusData.State
				_ = mb.taskRepo.UpdateState(taskID, model.TaskState(newState))
				task.State = model.TaskState(newState)
				mb.tracer.RecordTaskUpdate(taskID, oldState, newState)

				streamingEvents = append(streamingEvents, StreamingEvent{
					Type: "status_update",
					Data: map[string]interface{}{"old_state": oldState, "new_state": newState},
				})
			}

		case "artifact_update":
			var artifactData struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			}
			if err := json.Unmarshal(evt.Data, &artifactData); err == nil {
				for _, part := range artifactData.Parts {
					if part.Text != "" {
						mb.tracer.RecordArtifact(taskID, part.Text)
						if responseText == "" {
							responseText = part.Text
						}
						streamingEvents = append(streamingEvents, StreamingEvent{
							Type: "artifact",
							Data: map[string]interface{}{"text": part.Text},
						})
					}
				}
			}

		case "message":
			var msgData struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			}
			if err := json.Unmarshal(evt.Data, &msgData); err == nil {
				for _, part := range msgData.Parts {
					if part.Text != "" {
						responseText = part.Text
					}
				}
				streamingEvents = append(streamingEvents, StreamingEvent{
					Type: "message",
					Data: map[string]interface{}{
						"role":  msgData.Role,
						"text":  responseText,
					},
				})
			}

		default:
			// Unknown event type - include as raw data
			streamingEvents = append(streamingEvents, StreamingEvent{
				Type: "unknown",
				Data: json.RawMessage(evt.Data),
			})
		}
	}

	// Fallback: if no response text, use "(empty)"
	if responseText == "" {
		responseText = "(empty)"
	}

	// Update task state to RESPONDED
	_ = mb.taskRepo.UpdateState(taskID, model.TaskStateResponded)
	task.State = model.TaskStateResponded

	// Record assistant message
	_, _ = mb.messageRepo.Create(&model.MessageRecord{
		TaskID:  task.ID,
		Role:    model.MessageRoleAgent,
		Content: responseText,
	})

	// Record response trace
	mb.tracer.RecordResponse(taskID, responseText, agentName)

	streamingEvents = append(streamingEvents, StreamingEvent{
		Type: "response",
		Data: map[string]interface{}{"text": responseText},
	})

	return task, streamingEvents, nil
}

// ListTasks lists tasks for an agent with optional state filter.
func (mb *MessageBus) ListTasks(agentName string, state model.TaskState) ([]*model.TaskRecord, error) {
	return mb.taskRepo.List(agentName, state)
}

// GetDebugTrace returns a formatted debug trace for a task.
func (mb *MessageBus) GetDebugTrace(taskID string) string {
	return mb.tracer.GetDebugTrace(taskID)
}

// parseTextFromParts extracts text from SSE event parts data.
func parseTextFromParts(data json.RawMessage) string {
	var msgData struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(data, &msgData); err != nil {
		return ""
	}
	var texts []string
	for _, part := range msgData.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "")
}
