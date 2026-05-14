package handler

import (
	"net/http"

	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
)

// GetTasks returns tasks filtered by optional agent_name and state query params.
func GetTasks(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentName := r.URL.Query().Get("agent_name")
		state := r.URL.Query().Get("state")

		tasks, err := svcCtx.TaskRepo.List(agentName, model.TaskState(state))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, tasks)
	}
}

// GetTaskTrace returns the debug trace for a task.
func GetTaskTrace(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := pathParam(r, "id")
		if taskID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}

		trace := svcCtx.Tracer.GetDebugTrace(taskID)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(trace))
	}
}
