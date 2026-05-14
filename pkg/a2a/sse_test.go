package a2a

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSSEStream(t *testing.T) {
	input := `data: {"type":"task","data":{"id":"task-1","status":"WORKING"}}

data: {"type":"status_update","data":{"status":"COMPLETED"}}

data: {"type":"message","data":{"role":"agent","parts":[{"kind":"text","text":"hello"}]}}

`
	events, err := ParseSSEStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSSEStream returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}

	if events[0].Type != "task" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "task")
	}
	var taskData map[string]interface{}
	if err := json.Unmarshal(events[0].Data, &taskData); err != nil {
		t.Fatalf("failed to unmarshal events[0].Data: %v", err)
	}
	if taskData["id"] != "task-1" {
		t.Errorf("events[0].Data.id = %v, want %q", taskData["id"], "task-1")
	}

	if events[1].Type != "status_update" {
		t.Errorf("events[1].Type = %q, want %q", events[1].Type, "status_update")
	}

	if events[2].Type != "message" {
		t.Errorf("events[2].Type = %q, want %q", events[2].Type, "message")
	}
}

func TestParseSSEStream_Empty(t *testing.T) {
	events, err := ParseSSEStream(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseSSEStream returned error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}
}

func TestParseSSEStream_Malformed(t *testing.T) {
	input := `data: {"type":"task","data":{"id":"task-1"}}

data: this is not json

data: {"type":"message","data":{"role":"agent"}}

`
	events, err := ParseSSEStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSSEStream returned error: %v", err)
	}
	// Malformed JSON should be silently skipped
	if len(events) != 2 {
		t.Errorf("len(events) = %d, want 2 (malformed event skipped)", len(events))
	}
	if events[0].Type != "task" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "task")
	}
	if events[1].Type != "message" {
		t.Errorf("events[1].Type = %q, want %q", events[1].Type, "message")
	}
}

func TestWriteSSEData(t *testing.T) {
	var buf bytes.Buffer
	WriteSSEData(&buf, `{"type":"task","data":{}}`)

	output := buf.String()
	if !strings.HasPrefix(output, "data: ") {
		t.Errorf("output missing 'data: ' prefix, got: %q", output)
	}
	if !strings.HasSuffix(output, "\n\n") {
		t.Errorf("output missing '\\n\\n' suffix, got: %q", output)
	}

	// Verify the content between prefix and suffix
	inner := output[len("data: "):]
	inner = inner[:len(inner)-len("\n\n")]
	if inner != `{"type":"task","data":{}}` {
		t.Errorf("inner content = %q, want %q", inner, `{"type":"task","data":{}}`)
	}
}

func TestIsSSEContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/event-stream", true},
		{"text/event-stream; charset=utf-8", true},
		{"application/json", false},
		{"text/plain", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := IsSSEContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("IsSSEContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
