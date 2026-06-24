package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func testDecoder() *Decoder {
	return NewDecoder(&DecoderConfig{
		HostnameKeys:  []string{"source"},
		TimestampKeys: []string{"timestamp"},
		LevelKeys:     []string{"level"},
		TextKeys:      []string{"message"},
		FieldKeys:     []string{"request_id"},
	})
}

func testMessage() *Message {
	return &Message{
		Message: map[string]any{
			"timestamp":  "2026-06-22T01:02:03.004Z",
			"level":      float64(3),
			"source":     "api-1",
			"message":    "failed request",
			"request_id": "req-1",
		},
	}
}

func TestRenderMessageCompact(t *testing.T) {
	got, err := RenderMessage(testDecoder(), false, testMessage(), OutputCompact)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	for _, want := range []string{"api-1", "2026-06-22T01:02:03.004Z", "ERROR", "failed request", "request_id:req-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderMessage() = %q, missing %q", got, want)
		}
	}
}

func TestRenderMessagePretty(t *testing.T) {
	got, err := RenderMessage(testDecoder(), false, testMessage(), OutputPretty)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	for _, want := range []string{"2026-06-22T01:02:03.004Z ERROR api-1", "message: failed request", "request_id: req-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderMessage() = %q, missing %q", got, want)
		}
	}
}

func TestRenderMessageJSON(t *testing.T) {
	got, err := RenderMessage(testDecoder(), false, testMessage(), OutputJSON)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("RenderMessage() returned invalid JSON: %v", err)
	}
	if decoded["message"] != "failed request" {
		t.Fatalf("message = %v, want failed request", decoded["message"])
	}
}
