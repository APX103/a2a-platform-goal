package a2a

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ParseSSEStream reads SSE data from a reader and returns parsed events.
// SSE format: lines starting with "data: " contain JSON, empty line ends event.
func ParseSSEStream(r io.Reader) ([]SSEEventEnvelope, error) {
	var events []SSEEventEnvelope
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "" && len(dataLines) > 0 {
			raw := strings.Join(dataLines, "\n")
			var event SSEEventEnvelope
			if err := json.Unmarshal([]byte(raw), &event); err == nil {
				events = append(events, event)
			}
			dataLines = nil
		}
	}
	return events, scanner.Err()
}

// WriteSSEData writes a single SSE data line followed by double newline.
func WriteSSEData(w io.Writer, data string) {
	w.Write([]byte("data: " + data + "\n\n"))
}

// IsSSEContentType checks if the content type indicates SSE.
func IsSSEContentType(contentType string) bool {
	return strings.Contains(contentType, "text/event-stream")
}
