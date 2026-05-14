package handler

import (
	"encoding/json"
	"net/http"

	"a2a-platform/internal/svc"
)

// GetAgents returns a JSON list of all registered agents.
func GetAgents(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents := svcCtx.Registry.ListAgents()
		if agents == nil {
			writeJSON(w, http.StatusOK, []map[string]interface{}{})
			return
		}

		// Parse SkillsJSON into Skills field for each agent
		type agentOut struct {
			Name         string   `json:"name"`
			URL          string   `json:"url"`
			Description  string   `json:"description"`
			Version      string   `json:"version"`
			AgentType    string   `json:"type"`
			Status       string   `json:"status"`
			Skills       []string `json:"skills"`
			ErrorMessage string   `json:"error_message,omitempty"`
			CreatedAt    string   `json:"created_at"`
			UpdatedAt    string   `json:"updated_at"`
		}

		out := make([]agentOut, len(agents))
		for i, a := range agents {
			var skills []string
			if a.SkillsJSON != "" && a.SkillsJSON != "[]" {
				_ = json.Unmarshal([]byte(a.SkillsJSON), &skills)
			}
			out[i] = agentOut{
				Name:         a.Name,
				URL:          a.URL,
				Description:  a.Description,
				Version:      a.Version,
				AgentType:    a.AgentType,
				Status:       string(a.Status),
				Skills:       skills,
				ErrorMessage: a.ErrorMessage,
				CreatedAt:    a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt:    a.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
		}

		writeJSON(w, http.StatusOK, out)
	}
}

// GetAgent returns a single agent by name.
func GetAgent(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathParam(r, "name")
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent name is required"})
			return
		}

		agent, err := svcCtx.Registry.GetAgent(name)
		if err != nil || agent == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Agent '" + name + "' not found"})
			return
		}

		var skills []string
		if agent.SkillsJSON != "" && agent.SkillsJSON != "[]" {
			_ = json.Unmarshal([]byte(agent.SkillsJSON), &skills)
		}

		out := map[string]interface{}{
			"name":        agent.Name,
			"url":         agent.URL,
			"description": agent.Description,
			"version":     agent.Version,
			"type":        agent.AgentType,
			"status":      string(agent.Status),
			"skills":      skills,
			"created_at":  agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at":  agent.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if agent.ErrorMessage != "" {
			out["error_message"] = agent.ErrorMessage
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteAgent removes an agent by name.
func DeleteAgent(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathParam(r, "name")
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent name is required"})
			return
		}

		err := svcCtx.Registry.DeleteAgent(name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
	}
}

// RegisterAgent registers a new agent from a JSON body.
func RegisterAgent(svcCtx *svc.ServiceContext) http.HandlerFunc {
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

		conn, err := svcCtx.Registry.ConnectByURL(body.URL, body.AgentType)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to connect to agent: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":  "connected",
			"name":    conn.Name,
			"url":     conn.URL,
			"type":    conn.Type,
			"version": conn.Card.Version,
		})
	}
}

// AddAgent is a convenience endpoint that accepts {"url":"..."} and registers the agent.
func AddAgent(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return RegisterAgent(svcCtx)
}
