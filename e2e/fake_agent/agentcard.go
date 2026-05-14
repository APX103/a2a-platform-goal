package main

import (
	"encoding/json"
	"net/http"
)

// handleAgentCard returns a JSON AgentCard for the fake agent.
// The card includes the agent name and current mode in its description and skills.
func handleAgentCard(w http.ResponseWriter, r *http.Request, name string, mode string) {
	card := map[string]interface{}{
		"name":        name,
		"description": "Fake agent for testing (" + mode + ")",
		"version":     "1.0.0",
		"url":         "http://" + r.Host,
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
}
