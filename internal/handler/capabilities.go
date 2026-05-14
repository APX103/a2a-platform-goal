package handler

import (
	"net/http"

	"a2a-platform/internal/messagebus"
	"a2a-platform/internal/svc"
)

// GetCapabilities returns the platform capabilities and available tools.
func GetCapabilities(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":         "a2a-platform-host",
			"description":  "A2A protocol host platform",
			"version":      "0.1.0",
			"capabilities": map[string]interface{}{"streaming": true},
			"tools":        messagebus.HostTools,
		})
	}
}
