package cmd

import (
	"encoding/json"
	"testing"
)

func TestHookInput_ClaudeFormat(t *testing.T) {
	// Claude Code sends session_id, transcript_path, and source.
	data := `{"session_id":"abc-123","transcript_path":"/tmp/transcript","source":"startup"}`
	var input hookInput
	if err := json.Unmarshal([]byte(data), &input); err != nil {
		t.Fatalf("Failed to parse Claude format: %v", err)
	}
	if input.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want abc-123", input.SessionID)
	}
	if input.Source != "startup" {
		t.Errorf("Source = %q, want startup", input.Source)
	}
	if input.TranscriptPath != "/tmp/transcript" {
		t.Errorf("TranscriptPath = %q, want /tmp/transcript", input.TranscriptPath)
	}
	// Copilot-specific fields should be zero
	if input.Timestamp != 0 {
		t.Errorf("Timestamp = %d, want 0", input.Timestamp)
	}
}

func TestHookInput_CopilotFormat(t *testing.T) {
	// Copilot CLI sends timestamp, cwd, source, and optionally initialPrompt.
	data := `{"timestamp":1709251200,"cwd":"/home/user/repo","source":"new","initialPrompt":"hello"}`
	var input hookInput
	if err := json.Unmarshal([]byte(data), &input); err != nil {
		t.Fatalf("Failed to parse Copilot format: %v", err)
	}
	if input.Timestamp != 1709251200 {
		t.Errorf("Timestamp = %d, want 1709251200", input.Timestamp)
	}
	if input.Cwd != "/home/user/repo" {
		t.Errorf("Cwd = %q, want /home/user/repo", input.Cwd)
	}
	if input.Source != "new" {
		t.Errorf("Source = %q, want new", input.Source)
	}
	if input.InitialPrompt != "hello" {
		t.Errorf("InitialPrompt = %q, want hello", input.InitialPrompt)
	}
	// Claude-specific fields should be empty
	if input.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", input.SessionID)
	}
}

func TestHookInput_CopilotSourceMapping(t *testing.T) {
	// Test that Copilot "new" source is interpreted correctly during parsing.
	// The actual mapping happens in readHookSessionID, but struct must support it.
	tests := []struct {
		name   string
		json   string
		source string
	}{
		{"new", `{"timestamp":123,"source":"new"}`, "new"},
		{"resume", `{"timestamp":123,"source":"resume"}`, "resume"},
		{"empty", `{"timestamp":123}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input hookInput
			if err := json.Unmarshal([]byte(tt.json), &input); err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}
			if input.Source != tt.source {
				t.Errorf("Source = %q, want %q", input.Source, tt.source)
			}
		})
	}
}

func TestHookInput_MalformedJSON(t *testing.T) {
	data := `{not valid json`
	var input hookInput
	if err := json.Unmarshal([]byte(data), &input); err == nil {
		t.Error("Expected error for malformed JSON")
	}
}

func TestHookInput_EmptyJSON(t *testing.T) {
	data := `{}`
	var input hookInput
	if err := json.Unmarshal([]byte(data), &input); err != nil {
		t.Fatalf("Failed to parse empty JSON: %v", err)
	}
	if input.SessionID != "" || input.Timestamp != 0 || input.Source != "" {
		t.Error("Empty JSON should result in zero values")
	}
}
