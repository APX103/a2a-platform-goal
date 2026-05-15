package handler

import (
	"encoding/json"
	"net/http"

	"a2a-platform/internal/svc"
	"a2a-platform/pkg/a2a"
)

// Chat handles SSE chat requests: sends a message to an agent and streams events back.
func Chat(svcCtx *svc.ServiceContext) http.HandlerFunc {
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

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, canFlush := w.(http.Flusher)

		// Send streaming message
		task, events, err := svcCtx.MessageBus.SendStreaming("user", body.AgentName, body.Message, body.ContextID)
		if err != nil {
			errData, _ := json.Marshal(map[string]string{"error": err.Error()})
			a2a.WriteSSEData(w, string(errData))
			if canFlush {
				flusher.Flush()
			}
			return
		}

		// Write task creation event
		taskData, _ := json.Marshal(map[string]string{
			"type":          "task",
			"local_task_id": task.LocalTaskID,
			"agent_name":    task.AgentName,
			"state":         string(task.State),
		})
		a2a.WriteSSEData(w, string(taskData))
		if canFlush {
			flusher.Flush()
		}

		// Stream each event
		for _, evt := range events {
			evtData, _ := json.Marshal(evt)
			a2a.WriteSSEData(w, string(evtData))
			if canFlush {
				flusher.Flush()
			}
		}
	}
}
