package main

import (
	"encoding/json"
	"net/http"
)

// setSSEHeaders sets the standard Server-Sent Events response headers.
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// writeSSEEvent writes a single SSE event with the given type and data payload.
func writeSSEEvent(w http.ResponseWriter, eventType string, data interface{}) {
	payload := map[string]interface{}{
		"type": eventType,
		"data": data,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}
	w.Write([]byte("data: " + string(jsonData) + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
